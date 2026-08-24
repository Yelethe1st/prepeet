package outbox_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/platform/broadcast"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// The dispatcher's interesting behaviour is retry, panic containment, and when
// it waits rather than looks again. None of that is about SQL, so none of it is
// asserted against a database here: a test that needed PostgreSQL to check that
// a panicking handler does not stop delivery would be a test nobody runs on
// every change.
//
// What genuinely needs a database, concurrent claiming and the transactional
// wakeup, is in dispatcher_integration_test.go.

// fakeStore records what the dispatcher did, and hands out whatever the test
// wants claimed.
type fakeStore struct {
	mu sync.Mutex

	// batches are returned one per Claim, in order. An exhausted list returns
	// nothing, which is how a test lets the dispatcher settle.
	batches [][]outbox.Pending
	claims  int

	claimErr error

	delivered []string
	failures  map[string]string

	// claimed is signalled on every Claim, so a test can wait for the
	// dispatcher to have looked rather than sleeping and hoping.
	claimed chan struct{}
}

func newFakeStore(batches ...[]outbox.Pending) *fakeStore {
	return &fakeStore{
		batches:  batches,
		failures: map[string]string{},
		claimed:  make(chan struct{}, 256),
	}
}

func (f *fakeStore) Claim(_ context.Context, _ int) ([]outbox.Pending, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.claims++
	select {
	case f.claimed <- struct{}{}:
	default:
	}

	if f.claimErr != nil {
		return nil, f.claimErr
	}
	if len(f.batches) == 0 {
		return nil, nil
	}
	batch := f.batches[0]
	f.batches = f.batches[1:]
	return batch, nil
}

func (f *fakeStore) MarkDelivered(_ context.Context, eventID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.delivered = append(f.delivered, eventID)
	return nil
}

func (f *fakeStore) MarkFailed(_ context.Context, eventID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failures[eventID] = reason
	return nil
}

func (f *fakeStore) deliveredIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.delivered...)
}

func (f *fakeStore) failure(eventID string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	reason, ok := f.failures[eventID]
	return reason, ok
}

func (f *fakeStore) claimCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.claims
}

func (f *fakeStore) waitForClaims(t *testing.T, n int) {
	t.Helper()

	deadline := time.After(3 * time.Second)
	for range n {
		select {
		case <-f.claimed:
		case <-deadline:
			t.Fatalf("the dispatcher made %d claims, want at least %d", f.claimCount(), n)
		}
	}
}

func pending(id string) outbox.Pending {
	return outbox.Pending{ID: id, Type: "session.started.v1", SchemaVersion: "1"}
}

// silent keeps the expected error logging out of the test output.
func silent() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// run starts a dispatcher and stops it when the test ends.
func run(t *testing.T, store outbox.EventStore, handler outbox.Handler,
	wakeups broadcast.Broadcaster, opts outbox.DispatcherOptions) {
	t.Helper()

	if opts.Logger == nil {
		opts.Logger = silent()
	}

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan error, 1)
	go func() {
		stopped <- outbox.NewDispatcher(store, handler, wakeups, opts).Run(ctx)
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case err := <-stopped:
			if err != nil {
				t.Errorf("Run returned %v; cancellation is not a failure", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("Run did not return after its context was cancelled")
		}
	})
}

// The whole point. Before this existed, Publish wrote rows nothing read.
func TestEveryClaimedEventIsDelivered(t *testing.T) {
	t.Parallel()

	store := newFakeStore([]outbox.Pending{pending("a"), pending("b"), pending("c")})

	var mu sync.Mutex
	var seen []string
	run(t, store, outbox.HandlerFunc(func(_ context.Context, e outbox.Pending) error {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, e.ID)
		return nil
	}), nil, outbox.DispatcherOptions{PollInterval: 20 * time.Millisecond})

	store.waitForClaims(t, 2)

	mu.Lock()
	defer mu.Unlock()
	if strings.Join(seen, ",") != "a,b,c" {
		t.Errorf("handler saw %v, want a,b,c", seen)
	}
}

