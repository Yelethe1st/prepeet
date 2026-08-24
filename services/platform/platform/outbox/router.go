package outbox

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sync"
)

// ErrNoRoute means no decision has been recorded about an event type.
var ErrNoRoute = errors.New("outbox: no route for event type")

// Router sends each event to the handler registered for its type.
//
// Its real job is what it does with a type nobody registered: it fails.
//
// The tempting alternative is to treat an unknown type as nothing to do and
// mark it delivered. That is wrong in the direction that loses data. The day
// somebody adds a producer and forgets the consumer, every event of that type
// is marked delivered and is gone, and nothing about the system looks wrong.
// Failing instead means the events retry, then dead letter, which is a row in
// an operational view somebody is looking at.
//
// An event type with no consumer is still a legitimate state. It just has to be
// stated with Ignore rather than arrived at by omission.
type Router struct {
	// mu guards registration against delivery. Routes are registered at startup
	// and read on every delivery from several dispatcher goroutines, so the
	// unguarded map would be a race that only appears under load.
	mu       sync.RWMutex
	handlers map[string]Handler
	ignored  map[string]string
}

// NewRouter builds an empty router. Every event type fails until something is
// registered, which is the intended starting position.
func NewRouter() *Router {
	return &Router{
		handlers: map[string]Handler{},
		ignored:  map[string]string{},
	}
}

// Handle registers the handler for an event type.
//
// It panics on a duplicate registration rather than replacing or chaining.
// Either alternative means one of the two handlers silently never runs, and
// this is startup wiring, so a panic here is a failure to start rather than a
// failure in production.
func (r *Router) Handle(eventType string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.mustBeUnregistered(eventType)
	r.handlers[eventType] = handler
}

// Ignore records that an event type deliberately has no consumer, and why.
//
// The reason is required by the signature rather than by convention, because
// the reason is the entire value of the entry. "Ignored" without one is
// indistinguishable from an oversight six months later.
func (r *Router) Ignore(eventType, reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.mustBeUnregistered(eventType)
	r.ignored[eventType] = reason
}

// mustBeUnregistered panics if the type is already known. Callers hold the lock.
func (r *Router) mustBeUnregistered(eventType string) {
	if _, handled := r.handlers[eventType]; handled {
		panic(fmt.Sprintf("outbox: %q is already handled", eventType))
	}
	if _, ignored := r.ignored[eventType]; ignored {
		panic(fmt.Sprintf("outbox: %q is already ignored", eventType))
	}
}

// Deliver routes one event.
func (r *Router) Deliver(ctx context.Context, event Pending) error {
	r.mu.RLock()
	handler, handled := r.handlers[event.Type]
	_, ignored := r.ignored[event.Type]
	r.mu.RUnlock()

	switch {
	case handled:
		// The handler's error is returned unwrapped in the errors.Is sense, so
		// a caller can still recognise its own sentinel through the router.
		return handler.Deliver(ctx, event)
	case ignored:
		return nil
	default:
		return fmt.Errorf("%w: %s has no handler and is not explicitly ignored", ErrNoRoute, event.Type)
	}
}

// Routes reports what the router knows, for logging at startup.
//
// Wiring that only exists inside main is wiring nobody reviews. Printing it
// once at startup makes "what happens to this event type" answerable from a log
// line rather than from reading the composition root.
func (r *Router) Routes() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	routes := make(map[string]string, len(r.handlers)+len(r.ignored))
	for eventType := range maps.Keys(r.handlers) {
		routes[eventType] = "handled"
	}
	for eventType, reason := range r.ignored {
		routes[eventType] = "ignored: " + reason
	}
	return routes
}

var _ Handler = (*Router)(nil)
