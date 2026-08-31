package outbox

import (
	"context"

	"go.opentelemetry.io/otel/propagation"
)

// TraceContext is the W3C trace context an event was published in.
//
// Stored as it travels rather than in an encoding of our own. The point of the
// standard is that the Python plane, and any provider we hand the context to,
// can read it without having agreed with us first.
//
// Both fields empty means the event was published outside any trace, which is
// a real state rather than a missing value: a backfill or a migration has no
// request behind it. Keeping that distinct from a zero-valued traceparent
// matters, because a syntactically valid traceparent with a zero trace id
// produces spans that link to a trace nobody can find.
type TraceContext struct {
	// Parent is the W3C traceparent header value.
	Parent string
	// State is the W3C tracestate. Vendor-specific, usually empty, carried
	// rather than dropped so a system that set it still sees it.
	State string
}

// propagator is the composite the process installs, addressed directly rather
// than through otel.GetTextMapPropagator so that these functions behave the
// same in a test that has not called telemetry.Setup. A propagator that
// silently became a noop would make the outbox drop trace context with nothing
// to show for it.
var propagator = propagation.TraceContext{}

// TraceContextOf captures the trace the caller is in, for storing with an event.
func TraceContextOf(ctx context.Context) TraceContext {
	carrier := propagation.MapCarrier{}
	propagator.Inject(ctx, carrier)
	// Inject writes nothing when there is no recording span, so an absent key
	// is how "published outside a trace" arrives, and it stays absent rather
	// than becoming an empty string that looks like a value.
	return TraceContext{Parent: carrier["traceparent"], State: carrier["tracestate"]}
}

// ContextFromTrace rejoins the trace an event was published in.
//
// The delivery becomes a child of the publisher rather than a linked root, so
// one trace covers the request and the work it caused. The messaging
// conventions allow either, and a link would be the better choice if producer
// and consumer were genuinely independent. Here they are one piece of work that
// happens to cross a queue, and the question a person asks of these traces is
// "what happened to that request", which two traces cannot answer without being
// joined by hand.
//
// A carrier that does not parse yields a context with no remote parent rather
// than an error. The column is data, data arrives wrong, and a span pointing at
// a trace that cannot exist is worse than a span with no parent.
func ContextFromTrace(ctx context.Context, carried TraceContext) context.Context {
	if carried.Parent == "" {
		return ctx
	}
	carrier := propagation.MapCarrier{"traceparent": carried.Parent}
	if carried.State != "" {
		carrier["tracestate"] = carried.State
	}
	return propagator.Extract(ctx, carrier)
}