func TestASuccessfulDeliveryIsMarkedDelivered(t *testing.T) {
	t.Parallel()

	store := newFakeStore([]outbox.Pending{pending("a")})
	run(t, store, outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return nil
	}), nil, outbox.DispatcherOptions{PollInterval: 20 * time.Millisecond})

	store.waitForClaims(t, 3)

	if got := store.deliveredIDs(); len(got) != 1 || got[0] != "a" {
		t.Errorf("delivered = %v, want [a]; an unmarked event is redelivered forever", got)
	}
}

// A failure must be recorded rather than swallowed, or the event is marked
// delivered and nothing ever retries it.
func TestAFailedDeliveryIsRecordedWithItsReason(t *testing.T) {
	t.Parallel()

	store := newFakeStore([]outbox.Pending{pending("a")})
	run(t, store, outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return errors.New("the endpoint returned 503")
	}), nil, outbox.DispatcherOptions{PollInterval: 20 * time.Millisecond})

	store.waitForClaims(t, 3)

	reason, failed := store.failure("a")
	if !failed {
		t.Fatal("a failed delivery was not recorded, so the event will never be retried")
	}
	if !strings.Contains(reason, "503") {
		t.Errorf("reason = %q, and an operator needs to know what the endpoint said", reason)
	}
	if len(store.deliveredIDs()) != 0 {
		t.Error("a failed delivery was also marked delivered")
	}
}

// The reason reaches a database column an operator reads, so it is scrubbed the
// same as anything else carrying a provider's error text.
func TestAFailureReasonIsScrubbed(t *testing.T) {
	t.Parallel()

	store := newFakeStore([]outbox.Pending{pending("a")})
	run(t, store, outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return errors.New("dial postgres://prepeet:hunter2@db.internal:5432/prepeet: refused")
	}), nil, outbox.DispatcherOptions{PollInterval: 20 * time.Millisecond})

	store.waitForClaims(t, 3)

	reason, _ := store.failure("a")
	if strings.Contains(reason, "hunter2") {
		t.Errorf("the recorded reason carries a credential: %q", reason)
	}
}

// A handler is written by another context and talks to a provider. A panic in
// one must not stop delivery of everything else, which is what it would do if
// it unwound the dispatcher's loop.
func TestAPanickingHandlerBecomesAFailedAttempt(t *testing.T) {
	t.Parallel()

	store := newFakeStore([]outbox.Pending{pending("boom"), pending("fine")})
	run(t, store, outbox.HandlerFunc(func(_ context.Context, e outbox.Pending) error {
		if e.ID == "boom" {
			panic("nil client")
		}
		return nil
	}), nil, outbox.DispatcherOptions{PollInterval: 20 * time.Millisecond})

	store.waitForClaims(t, 3)

	reason, failed := store.failure("boom")
	if !failed {
		t.Fatal("a panicking handler was not recorded as a failed attempt")
	}
	if !strings.Contains(reason, "panic") {
		t.Errorf("reason = %q, want it to say the handler panicked", reason)
	}
	if got := store.deliveredIDs(); len(got) != 1 || got[0] != "fine" {
		t.Errorf("delivered = %v, want [fine]: one panicking event stopped the rest of the batch", got)
	}
}

// A panic message is free text nobody wrote with a classification in mind.
func TestAPanicReasonIsScrubbed(t *testing.T) {
	t.Parallel()

	store := newFakeStore([]outbox.Pending{pending("a")})
	run(t, store, outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		panic("cannot reach postgres://prepeet:hunter2@db.internal:5432/prepeet")
	}), nil, outbox.DispatcherOptions{PollInterval: 20 * time.Millisecond})

	store.waitForClaims(t, 3)

	if reason, _ := store.failure("a"); strings.Contains(reason, "hunter2") {
		t.Errorf("the recorded panic carries a credential: %q", reason)
	}
}

// A full batch means there is probably more. Waiting a poll interval between
// full batches drains a backlog at one batch per interval, which for a real
// backlog is hours.
func TestAFullBatchIsFollowedImmediately(t *testing.T) {
	t.Parallel()

	// Three full batches of two, then nothing.
	store := newFakeStore(
		[]outbox.Pending{pending("a"), pending("b")},
		[]outbox.Pending{pending("c"), pending("d")},
		[]outbox.Pending{pending("e"), pending("f")},
	)

	run(t, store, outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return nil
	}), nil, outbox.DispatcherOptions{
		Batch: 2,
		// Long enough that reaching the fourth claim proves the dispatcher did
		// not wait between full batches. Three waits would take a minute.
		PollInterval: 20 * time.Second,
	})

	store.waitForClaims(t, 4)

	if got := len(store.deliveredIDs()); got != 6 {
		t.Errorf("delivered %d events, want 6: the backlog drained one batch per poll interval", got)
	}
}

