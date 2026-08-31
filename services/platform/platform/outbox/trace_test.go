package outbox_test

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// PLT-08's open criterion is that one trace spans the journey with no broken
// links, and the outbox is where it broke first. A request that publishes an
// event and the worker that later delivers it are one piece of work to
// everybody except the tracing system, which saw two: the trace ended at the
// HTTP response, and an unrelated one began when the dispatcher picked the row
// up. Every question worth asking of a trace crosses that boundary, because the
// slow part of creating an interview is rarely the request.
//
// These tests are about continuity rather than about the outbox. They assert
// the thing a human actually wants: one trace id, from the publisher's span to
// the handler's.

// recorder gives a tracer whose spans can be read back.
func recorder(t *testing.T) (oteltrace.Tracer, *tracetest.SpanRecorder) {
	t.Helper()
	spans := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(spans))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return provider.Tracer("test"), spans
}

func TestAPublishedEventCarriesTheTraceItWasPublishedIn(t *testing.T) {
	t.Parallel()
	tracer, _ := recorder(t)

	ctx, span := tracer.Start(context.Background(), "publish")
	defer span.End()

	carried := outbox.TraceContextOf(ctx)

	if carried.Parent == "" {
		t.Fatal("an event published inside a span carries no traceparent")
	}
	if !contains(carried.Parent, span.SpanContext().TraceID().String()) {
		t.Fatalf("the traceparent %q does not name the publishing trace %s",
			carried.Parent, span.SpanContext().TraceID())
	}
}

func TestPublishingOutsideATraceCarriesNothingRatherThanSomethingEmpty(t *testing.T) {
	t.Parallel()

	// A real state, not a missing value: plenty of work is published by a
	// migration or a backfill with no request behind it. Storing a
	// syntactically valid traceparent with a zero trace id would produce spans
	// that link to a trace nobody can find, which is worse than an honest null.
	carried := outbox.TraceContextOf(context.Background())

	if carried.Parent != "" {
		t.Fatalf("a publisher outside any trace produced %q", carried.Parent)
	}
}

func TestDeliveryContinuesThePublishersTraceRatherThanStartingItsOwn(t *testing.T) {
	t.Parallel()
	tracer, spans := recorder(t)

	// The publisher, whose trace must survive the queue.
	ctx, publishing := tracer.Start(context.Background(), "publish")
	carried := outbox.TraceContextOf(ctx)
	publishing.End()

	// The worker, later, with nothing but the row.
	delivery := outbox.ContextFromTrace(context.Background(), carried)
	_, handling := tracer.Start(delivery, "deliver")
	handling.End()

	recorded := spans.Ended()
	if len(recorded) != 2 {
		t.Fatalf("want two spans, got %d", len(recorded))
	}
	published, delivered := recorded[0], recorded[1]
	if published.SpanContext().TraceID() != delivered.SpanContext().TraceID() {
		t.Fatalf("the trace broke across the queue: published in %s, delivered in %s",
			published.SpanContext().TraceID(), delivered.SpanContext().TraceID())
	}
	if delivered.Parent().SpanID() != published.SpanContext().SpanID() {
		t.Fatal("the delivery span is in the right trace but is not a child of the publisher")
	}
}

func TestAnAbsentTraceContextStartsAFreshTraceRatherThanFailing(t *testing.T) {
	t.Parallel()
	tracer, spans := recorder(t)

	// Rows published before this column existed have no traceparent, and so
	// does anything published outside a request. Delivery must still be traced,
	// just not joined to something that does not exist.
	delivery := outbox.ContextFromTrace(context.Background(), outbox.TraceContext{})
	_, handling := tracer.Start(delivery, "deliver")
	handling.End()

	recorded := spans.Ended()
	if len(recorded) != 1 {
		t.Fatalf("want one span, got %d", len(recorded))
	}
	if recorded[0].Parent().IsValid() {
		t.Fatal("a delivery with no carried context invented a parent")
	}
	if !recorded[0].SpanContext().TraceID().IsValid() {
		t.Fatal("a delivery with no carried context was not traced at all")
	}
}

func TestAMalformedTraceParentIsIgnoredRatherThanTrusted(t *testing.T) {
	t.Parallel()
	tracer, spans := recorder(t)

	// The column is data, and data arrives wrong. A traceparent that does not
	// parse must not become a parent, because the alternative is a span
	// pointing at a trace that cannot exist.
	delivery := outbox.ContextFromTrace(context.Background(),
		outbox.TraceContext{Parent: "not-a-traceparent"})
	_, handling := tracer.Start(delivery, "deliver")
	handling.End()

	if spans.Ended()[0].Parent().IsValid() {
		t.Fatal("a malformed traceparent was accepted as a parent")
	}
}

func TestTheCarriedContextIsTheStandardOneRatherThanOurOwn(t *testing.T) {
	t.Parallel()
	tracer, _ := recorder(t)

	// The whole point of W3C trace context is that the Python plane and any
	// provider can read it without agreeing with us first, so this asserts the
	// standard propagator round-trips what we store.
	ctx, span := tracer.Start(context.Background(), "publish")
	defer span.End()
	carried := outbox.TraceContextOf(ctx)

	restored := propagation.TraceContext{}.Extract(context.Background(),
		propagation.MapCarrier{"traceparent": carried.Parent})

	if oteltrace.SpanContextFromContext(restored).TraceID() != span.SpanContext().TraceID() {
		t.Fatal("what we store is not what the standard propagator reads")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && stringIndex(haystack, needle) >= 0
}

func stringIndex(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
