//go:build integration

package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// The storage half of OPS-03: what an operator is shown, and the two
// transitions they may make. Against real PostgreSQL, because every property
// here is a property of the UPDATE rather than of the Go around it - which row
// it matches, what it returns when it matches none, and whether a discarded row
// is still there afterwards.

// publish writes one committed event and returns its id.
func publish(t *testing.T, eventType string) string {
	t.Helper()
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	id, err := outbox.New(pool).Publish(ctx, tx, event(t, eventType))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return id
}

// killEvent exhausts an event's delivery attempts, which is how a real one
// arrives in front of an operator.
//
// Through MarkFailed rather than by setting dead_at directly, so the test's
// starting position is one production actually produces. A test that
// hand-crafts the state it wants proves the recovery path works on a state that
// may never occur.
func killEvent(t *testing.T, eventID string) {
	t.Helper()
	ctx := context.Background()
	store := outbox.New(pool)

	for range outbox.MaxAttempts {
		if err := store.MarkFailed(ctx, eventID, "the endpoint refused"); err != nil {
			t.Fatalf("MarkFailed: %v", err)
		}
	}

	var dead bool
	if err := pool.QueryRow(ctx,
		`SELECT dead_at IS NOT NULL FROM integration.outbox WHERE id = $1`, eventID).Scan(&dead); err != nil {
		t.Fatalf("reading dead_at: %v", err)
	}
	if !dead {
		t.Fatalf("%d failures did not dead letter the event, so the fixture is wrong", outbox.MaxAttempts)
	}
}

// inTransaction runs one operator transition the way the console does: inside a
// transaction of the caller's, so the transition and its audit row commit
// together.
func inTransaction(t *testing.T, act func(tx pgx.Tx) (bool, error)) bool {
	t.Helper()
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	changed, err := act(tx)
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return changed
}

func TestTheBacklogCountsWhatIsWaitingAndHowOldItIs(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	before, err := store.Backlog(ctx)
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	publish(t, string(probeA))
	publish(t, string(probeB))

	after, err := store.Backlog(ctx)
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	if after.Pending != before.Pending+2 {
		t.Errorf("pending went from %d to %d, want two more", before.Pending, after.Pending)
	}
	// Never negative, whatever the clocks are doing. occurred_at comes from the
	// publisher and now() from the database, and a negative age is below every
	// threshold, so the alert would read healthy at exactly the wrong moment.
	if after.OldestPending < 0 {
		t.Errorf("the oldest pending age is %s, which no threshold can be compared against", after.OldestPending)
	}
}

// The age the alert fires on is the age of the fact, not of the row. An event
// published carrying an older occurred_at has already been waiting that long,
// and reporting it as new would hide precisely the backlog worth alerting on.
func TestTheBacklogAgeIsMeasuredFromWhenTheFactOccurred(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	aged := event(t, string(probeA))
	aged.OccurredAt = time.Now().UTC().Add(-10 * time.Minute)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := store.Publish(ctx, tx, aged); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}

	backlog, err := store.Backlog(ctx)
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	if backlog.OldestPending < 9*time.Minute {
		t.Errorf("OldestPending = %s for work published ten minutes late, want roughly ten minutes",
			backlog.OldestPending)
	}
}

// Dead-lettered work counts separately, and stops counting when it is dealt
// with. A backlog that keeps counting work an operator has already decided
// about is an alert that cannot be cleared.
func TestFailedWorkIsCountedUntilItIsDealtWith(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	before, err := store.Backlog(ctx)
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	eventID := publish(t, string(probeA))
	killEvent(t, eventID)

	dead, err := store.Backlog(ctx)
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	if dead.Failed != before.Failed+1 {
		t.Errorf("failed went from %d to %d, want one more", before.Failed, dead.Failed)
	}

	if !inTransaction(t, func(tx pgx.Tx) (bool, error) {
		return store.Discard(ctx, tx, eventID, "the destination was decommissioned")
	}) {
		t.Fatal("discarding failed work reported no change")
	}

	after, err := store.Backlog(ctx)
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	if after.Failed != before.Failed {
		t.Errorf("failed is %d after the discard, want the %d it started at", after.Failed, before.Failed)
	}
}

func TestFailedWorkIsListedWithWhatAnOperatorNeeds(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	eventID := publish(t, string(probeC))
	killEvent(t, eventID)

	failed, err := store.FailedEvents(ctx, 50)
	if err != nil {
		t.Fatalf("FailedEvents: %v", err)
	}

	var found bool
	for _, item := range failed {
		if item.ID != eventID {
			continue
		}
		found = true
		if item.Type != string(probeC) {
			t.Errorf("Type = %q, want %q", item.Type, probeC)
		}
		if item.Attempts != outbox.MaxAttempts {
			t.Errorf("Attempts = %d, want %d", item.Attempts, outbox.MaxAttempts)
		}
		if item.LastError == "" {
			t.Error("the failure has no reason, so an operator cannot tell whether retrying would help")
		}
		if item.DeadAt.IsZero() {
			t.Error("the failure has no time, so an operator cannot tell what else broke with it")
		}
	}
	if !found {
		t.Fatalf("%s is dead lettered but was not listed", eventID)
	}
}

