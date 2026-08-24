package telemetry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Config configures telemetry for one process.
type Config struct {
	// ServiceName appears on every span, so a trace crossing api and worker
	// says which produced which part.
	ServiceName string
	Environment string
	// Endpoint is the OTLP collector. Empty disables export entirely, which is
	// the local default: an engineer running the stack should not need a
	// collector, and spans that go nowhere still exercise the same code paths.
	Endpoint string
	// SampleRatio is the fraction of traces recorded. One in deployed
	// environments until volume says otherwise, because a product with no
	// traffic gains nothing from sampling and loses the trace it needed.
	SampleRatio float64
	// MetricInterval is how often metrics are exported. Zero selects the
	// default below.
	MetricInterval time.Duration
}

// defaultMetricInterval is how often metrics are pushed.
//
// Metrics are not sampled, so the interval is the only volume control. Thirty
// seconds is short enough that a scaling trigger is visible while it matters
// and long enough that the export is not itself a load.
const defaultMetricInterval = 30 * time.Second

// Shutdown flushes and stops telemetry. It is returned rather than registered,
// so the process decides when to stop and can wait for the flush: spans buffered
// at exit are spans describing the failure that caused the exit.
type Shutdown func(context.Context) error

// OTLP is chosen deliberately over a vendor SDK. The observability vendor is
// still an open decision in docs/operations/deployment-topology.md, and a
// vendor-specific client would quietly make that decision by being expensive to
// remove.
//
// Setup installs the global tracer, meter and propagator.
//
// Traces and metrics answer different questions and both are needed. A trace
// says what happened to one request; a metric says whether the thing is getting
// worse, which is the question every scaling trigger in ADR-0006 is written
// against and the one a trace cannot answer cheaply.
func Setup(ctx context.Context, cfg Config) (Shutdown, error) {
	// Propagation is set even when export is off, so a request arriving with a
	// trace context still carries it onward. Otherwise local development would
	// behave differently from deployment in the one respect a distributed trace
	// depends on.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	if cfg.Endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetMeterProvider(metricnoop.NewMeterProvider())
		return func(context.Context) error { return nil }, nil
	}

	traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(cfg.Endpoint), otlptracegrpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("telemetry: building the OTLP trace exporter: %w", err)
	}

	metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpoint(cfg.Endpoint), otlpmetricgrpc.WithInsecure())
	if err != nil {
		// The trace exporter is already open, so it is closed rather than
		// leaked. A partial Setup that returns an error must leave nothing
		// running, or a caller that exits on the error still holds a connection.
		return nil, errors.Join(
			fmt.Errorf("telemetry: building the OTLP metric exporter: %w", err),
			traceExporter.Shutdown(ctx),
		)
	}

	ratio := cfg.SampleRatio
	if ratio <= 0 {
		ratio = 1
	}

	resource := resourceFor(cfg)

	interval := cfg.MetricInterval
	if interval <= 0 {
		interval = defaultMetricInterval
	}

	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter,
			sdkmetric.WithInterval(interval))),
		sdkmetric.WithResource(resource),
	)
	otel.SetMeterProvider(meterProvider)

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		// ParentBased keeps a trace whole. Sampling each span independently
		// produces traces with holes, which are worse than no trace: a gap
		// reads as an absence of work rather than an absence of recording.
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
		sdktrace.WithResource(resource),
	)
	otel.SetTracerProvider(provider)

	// Providers are shut down before their exporters, so buffered telemetry is
	// flushed through a connection that is still open. The reverse order loses
	// exactly the data produced closest to the shutdown, which is the data
	// describing whatever caused it.
	return func(ctx context.Context) error {
		return errors.Join(
			provider.Shutdown(ctx),
			meterProvider.Shutdown(ctx),
			traceExporter.Shutdown(ctx),
			metricExporter.Shutdown(ctx),
		)
	}, nil
}

// requestIDKey carries the correlation identifier through a request.
type requestIDKey struct{}

// WithRequestID records the correlation identifier for this request.
//
// It lives here rather than in platform/httpserver, where it is set, because
// everything that needs to read it is not the HTTP layer. An audit write, a
// workflow start and a log line all want the identifier, and a bounded context
// reaching into the transport package to get it would make a domain service
// depend on how the request arrived.
//
// Correlation is what this package is for, so this is its home. httpserver puts
// the value in; anything may read it.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestIDFrom returns the correlation identifier, or an empty string.
//
// Empty rather than an error, because most callers are attaching it to
// something they are writing anyway. A missing identifier makes that record
// harder to correlate; it does not make the operation wrong.
func RequestIDFrom(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

// Tracer returns the tracer for a component.
func Tracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// Meter returns the meter for a component.
//
// Call it after Setup. An instrument binds to whichever provider is installed
// when the instrument is created, so one created during package initialisation
// binds to the noop provider and records nothing for the life of the process,
// without erroring. Instruments therefore belong in a constructor.
func Meter(name string) metric.Meter {
	return otel.Meter(name)
}

// NewLogger builds the structured logger, correlated with the active trace.
//
// A log line and a span describing the same moment are useless separately: the
// log says what happened and the trace says where it sat in the request. The
// handler below joins them, so a log line found in a search leads to its trace
// and back.
//
// Every message is scrubbed. Log messages are written by whoever is debugging
// at the time, with no classification in mind, and that is precisely how a
// candidate's address ends up in a telemetry store.
//
// out is an io.Writer rather than *os.File so the scanner in SEC-08 can assert
// against what this function actually produces. A logger whose scrubbing can
// only be tested by rebuilding it elsewhere is a logger nobody has tested.
func NewLogger(cfg Config, out io.Writer) *slog.Logger {
	base := slog.NewJSONHandler(out, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Value.Kind() == slog.KindString {
				attr.Value = slog.StringValue(Scrub(attr.Value.String()))
			}
			return attr
		},
	})

	return slog.New(&correlatingHandler{
		inner:       base,
		serviceName: cfg.ServiceName,
		environment: cfg.Environment,
	})
}

// correlatingHandler adds trace correlation and the service identity to every
// record, so no call site has to remember either.
type correlatingHandler struct {
	inner       slog.Handler
	serviceName string
	environment string
}

func (h *correlatingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *correlatingHandler) Handle(ctx context.Context, record slog.Record) error {
	record.Message = Scrub(record.Message)
	record.AddAttrs(
		slog.String("service", h.serviceName),
		slog.String("environment", h.environment),
	)

	if span := trace.SpanContextFromContext(ctx); span.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", span.TraceID().String()),
			slog.String("span_id", span.SpanID().String()),
		)
	}
	return h.inner.Handle(ctx, record)
}

func (h *correlatingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &correlatingHandler{inner: h.inner.WithAttrs(attrs), serviceName: h.serviceName, environment: h.environment}
}

func (h *correlatingHandler) WithGroup(name string) slog.Handler {
	return &correlatingHandler{inner: h.inner.WithGroup(name), serviceName: h.serviceName, environment: h.environment}
}

// RecordDuration attaches a duration to the active span using the approved key,
// so timings do not each invent their own attribute name.
func RecordDuration(span trace.Span, since time.Time) {
	span.SetAttributes(attribute.Int64(string(KeyDurationMS), time.Since(since).Milliseconds()))
}

// resourceFor describes this process on every span it produces.
//
// Only identity and environment. A resource attribute is attached to every
// span, so anything restricted put here would be repeated on every span in the
// system rather than on one.
func resourceFor(cfg Config) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(cfg.ServiceName),
		semconv.DeploymentEnvironment(cfg.Environment),
	)
}
