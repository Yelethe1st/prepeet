package broadcast_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/broadcast"
)

// The behaviour every Broadcaster must have, written once.
//
// This file has no build tag, so it compiles for both the in-memory tests and
// the integration ones. Each implementation runs it and then adds only the
// assertions specific to it.
//
// It is written before a second implementation exists on purpose. ADR-0006
// defers Redis behind a trigger table whose most likely first entry is exactly
// this problem, fanning live interview progress across instances, and the
// reversibility argument in that ADR depends on the swap being cheap. A swap is
// only cheap if the replacement has something to prove itself against.

// The receive deadline for a message that should arrive. Generous, because the
// PostgreSQL implementation goes through a real connection, and a flaky test
// here would be worse than a slow one.
const arrival = 3 * time.Second

// The wait before concluding a message will not arrive. Short, because this
// path is taken by assertions that expect silence and there are several.
const silence = 250 * time.Millisecond

// newBroadcaster builds an implementation for one subtest.
type newBroadcaster func(t *testing.T) broadcast.Broadcaster

// runBroadcasterContract runs every assertion that holds for any Broadcaster.
//
// Topics are derived from the subtest name, so implementations that share a
// namespace, which PostgreSQL does, do not bleed between assertions.
func runBroadcasterContract(t *testing.T, build newBroadcaster) {
	t.Helper()

	t.Run("delivers a message to a subscriber", func(t *testing.T) {
		bus := build(t)
		topic := topicFor(t)

		subscription := subscribe(t, bus, topic)

		publish(t, bus, topic, []byte("evt_01a03"))

		if got := receive(t, subscription); !bytes.Equal(got.Payload, []byte("evt_01a03")) {
			t.Errorf("payload = %q, want %q", got.Payload, "evt_01a03")
		}
	})

	t.Run("names the topic on the message", func(t *testing.T) {
		bus := build(t)
		topic := topicFor(t)

		subscription := subscribe(t, bus, topic)
		publish(t, bus, topic, []byte("x"))

		if got := receive(t, subscription); got.Topic != topic {
			t.Errorf("Topic = %q, want %q: a subscriber holding several cannot tell them apart", got.Topic, topic)
		}
	})

	// Fan-out, not a queue. Every instance gets the message, which is the whole
	// reason this abstraction exists: a browser connected to task 2 must see
	// progress produced by task 1.
	t.Run("delivers to every subscriber rather than one of them", func(t *testing.T) {
		bus := build(t)
		topic := topicFor(t)

		subscriptions := []broadcast.Subscription{
			subscribe(t, bus, topic),
			subscribe(t, bus, topic),
			subscribe(t, bus, topic),
		}

		publish(t, bus, topic, []byte("progress"))

		for i, subscription := range subscriptions {
			if got := receive(t, subscription); !bytes.Equal(got.Payload, []byte("progress")) {
				t.Errorf("subscriber %d received %q, want %q", i, got.Payload, "progress")
			}
		}
	})

	t.Run("does not deliver across topics", func(t *testing.T) {
		bus := build(t)
		mine, theirs := topicFor(t), topicFor(t)+"_other"

		subscription := subscribe(t, bus, mine)

		publish(t, bus, theirs, []byte("not for me"))

		expectSilence(t, subscription, "a message published to another topic was delivered")
	})

	// The property everything else depends on, and the one worth failing loudly
	// over. This transport is best effort: nothing is stored, so a message
	// published while nobody was subscribed is gone rather than waiting.
	//
	// Anything that must not be lost goes through the outbox, which is durable.
	// Asserting the loss here is deliberate, so that a future implementation
	// which happens to buffer cannot tempt anyone into relying on delivery.
	//
	// The marker is what makes this deterministic, and the reason it is needed
	// is worth stating. An earlier version simply published and then subscribed,
	// and failed intermittently against PostgreSQL: the process is already
	// listening on a shared channel, so a message published moments before a
	// local subscribe can still be in flight and arrive just after it. That is
	// not a defect. It means the contract is "may or may not arrive", and a test
	// demanding non-delivery was asserting a guarantee in the opposite
	// direction, which is no more supportable than the one it was guarding
	// against.
	//
	// So this asserts the claim that is actually load bearing: nothing is
	// replayed. The marker on a second topic arrives after the first message, so
	// receiving it proves the first was already processed and dropped, and any
	// later delivery would have to be a replay.
	t.Run("does not replay a message published before the subscription", func(t *testing.T) {
		bus := build(t)
		topic, marker := topicFor(t), topicFor(t)+"_mark"

		watcher := subscribe(t, bus, marker)

		publish(t, bus, topic, []byte("too early"))
		publish(t, bus, marker, []byte("processed"))
		receive(t, watcher)

		subscription := subscribe(t, bus, topic)

		expectSilence(t, subscription, "a message published before subscribing was replayed to a "+
			"later subscriber, which would let a caller mistake this transport for a durable one")
	})

	t.Run("publishing with nobody listening is not an error", func(t *testing.T) {
		bus := build(t)

		if err := bus.Publish(context.Background(), topicFor(t), []byte("into the void")); err != nil {
			t.Errorf("Publish with no subscribers returned %v, and a producer must not depend on a consumer existing", err)
		}
	})

	t.Run("closing a subscription stops delivery and closes the channel", func(t *testing.T) {
		bus := build(t)
		topic := topicFor(t)

		subscription := subscribe(t, bus, topic)
		if err := subscription.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		publish(t, bus, topic, []byte("after close"))

		select {
		case message, open := <-subscription.Messages():
			if open {
				t.Errorf("a closed subscription delivered %q", message.Payload)
			}
		case <-time.After(arrival):
			t.Error("the channel of a closed subscription is still open, so a consumer ranging over it never returns")
		}
	})

	t.Run("closing twice is not an error", func(t *testing.T) {
		bus := build(t)

		subscription := subscribe(t, bus, topicFor(t))
		_ = subscription.Close()

		if err := subscription.Close(); err != nil {
			t.Errorf("the second Close returned %v; shutdown paths close things twice", err)
		}
	})

	t.Run("cancelling the subscription context ends it", func(t *testing.T) {
		bus := build(t)
		topic := topicFor(t)

		ctx, cancel := context.WithCancel(context.Background())
		subscription, err := bus.Subscribe(ctx, topic)
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		t.Cleanup(func() { _ = subscription.Close() })

		cancel()

		deadline := time.After(arrival)
		for {
			select {
			case _, open := <-subscription.Messages():
				if !open {
					return
				}
			case <-deadline:
				t.Fatal("cancelling the context left the subscription running, so a request scoped subscriber leaks")
			}
		}
	})

	// A subscriber that stops reading must not stop the publisher. The producer
	// here is a request handler committing a transaction, and blocking it
	// because a browser stopped reading would turn one slow client into an
	// outage.
	t.Run("a subscriber that stops reading does not block the publisher", func(t *testing.T) {
		bus := build(t)
		topic := topicFor(t)

		subscription := subscribe(t, bus, topic)
		_ = subscription // deliberately never read from

		done := make(chan error, 1)
		go func() {
			for i := range 200 {
				if err := bus.Publish(context.Background(), topic, fmt.Appendf(nil, "%d", i)); err != nil {
					done <- err
					return
				}
			}
			done <- nil
		}()

		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Publish returned %v while a subscriber was not reading", err)
			}
		case <-time.After(arrival):
			t.Fatal("a subscriber that stopped reading blocked the publisher, which makes one slow client an outage")
		}
	})

	t.Run("an empty payload is delivered", func(t *testing.T) {
		bus := build(t)
		topic := topicFor(t)

		subscription := subscribe(t, bus, topic)
		publish(t, bus, topic, nil)

		if got := receive(t, subscription); len(got.Payload) != 0 {
			t.Errorf("payload = %q, want empty: a signal carrying nothing is a valid signal", got.Payload)
		}
	})

	// The cap is the intersection of what every implementation can carry, not
	// what the current one can. PostgreSQL's NOTIFY has a hard ceiling and Redis
	// does not, so a payload accepted by one and refused by the other would make
	// the swap in ADR-0006 a breaking change discovered at runtime.
	t.Run("refuses a payload over the cap", func(t *testing.T) {
		bus := build(t)

		err := bus.Publish(context.Background(), topicFor(t), bytes.Repeat([]byte("x"), broadcast.MaxPayload+1))
		if !errors.Is(err, broadcast.ErrPayloadTooLarge) {
			t.Errorf("Publish of an oversized payload returned %v, want ErrPayloadTooLarge", err)
		}
	})

	t.Run("accepts a payload at exactly the cap", func(t *testing.T) {
		bus := build(t)
		topic := topicFor(t)

		subscription := subscribe(t, bus, topic)

		if err := bus.Publish(context.Background(), topic, bytes.Repeat([]byte("x"), broadcast.MaxPayload)); err != nil {
			t.Fatalf("Publish at exactly the cap returned %v", err)
		}
		if got := receive(t, subscription); len(got.Payload) != broadcast.MaxPayload {
			t.Errorf("payload length = %d, want %d", len(got.Payload), broadcast.MaxPayload)
		}
	})

	// A topic name reaches LISTEN, which cannot take a bound parameter. Every
	// implementation refuses the same names so the validation cannot be
	// something only the PostgreSQL one does.
	t.Run("refuses an unusable topic", func(t *testing.T) {
		bus := build(t)

		for _, topic := range []string{
			"",
			"Session_01A03",           // upper case: PostgreSQL folds it, so it stops round tripping
			"session-01a03",           // a hyphen is not an identifier character
			"session 01a03",           // a space is not either
			`session"; DROP TABLE x;`, // the reason this is validated rather than escaped
			"session\x00null",
			string(bytes.Repeat([]byte("s"), 64)), // over the identifier length limit
		} {
			if err := bus.Publish(context.Background(), topic, nil); !errors.Is(err, broadcast.ErrInvalidTopic) {
				t.Errorf("Publish to topic %q returned %v, want ErrInvalidTopic", topic, err)
			}
			if _, err := bus.Subscribe(context.Background(), topic); !errors.Is(err, broadcast.ErrInvalidTopic) {
				t.Errorf("Subscribe to topic %q returned %v, want ErrInvalidTopic", topic, err)
			}
		}
	})

	t.Run("accepts a topic of the shape this system uses", func(t *testing.T) {
		bus := build(t)

		for _, topic := range []string{
			"outbox_pending",
			"session_01a0301daa1070008f3e1234567890ab",
			"a",
		} {
			if err := bus.Publish(context.Background(), topic, nil); err != nil {
				t.Errorf("Publish to topic %q returned %v", topic, err)
			}
		}
	})
}