// The property the whole retry path rests on: the transition happens once. A
// second retry of the same item matches no row and says so, which is what stops
// two operators working the same queue from delivering the work twice.
func TestRecoveringTheSameItemTwiceTransitionsItOnce(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	eventID := publish(t, string(probeD))
	killEvent(t, eventID)

	if !inTransaction(t, func(tx pgx.Tx) (bool, error) {
		return store.Recover(ctx, tx, eventID)
	}) {
		t.Fatal("the first recovery reported no change")
	}
	if inTransaction(t, func(tx pgx.Tx) (bool, error) {
		return store.Recover(ctx, tx, eventID)
	}) {
		t.Error("the second recovery reported a change, so retrying twice would deliver twice")
	}
}

func TestRecoveredWorkIsDeliverableAgainWithAFreshBudget(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	eventID := publish(t, string(probeD))
	killEvent(t, eventID)
	if !inTransaction(t, func(tx pgx.Tx) (bool, error) {
		return store.Recover(ctx, tx, eventID)
	}) {
		t.Fatal("recovery reported no change")
	}

	claimed, err := store.Claim(ctx, 200)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	var found bool
	for _, item := range claimed {
		if item.ID == eventID {
			found = true
			if item.Attempts != 0 {
				t.Errorf("Attempts = %d after an operator retry, want a fresh budget", item.Attempts)
			}
		}
	}
	if !found {
		t.Error("recovered work was not claimed, so the retry moved nothing")
	}
}

// Discarding is a state change and not a delete. The row has to stay: it is the
// only remaining answer to what happened to that notification, and a delete
// under forced row-level security would remove nothing while reporting success.
func TestDiscardedWorkStaysInTheTableAndOutOfTheQueue(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	eventID := publish(t, string(probeE))
	killEvent(t, eventID)
	if !inTransaction(t, func(tx pgx.Tx) (bool, error) {
		return store.Discard(ctx, tx, eventID, "superseded by a manual correction")
	}) {
		t.Fatal("the discard reported no change")
	}

	var reason string
	var discarded bool
	if err := pool.QueryRow(ctx,
		`SELECT coalesce(discard_reason, ''), discarded_at IS NOT NULL
		 FROM integration.outbox WHERE id = $1`, eventID).Scan(&reason, &discarded); err != nil {
		t.Fatalf("reading the discarded row: %v", err)
	}
	if !discarded {
		t.Error("the row is not marked discarded")
	}
	if reason != "superseded by a manual correction" {
		t.Errorf("discard_reason = %q, want the operator's reason", reason)
	}

	failed, err := store.FailedEvents(ctx, 200)
	if err != nil {
		t.Fatalf("FailedEvents: %v", err)
	}
	for _, item := range failed {
		if item.ID == eventID {
			t.Error("discarded work is still listed as needing an operator")
		}
	}
}

// Discarded work must not come back through the retry path. Recovering it would
// deliver an event somebody decided must never be delivered.
func TestDiscardedWorkCannotBeRecovered(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	eventID := publish(t, string(probeE))
	killEvent(t, eventID)
	if !inTransaction(t, func(tx pgx.Tx) (bool, error) {
		return store.Discard(ctx, tx, eventID, "duplicate of an earlier event")
	}) {
		t.Fatal("the discard reported no change")
	}

	if inTransaction(t, func(tx pgx.Tx) (bool, error) {
		return store.Recover(ctx, tx, eventID)
	}) {
		t.Error("discarded work was recovered, so a discard is not terminal")
	}
}

// Work that is still being retried is not an operator's to touch. It has not
// failed yet, and retrying it would only shorten its own backoff.
func TestWorkThatHasNotFailedCannotBeRecoveredOrDiscarded(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	eventID := publish(t, string(probeA))

	if inTransaction(t, func(tx pgx.Tx) (bool, error) {
		return store.Recover(ctx, tx, eventID)
	}) {
		t.Error("live work was recovered, so the guard on dead_at does nothing")
	}
	if inTransaction(t, func(tx pgx.Tx) (bool, error) {
		return store.Discard(ctx, tx, eventID, "no")
	}) {
		t.Error("live work was discarded, so an operator can drop work that was still being delivered")
	}
}

// An identifier that names nothing is an ordinary operator mistake, not a
// failure: the answer is the same as for work already dealt with, because in
// both cases there is nothing to transition.
func TestAnUnknownItemTransitionsNothing(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)

	const absent = "00000000-0000-7000-8000-00000000dead"
	if inTransaction(t, func(tx pgx.Tx) (bool, error) {
		return store.Recover(ctx, tx, absent)
	}) {
		t.Error("recovering an unknown item reported a change")
	}
}
