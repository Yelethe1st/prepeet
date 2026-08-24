package broadcast_test

import (
	"context"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/broadcast"
)

// Memory satisfies the same contract as every other broadcaster.
func TestMemorySatisfiesTheBroadcasterContract(t *testing.T) {
	t.Parallel()

	runBroadcasterContract(t, func(t *testing.T) broadcast.Broadcaster {
		return broadcast.NewMemory()
	})
}

// What follows is specific to fanning out inside one process.

// A process that subscribes per interview session must not accumulate one
// entry per session it has ever served.
func TestMemoryForgetsClosedSubscriptions(t *testing.T) {
	t.Parallel()

	bus := broadcast.NewMemory()
	const topic = "session_01a03"

	first, err := bus.Subscribe(context.Background(), topic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	second, err := bus.Subscribe(context.Background(), topic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if got := bus.Subscribers(topic); got != 2 {
		t.Fatalf("Subscribers = %d, want 2", got)
	}

	_ = first.Close()
	if got := bus.Subscribers(topic); got != 1 {
		t.Errorf("Subscribers = %d after one close, want 1", got)
	}

	_ = second.Close()
	if got := bus.Subscribers(topic); got != 0 {
		t.Errorf("Subscribers = %d after both closed, want 0", got)
	}
}

// Concurrent publishing and subscribing is the ordinary case, not an edge one:
// several requests publish while several sockets open and close. Run under the
// race detector, this asserts the registry is actually guarded.
func TestMemoryIsSafeUnderConcurrency(t *testing.T) {
	t.Parallel()

	bus := broadcast.NewMemory()
	const topic = "session_concurrent"

	done := make(chan struct{})
	for range 8 {
		go func() {
			for range 50 {
				subscription, err := bus.Subscribe(context.Background(), topic)
				if err != nil {
					t.Error(err)
					break
				}
				_ = bus.Publish(context.Background(), topic, []byte("tick"))
				_ = subscription.Close()
			}
			done <- struct{}{}
		}()
	}
	for range 8 {
		<-done
	}

	if got := bus.Subscribers(topic); got != 0 {
		t.Errorf("Subscribers = %d after every subscription closed, want 0", got)
	}
}

// One subscriber falling behind must not cost the others their messages.
//
// The reader is kept in step with the publisher rather than racing it, because
// delivery is non-blocking for every subscriber alike: a reader outrun by a
// tight publish loop would drop messages too, and the assertion would then be
// about scheduling rather than about isolation between subscribers.
func TestMemoryDropsOnlyForTheSubscriberThatIsBehind(t *testing.T) {
	t.Parallel()

	bus := broadcast.NewMemory()
	const topic = "session_backpressure"

	stalled, err := bus.Subscribe(context.Background(), topic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = stalled.Close() }()

	reading, err := bus.Subscribe(context.Background(), topic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = reading.Close() }()

	// Well past the buffer, so the stalled subscriber is certainly dropping by
	// the end of this loop.
	const sent = 500
	for i := range sent {
		if err := bus.Publish(context.Background(), topic, []byte("x")); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		select {
		case <-reading.Messages():
		case <-time.After(arrival):
			t.Fatalf("message %d never reached the reading subscriber; a stalled peer cost it delivery", i)
		}
	}
}

// And the stalled subscriber's loss is bounded by the buffer rather than
// growing with what it missed, which is the property that stops a browser that
// stopped reading from becoming unbounded memory.
func TestMemoryBoundsWhatAStalledSubscriberHolds(t *testing.T) {
	t.Parallel()

	bus := broadcast.NewMemory()
	const topic = "session_bounded"

	stalled, err := bus.Subscribe(context.Background(), topic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = stalled.Close() }()

	const sent = 5000
	for range sent {
		if err := bus.Publish(context.Background(), topic, []byte("x")); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	held := len(stalled.Messages())
	if held >= sent {
		t.Errorf("a subscriber that never read holds %d of %d messages, so memory grows with what it missed",
			held, sent)
	}
}