// ───────────────────────────────────────────────────────────────── helpers

// topicFor derives a topic from the running test, so assertions sharing a
// PostgreSQL database do not hear each other. Test names carry characters a
// topic may not, so they are folded rather than used directly.
func topicFor(t *testing.T) string {
	t.Helper()

	var folded []byte
	for _, r := range t.Name() {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			folded = append(folded, byte(r))
		case r >= 'A' && r <= 'Z':
			folded = append(folded, byte(r-'A'+'a'))
		default:
			folded = append(folded, '_')
		}
	}
	// Well under the 63 byte limit, because assertions append a suffix to a
	// derived topic and the truncation must leave room for it.
	if len(folded) > 40 {
		folded = folded[len(folded)-40:]
	}
	return "t_" + string(folded)
}

func subscribe(t *testing.T, bus broadcast.Broadcaster, topic string) broadcast.Subscription {
	t.Helper()

	subscription, err := bus.Subscribe(context.Background(), topic)
	if err != nil {
		t.Fatalf("Subscribe(%q): %v", topic, err)
	}
	t.Cleanup(func() { _ = subscription.Close() })
	return subscription
}

func publish(t *testing.T, bus broadcast.Broadcaster, topic string, payload []byte) {
	t.Helper()

	if err := bus.Publish(context.Background(), topic, payload); err != nil {
		t.Fatalf("Publish(%q): %v", topic, err)
	}
}

func receive(t *testing.T, subscription broadcast.Subscription) broadcast.Message {
	t.Helper()

	select {
	case message, open := <-subscription.Messages():
		if !open {
			t.Fatal("the subscription closed before a message arrived")
		}
		return message
	case <-time.After(arrival):
		t.Fatal("no message arrived within the deadline")
		return broadcast.Message{}
	}
}

func expectSilence(t *testing.T, subscription broadcast.Subscription, complaint string) {
	t.Helper()

	select {
	case message, open := <-subscription.Messages():
		if open {
			t.Errorf("%s: received %q", complaint, message.Payload)
		}
	case <-time.After(silence):
	}
}
