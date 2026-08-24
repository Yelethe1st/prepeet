package outbox_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

func typed(eventType string) outbox.Pending {
	return outbox.Pending{ID: "evt_1", Type: eventType, SchemaVersion: "1"}
}

func TestTheRouterDeliversToTheHandlerForTheType(t *testing.T) {
	t.Parallel()

	var got string
	router := outbox.NewRouter()
	router.Handle("session.started.v1", outbox.HandlerFunc(func(_ context.Context, e outbox.Pending) error {
		got = e.Type
		return nil
	}))
	router.Handle("session.completed.v1", outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		t.Error("the wrong handler was called")
		return nil
	}))

	if err := router.Deliver(context.Background(), typed("session.started.v1")); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got != "session.started.v1" {
		t.Errorf("handler saw %q", got)
	}
}

// The decision this type exists to force.
//
// An event type nobody has decided about is not silently delivered and not
// silently dropped. It fails, so it retries, and eventually dead letters, which
// is an operational fact somebody sees. The alternative, treating unknown as
// nothing to do, means the day somebody adds a producer and forgets the
// consumer, the events are marked delivered and are gone.
func TestAnUnknownEventTypeFailsRatherThanBeingDropped(t *testing.T) {
	t.Parallel()

	router := outbox.NewRouter()

	err := router.Deliver(context.Background(), typed("billing.invoiced.v1"))
	if !errors.Is(err, outbox.ErrNoRoute) {
		t.Fatalf("Deliver returned %v, want ErrNoRoute", err)
	}
	if !strings.Contains(err.Error(), "billing.invoiced.v1") {
		t.Errorf("the error does not name the unrouted type: %v", err)
	}
}

// And the escape hatch, which must be explicit. An event with no consumer is a
// legitimate state; it just has to be a decision rather than an omission.
func TestAnIgnoredEventTypeSucceeds(t *testing.T) {
	t.Parallel()

	router := outbox.NewRouter()
	router.Ignore("analytics.page_viewed.v1", "consumed from the read model, not the outbox")

	if err := router.Deliver(context.Background(), typed("analytics.page_viewed.v1")); err != nil {
		t.Errorf("Deliver of an explicitly ignored type returned %v", err)
	}
}

// A handler's error is the dispatcher's signal to retry, so it must survive
// routing intact rather than being replaced by a routing error.
func TestAHandlerErrorReachesTheCaller(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("the endpoint returned 503")

	router := outbox.NewRouter()
	router.Handle("session.started.v1", outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return sentinel
	}))

	if err := router.Deliver(context.Background(), typed("session.started.v1")); !errors.Is(err, sentinel) {
		t.Errorf("Deliver returned %v, want the handler's own error", err)
	}
}

// Registering the same type twice is a wiring mistake that would otherwise mean
// one consumer silently never runs.
func TestRegisteringATypeTwicePanics(t *testing.T) {
	t.Parallel()

	router := outbox.NewRouter()
	router.Handle("session.started.v1", outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return nil
	}))

	defer func() {
		if recover() == nil {
			t.Error("registering a type twice was accepted, so one of the two handlers never runs")
		}
	}()
	router.Handle("session.started.v1", outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return nil
	}))
}

func TestIgnoringATypeThatIsHandledPanics(t *testing.T) {
	t.Parallel()

	router := outbox.NewRouter()
	router.Handle("session.started.v1", outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return nil
	}))

	defer func() {
		if recover() == nil {
			t.Error("a type was both handled and ignored, which cannot be what anybody meant")
		}
	}()
	router.Ignore("session.started.v1", "because")
}

// Routes are registered at startup and read by every delivery, and several
// dispatchers deliver concurrently in one process.
func TestTheRouterIsSafeForConcurrentDelivery(t *testing.T) {
	t.Parallel()

	router := outbox.NewRouter()
	router.Handle("session.started.v1", outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return nil
	}))

	var running sync.WaitGroup
	for range 16 {
		running.Add(1)
		go func() {
			defer running.Done()
			for range 100 {
				if err := router.Deliver(context.Background(), typed("session.started.v1")); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	running.Wait()
}

// The wiring is worth being able to read back, so a review can ask what happens
// to a type rather than reading main.
func TestTheRouterReportsWhatItKnows(t *testing.T) {
	t.Parallel()

	router := outbox.NewRouter()
	router.Handle("session.started.v1", outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return nil
	}))
	router.Ignore("analytics.page_viewed.v1", "consumed from the read model")

	routes := router.Routes()
	if routes["session.started.v1"] != "handled" {
		t.Errorf("session.started.v1 is reported as %q, want handled", routes["session.started.v1"])
	}
	if !strings.Contains(routes["analytics.page_viewed.v1"], "read model") {
		t.Errorf("an ignored type does not report why: %q", routes["analytics.page_viewed.v1"])
	}
}