// And the converse, or the dispatcher is a busy loop against the database.
func TestAPartialBatchWaits(t *testing.T) {
	t.Parallel()

	store := newFakeStore([]outbox.Pending{pending("a")})

	run(t, store, outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return nil
	}), nil, outbox.DispatcherOptions{Batch: 10, PollInterval: 30 * time.Second})

	store.waitForClaims(t, 1)
	time.Sleep(200 * time.Millisecond)

	if got := store.claimCount(); got > 2 {
		t.Errorf("the dispatcher claimed %d times in 200ms with a 30s poll interval, "+
			"so it is spinning against the database", got)
	}
}

// The wakeup is what makes delivery prompt. Without it an event waits up to a
// poll interval, which is the difference between a webhook arriving now and
// arriving in five seconds.
func TestAWakeupCausesAnImmediateClaim(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	bus := broadcast.NewMemory()

	run(t, store, outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return nil
	}), bus, outbox.DispatcherOptions{PollInterval: 30 * time.Second})

	// Wait for the first claim, so the subscription is certainly open.
	store.waitForClaims(t, 1)
	before := store.claimCount()

	// Publish until one lands, since the dispatcher subscribes as it starts.
	deadline := time.After(3 * time.Second)
	for {
		if err := bus.Publish(context.Background(), outbox.WakeupTopic, nil); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		if store.claimCount() > before {
			return
		}
		select {
		case <-deadline:
			t.Fatal("a wakeup did not cause a claim, so every event waits a full poll interval")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Polling is the correctness backstop. A dispatcher that only ran when notified
// stops forever the first time a notification is missed, and notifications are
// missed routinely.
func TestTheDispatcherStillDeliversWithNoWakeupsAtAll(t *testing.T) {
	t.Parallel()

	store := newFakeStore([]outbox.Pending{pending("a")})

	run(t, store, outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return nil
	}), nil, outbox.DispatcherOptions{PollInterval: 20 * time.Millisecond})

	store.waitForClaims(t, 3)

	if got := store.deliveredIDs(); len(got) != 1 {
		t.Errorf("delivered = %v, want one event delivered by polling alone", got)
	}
}

// The database being briefly unavailable is a wait, not an exit. A process that
// exited here would be restarted by the orchestrator into the same condition.
func TestAClaimFailureDoesNotStopTheDispatcher(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	store.claimErr = errors.New("connection refused")

	run(t, store, outbox.HandlerFunc(func(context.Context, outbox.Pending) error {
		return nil
	}), nil, outbox.DispatcherOptions{PollInterval: 20 * time.Millisecond})

	store.waitForClaims(t, 3)

	store.mu.Lock()
	store.claimErr = nil
	store.batches = [][]outbox.Pending{{pending("a")}}
	store.mu.Unlock()

	deadline := time.After(3 * time.Second)
	for len(store.deliveredIDs()) == 0 {
		select {
		case <-deadline:
			t.Fatal("the dispatcher never recovered after the database came back")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// A handler that hangs must not hold its claim until the visibility window
// expires, delivering nothing else in the meantime.
func TestAHangingHandlerIsTimedOut(t *testing.T) {
	t.Parallel()

	store := newFakeStore([]outbox.Pending{pending("slow")})

	run(t, store, outbox.HandlerFunc(func(ctx context.Context, _ outbox.Pending) error {
		<-ctx.Done()
		return fmt.Errorf("gave up: %w", ctx.Err())
	}), nil, outbox.DispatcherOptions{
		PollInterval:    20 * time.Millisecond,
		DeliveryTimeout: 100 * time.Millisecond,
	})

	deadline := time.After(3 * time.Second)
	for {
		if _, failed := store.failure("slow"); failed {
			return
		}
		select {
		case <-deadline:
			t.Fatal("a hanging handler was never timed out, so one bad endpoint stalls the dispatcher")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
