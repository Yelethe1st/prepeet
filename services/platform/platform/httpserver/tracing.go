package httpserver

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// tracerName identifies this instrumentation in the trace, so a span produced
// here is distinguishable from one produced by a library.
const tracerName = "github.com/Yelethe1st/prepeet/services/platform/platform/httpserver"

// knownMethods bounds the span name.
//
// The HTTP method is caller controlled, and Go's server accepts any token as
// one. A span named from an unchecked method lets a caller create unbounded
// span names, which is the same cardinality problem as naming spans after
// paths and is easier to miss because the method looks like a fixed set.
var knownMethods = map[string]struct{}{
	http.MethodGet: {}, http.MethodHead: {}, http.MethodPost: {},
	http.MethodPut: {}, http.MethodPatch: {}, http.MethodDelete: {},
	http.MethodOptions: {},
}

// newRequestDuration builds the instrument measuring how long requests take,
// broken down by route and outcome.
//
// A histogram rather than a counter and a sum, because the useful question is
// about the tail. An average latency stays acceptable while the slowest one
// request in a hundred times out, and that one request is the incident.
//
// It is built when the server is built, not at package initialisation. An
// instrument binds to whichever meter provider is installed at the moment it is
// created, so one created at init would bind to the noop provider that exists
// before telemetry.Setup runs and would then record nothing, silently, forever.
// Building it per request would instead allocate on the hot path and register a
// duplicate on every call.
func newRequestDuration() metric.Float64Histogram {
	instrument, err := telemetry.Meter(tracerName).Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("Duration of inbound HTTP requests."),
		metric.WithUnit("s"),
		// Boundaries in seconds, weighted towards the range this API should
		// live in. The SDK default is coarse above one second, which is
		// precisely where a p99 investigation needs resolution.
		metric.WithExplicitBucketBoundaries(
			0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10),
	)
	if err != nil {
		// Only a malformed instrument definition reaches here, which is a
		// programming error in the lines above rather than a runtime condition.
		panic("httpserver: building the request duration histogram: " + err.Error())
	}
	return instrument
}

// withTracing opens one span per request and closes it however the request
// ends, including a panic. It also measures the request.
//
// It takes the route mux rather than only the next handler because the span
// name is the route pattern, which only the mux can resolve.
func withTracing(routes *http.ServeMux, next http.Handler) http.Handler {
	requestDuration := newRequestDuration()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, route, name := describe(routes, r)

		// The inbound trace context is extracted before the span is started, so
		// a request arriving from the web tier continues that trace instead of
		// beginning a second one describing the same user action.
		// The inbound trace context is extracted before the span is started, so
		// a request arriving from the web tier continues that trace instead of
		// beginning a second one describing the same user action.
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		ctx, span := telemetry.Tracer(tracerName).Start(ctx,
			name,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodOriginal(method),
				semconv.HTTPRoute(route),
				telemetry.MustAttr(telemetry.KeyRequestID, RequestIDFrom(r.Context())),
			),
		)
		defer span.End()

		// The query string and the resolved path are deliberately absent. Both
		// are caller controlled free text, and a path segment in this API is an
		// identifier that the route pattern already describes.

		recorder := &statusRecorder{ResponseWriter: w}
		started := time.Now()

		defer func() {
			recovered := recover()
			if recovered != nil {
				// The message goes to the trace and never to the client. A
				// panic message is written by whoever wrote the code that
				// failed, with no classification in mind, so it is scrubbed
				// like any other free text.
				span.RecordError(
					fmt.Errorf("%s", telemetry.Scrub(fmt.Sprint(recovered))),
					trace.WithStackTrace(true),
				)
				span.SetStatus(codes.Error, "panic")

				if !recorder.written {
					WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR",
						"Something went wrong on our side. Please try again.", true)
				}
			}

			status := recorder.status
			if recovered != nil {
				status = http.StatusInternalServerError
			}

			span.SetAttributes(semconv.HTTPResponseStatusCode(status))

			// Only a server failure is an error. Marking 4xx as errors produces
			// an error rate that tracks how many people mistyped a password,
			// which trains everyone to ignore it.
			if recovered == nil && status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(status))
			}

			// Measured last, and measured for panics too. Leaving failures out
			// of the distribution removes exactly the requests that went worst,
			// which is how a latency graph stays flat through an incident.
			//
			// The dimensions are deliberately only these three. A metric
			// attribute is repeated across every series, so the request
			// identifier here would create one time series per request.
			requestDuration.Record(ctx, time.Since(started).Seconds(),
				metric.WithAttributes(
					attribute.String("http.route", route),
					attribute.String("http.request.method", method),
					attribute.Int("http.response.status_code", status),
				))
		}()

		next.ServeHTTP(recorder, r.WithContext(ctx))
	})
}

// describe returns the bounded method, the route, and the span name.
//
// Go's ServeMux patterns usually carry their own method, as in
// "GET /api/v1/sessions/{sessionID}", so the name is the pattern as registered.
// A pattern registered without one gets the method prepended, which is what
// makes span names uniform whichever way a route was declared.
func describe(routes *http.ServeMux, r *http.Request) (method, route, name string) {
	method = r.Method
	if _, known := knownMethods[method]; !known {
		method = "OTHER"
	}

	_, pattern := routes.Handler(r)
	if pattern == "" {
		// An unmatched path is never used as a name. Otherwise anyone can mint
		// span names by requesting paths that do not exist.
		return method, "(unmatched)", method + " (unmatched)"
	}

	if _, hasMethod := knownMethods[strings.Fields(pattern)[0]]; hasMethod {
		return method, pattern, pattern
	}
	return method, pattern, method + " " + pattern
}

// statusRecorder remembers the status code so the span can record it.
//
// It reports 200 before anything is written, because a handler that writes a
// body without calling WriteHeader has still answered 200 and a span saying
// nothing would be worse than one saying that.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (s *statusRecorder) WriteHeader(status int) {
	if !s.written {
		s.status = status
		s.written = true
	}
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.written {
		s.status = http.StatusOK
		s.written = true
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer, so flushing
// and hijacking keep working through this wrapper. Streaming responses depend
// on it, and a wrapper that silently disabled them would be found much later.
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
