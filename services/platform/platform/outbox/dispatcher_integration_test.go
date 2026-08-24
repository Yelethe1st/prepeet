//go:build integration

// Dispatcher tests against real PostgreSQL.
//
// The loop behaviour is asserted without a database in dispatcher_test.go. What
// is here is what only a database can show: that two dispatchers never deliver
// the same event, and that the wakeup published inside a publishing transaction
// actually reaches a dispatcher in another process and only after the commit.
package outbox_test

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/broadcast"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

func loud() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// publishCommitted writes one event and commits it, returning its id.
func publishCommitted(t *testing.T, eventType string) string {
	t.Helper()

	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	id, err := outbox.New(pool).Publish(ctx, tx, event(t, eventType))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return id
}

// The guarantee ADR-0006 chose SKIP LOCKED for, asserted end to end through the
// dispatcher rather than only at the Claim call. Two dispatchers stand in for
// two ECS tasks; an event delivered twice is a candidate submitted twice to a
// tenant's ATS.
func TestTwoDispatchersNeverDeliverTheSameEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const events = 40
	published := make(map[string]bool, events)
	for range events {
		published[publishCommitted(t, "session.started.v1")] = true
	}

	var mu sync.Mutex
	deliveries := map[string]int{}
	// Which dispatcher delivered what. Without this the test would pass just as
	// happily if one dispatcher did all the work and the other three never
	// contended, which would prove nothing about SKIP LOCKED.
	byDispatcher := map[int]int{}

	handler := func(which int) outbox.Handler {
		return outbox.HandlerFunc(func(_ context.Context, e outbox.Pending) error {
			mu.Lock()
			deliveries[e.ID]++
			byDispatcher[which]++
			mu.Unlock()
			// A delivery that takes a moment widens the window in which another
			// dispatcher could claim the same row, which is the window this is
			// about. Without it the test can pass by being too fast to overlap.
			time.Sleep(5 * time.Millisecond)
			return nil
		})
	}

	store := outbox.New(pool)
	opts := outbox.DispatcherOptions{Batch: 5, PollInterval: 20 * time.Millisecond, Logger: loud()}

	const dispatchers = 4
	var running sync.WaitGroup
	for which := range dispatchers {
		running.Add(1)
		go func() {
			defer running.Done()
			_ = outbox.NewDispatcher(store, handler(which), nil, opts).Run(ctx)
		}()
	}

	deadline := time.Now().Add(60 * time.Second)
	for {
		mu.Lock()
		delivered := len(deliveries)
		mu.Unlock()
		if delivered >= events {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d events were delivered", delivered, events)
		}
		time.Sleep(50 * time.Millisecond)
	}

	cancel()
	running.Wait()

	mu.Lock()
	defer mu.Unlock()

	for id, count := range deliveries {
		if !published[id] {
			continue // an event from another test sharing the table
		}
		if count != 1 {
			t.Errorf("event %s was delivered %d times by four dispatchers", id, count)
		}
	}

	if len(byDispatcher) < 2 {
		t.Errorf("only %d of %d dispatchers delivered anything, so nothing contended and this "+
			"test says nothing about SKIP LOCKED", len(byDispatcher), dispatchers)
	}
}

// The transactional wakeup, which is the reason outbox.Publish emits pg_notify
// itself rather than going through platform/broadcast.
//
// The signal must not arrive before the commit, or a dispatcher woken by it
// finds nothing and the event then waits a full poll interval anyway, which is
// the opposite of what the wakeup is for.
func TestTheWakeupArrivesOnlyOnCommit(t *testing.T) {
	ctx := context.Background()

	bus, err := broadcast.NewPostgres(ctx, pool, loud())
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	defer func() { _ = bus.Close() }()

	subscription, err := bus.Subscribe(ctx, outbox.WakeupTopic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = subscription.Close() }()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	if _, err := outbox.New(pool).Publish(ctx, tx, event(t, "session.started.v1")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Nothing may have arrived yet: the row is not visible to anyone else, so a
	// dispatcher woken now would find nothing and go back to sleep.
	select {
	case message := <-subscription.Messages():
		t.Fatalf("a wakeup arrived before the commit: %q", message.Payload)
	case <-time.After(500 * time.Millisecond):
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	select {
	case <-subscription.Messages():
	case <-time.After(5 * time.Second):
		t.Fatal("no wakeup arrived after the commit, so every event waits a full poll interval")
	}
}

// A rolled back publish must not wake anybody. PostgreSQL drops the
// notification with the transaction, which is the guarantee no external
// transport can offer and the reason this one signal does not go through
// platform/broadcast.
func TestARolledBackPublishSendsNoWakeup(t *testing.T) {
	ctx := context.Background()

	bus, err := broadcast.NewPostgres(ctx, pool, loud())
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	defer func() { _ = bus.Close() }()

	subscription, err := bus.Subscribe(ctx, outbox.WakeupTopic)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = subscription.Close() }()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := outbox.New(pool).Publish(ctx, tx, event(t, "session.started.v1")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	select {
	case message := <-subscription.Messages():
		t.Errorf("a rolled back publish woke the dispatcher: %q", message.Payload)
	case <-time.After(time.Second):
	}
}

// End to end: publish, and a dispatcher in another process delivers it promptly
// without the poll interval elapsing.
func TestAnEventIsDeliveredPromptlyByTheWakeup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus, err := broadcast.NewPostgres(ctx, pool, loud())
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	defer func() { _ = bus.Close() }()

	delivered := make(chan string, 8)
	dispatcher := outbox.NewDispatcher(outbox.New(pool),
		outbox.HandlerFunc(func(_ context.Context, e outbox.Pending) error {
			select {
			case delivered <- e.ID:
			default:
			}
			return nil
		}), bus, outbox.DispatcherOptions{
			// Far longer than the deadline below, so an event arriving in time
			// can only have arrived because of the wakeup.
			PollInterval: 60 * time.Second,
			Logger:       loud(),
		})

	go func() { _ = dispatcher.Run(ctx) }()

	// Let the dispatcher make its first claim and subscribe before publishing,
	// since a signal sent before the subscription exists is legitimately lost.
	time.Sleep(500 * time.Millisecond)

	want := publishCommitted(t, "session.started.v1")

	deadline := time.After(15 * time.Second)
	for {
		select {
		case got := <-delivered:
			if got == want {
				return
			}
		case <-deadline:
			t.Fatal("the event was not delivered within the deadline, and the poll interval " +
				"is longer than that, so the wakeup did not reach the dispatcher")
		}
	}
}

// The dispatcher must drain what was published while it was not running.
// Anything else means a deploy loses every event published during the restart.
func TestABacklogPublishedBeforeStartupIsDrained(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const backlog = 12
	want := make(map[string]bool, backlog)
	for range backlog {
		want[publishCommitted(t, "session.started.v1")] = true
	}

	var mu sync.Mutex
	seen := map[string]bool{}

	dispatcher := outbox.NewDispatcher(outbox.New(pool),
		outbox.HandlerFunc(func(_ context.Context, e outbox.Pending) error {
			mu.Lock()
			seen[e.ID] = true
			mu.Unlock()
			return nil
		}), nil, outbox.DispatcherOptions{
			Batch: 5, PollInterval: 50 * time.Millisecond, Logger: loud(),
		})

	go func() { _ = dispatcher.Run(ctx) }()

	deadline := time.Now().Add(30 * time.Second)
	for {
		mu.Lock()
		remaining := 0
		for id := range want {
			if !seen[id] {
				remaining++
			}
		}
		mu.Unlock()

		if remaining == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d backlogged events were never delivered", remaining, backlog)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
