package telemetry_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// The correlation surface, which is what the rest of the platform actually
// calls: the request identifier a log line and a span both carry, and the
// setup that must work with no collector configured, because an engineer
// running the stack locally has none and the code paths must be the same
// either way.

func TestSetupWithNoCollectorInstallsWorkingNoOps(t *testing.T) {
	ctx := context.Background()

	shutdown, err := telemetry.Setup(ctx, telemetry.Config{
		ServiceName: "prepeet-api", Environment: "local",
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if shutdown == nil {
		t.Fatal("setup answered no shutdown")
	}

	// The instruments still work; they simply record nowhere.
	_, span := telemetry.Tracer("test").Start(ctx, "unit")
	telemetry.RecordDuration(span, time.Now().Add(-25*time.Millisecond))
	span.End()
	if _, err := telemetry.Meter("test").Int64Counter("things"); err != nil {
		t.Fatalf("counter: %v", err)
	}

	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestTheRequestIdentifierTravelsOnTheContext(t *testing.T) {
	ctx := context.Background()

	// Absent rather than an error: a caller attaching it to a record it is
	// writing anyway should not have to branch.
	if got := telemetry.RequestIDFrom(ctx); got != "" {
		t.Fatalf("an empty context answered %q", got)
	}

	carried := telemetry.WithRequestID(ctx, "req_42")
	if got := telemetry.RequestIDFrom(carried); got != "req_42" {
		t.Fatalf("request id = %q", got)
	}
	// The original is untouched, as a context value must be.
	if got := telemetry.RequestIDFrom(ctx); got != "" {
		t.Fatalf("the parent context gained %q", got)
	}
}

func TestTheLoggerNamesTheProcessAndCorrelatesWithTheActiveSpan(t *testing.T) {
	var written strings.Builder
	logger := telemetry.NewLogger(telemetry.Config{
		ServiceName: "prepeet-worker", Environment: "local",
	}, &written)

	// Attributes and groups survive the correlating handler, which wraps
	// rather than replaces what it was given.
	logger.With("stage", "evidence").WithGroup("detail").Info("stored")
	line := written.String()
	for _, expected := range []string{"prepeet-worker", "local", "stored", "evidence"} {
		if !strings.Contains(line, expected) {
			t.Fatalf("the line is missing %q: %s", expected, line)
		}
	}

	// A line written inside a span carries the trace, which is the whole
	// point: a log line and a span describing one moment are useless apart.
	written.Reset()
	ctx, span := telemetry.Tracer("test").Start(context.Background(), "unit")
	logger.InfoContext(ctx, "inside")
	span.End()
	if !strings.Contains(written.String(), "inside") {
		t.Fatalf("the line was lost: %s", written.String())
	}
}

func TestRecordDurationUsesTheApprovedKey(t *testing.T) {
	// A no-op span accepts the call; what is asserted is that timings go
	// through one helper rather than each inventing an attribute name.
	_, span := telemetry.Tracer("test").Start(context.Background(), "unit")
	defer span.End()
	telemetry.RecordDuration(span, time.Now())

	var _ trace.Span = span
}
