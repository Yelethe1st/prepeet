//go:build integration

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/cmd/wiring"
	"github.com/Yelethe1st/prepeet/services/platform/internal/operations"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// OPS-03's first criterion, end to end and against real PostgreSQL: the same
// failed item, retried twice by an operator, is delivered twice and evaluates
// once.
//
// The two deliveries are the point. A test where the retry is refused would
// prove only that the operator was stopped, and the criterion is stronger than
// that: it says a retry never duplicates the effect, including when delivery
// really does happen more than once. So the first delivery here fails after the
// workflow has been started, which is the crash between an effect and the
// record of it that at-least-once delivery exists to survive, and the second
// delivery then runs the same handler against a workflow that is already there.

// retryOperator is the acting person. It is one of the accounts TestMain seeds,
// because audit rows carry a foreign key to identity.users and an operator
// action has to be attributable to somebody real.
const retryOperator = e2eAuthor

// completedSessionID is the entity the workflow id is derived from.
const completedSessionID = "00000000-0000-7000-8000-0000000000b7"

// quiet keeps the dispatcher's ordinary logging out of the test output. The
// failures under test are asserted, not read.
var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// publishCompletion writes a session_completed event and returns its id.
func publishCompletion(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	eventID, err := outbox.New(pool).Publish(ctx, tx, outbox.Event{
		Type:          "interview.session_completed.v1",
		SchemaVersion: "1.0",
		Producer:      "interview",
		Purpose:       "practice",
		OccurredAt:    time.Now().UTC(),
		Actor:         outbox.Actor{Type: "service", ID: e2eReviewer},
		Payload: []byte(`{"session_id":"` + completedSessionID +
			`","completion":"completed","turn_count":8,"duration_seconds":600}`),
	})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return eventID
}

// exhaust drives an event to dead letter, which is how work reaches an
// operator.
func exhaust(t *testing.T, eventID string) {
	t.Helper()
	store := outbox.New(pool)

	for range outbox.MaxAttempts {
		if err := store.MarkFailed(context.Background(), eventID, "the evaluation worker was down"); err != nil {
			t.Fatalf("MarkFailed: %v", err)
		}
	}
	if !failed(t, eventID) {
		t.Fatalf("%d failures did not dead letter the event", outbox.MaxAttempts)
	}
}

// failed reports whether the event is waiting for an operator.
func failed(t *testing.T, eventID string) bool {
	t.Helper()

	var dead bool
	if err := pool.QueryRow(context.Background(),
		`SELECT dead_at IS NOT NULL FROM integration.outbox WHERE id = $1`, eventID).Scan(&dead); err != nil {
		t.Fatalf("reading dead_at: %v", err)
	}
	return dead
}

// waitFor polls until the condition holds, or fails the test.
//
// Polling rather than a sleep, because the dispatcher's timing is its own and a
// sleep long enough to be reliable is a sleep long enough to be slow.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func delivered(t *testing.T, eventID string) bool {
	t.Helper()

	var published bool
	if err := pool.QueryRow(context.Background(),
		`SELECT published_at IS NOT NULL FROM integration.outbox WHERE id = $1`, eventID).Scan(&published); err != nil {
		t.Fatalf("reading published_at: %v", err)
	}
	return published
}

func TestRetryingFailedWorkTwiceEvaluatesOnce(t *testing.T) {
	ctx := context.Background()
	store := outbox.New(pool)
	stub := newTemporalStub()

	// The production route, wrapped in the failure that produces a genuine
	// second delivery: the workflow is started, and then the step after it
	// fails. Production has exactly this shape, because the completed-session
	// route starts two workflows and either can fail after the other began.
	beginEvidence := startEvidence(stub)
	var attempts atomic.Int32
	attempted := make(chan int32, 4)
	route := outbox.HandlerFunc(func(ctx context.Context, event outbox.Pending) error {
		if err := beginEvidence(ctx, event); err != nil {
			return err
		}
		attempt := attempts.Add(1)
		attempted <- attempt
		if attempt == 1 {
			return errors.New("the follow-on step failed after the workflow had started")
		}
		return nil
	})

	eventID := publishCompletion(t)
	exhaust(t, eventID)

	console := operations.NewConsole(pool, wiring.NewBacklog(store))
	operator := operations.Operator{UserID: retryOperator, RequestID: "req-ops03-e2e"}

	// The first retry. The dispatcher is the real one and the path is the real
	// path: the console only moves the row, and delivery happens where it
	// always happens.
	firstRun, stopFirst := context.WithCancel(ctx)
	go outbox.NewDispatcher(store, routes(nil, nil, route, nil, nil, nil, nil), nil,
		outbox.DispatcherOptions{PollInterval: 50 * time.Millisecond, Logger: quiet}).Run(firstRun)

	if err := console.Retry(ctx, operator, eventID, "the evaluation worker is back"); err != nil {
		stopFirst()
		t.Fatalf("first Retry: %v", err)
	}
	select {
	case <-attempted:
	case <-time.After(20 * time.Second):
		stopFirst()
		t.Fatal("the recovered work was never delivered, so the retry moved nothing")
	}
	// The failed attempt has to be recorded before the dispatcher is stopped,
	// or the row is left claimed and the second half of the test is racing the
	// visibility window rather than testing anything.
	waitFor(t, "the failed attempt to be recorded", func() bool {
		var recorded int
		if err := pool.QueryRow(ctx,
			`SELECT attempts FROM integration.outbox WHERE id = $1`, eventID).Scan(&recorded); err != nil {
			t.Fatalf("reading attempts: %v", err)
		}
		return recorded > 0
	})
	stopFirst()

	if delivered(t, eventID) {
		t.Fatal("the event was marked delivered even though its handler failed")
	}

	// It fails its way back to an operator, and the operator retries it again.
	exhaust(t, eventID)

	secondRun, stopSecond := context.WithCancel(ctx)
	defer stopSecond()
	go outbox.NewDispatcher(store, routes(nil, nil, route, nil, nil, nil, nil), nil,
		outbox.DispatcherOptions{PollInterval: 50 * time.Millisecond, Logger: quiet}).Run(secondRun)

	if err := console.Retry(ctx, operator, eventID, "trying again after the follow-on step was fixed"); err != nil {
		t.Fatalf("second Retry: %v", err)
	}
	waitFor(t, "the second delivery", func() bool { return delivered(t, eventID) })

	// The whole point. Two operator retries, two real deliveries, one
	// evaluation: the second start collided with the first on a workflow id
	// derived from the session and was refused.
	if starts := stub.startsOf("evidence-" + completedSessionID); starts != 1 {
		t.Errorf("two deliveries started %d evaluations, want exactly one", starts)
	}
	if got := attempts.Load(); got != 2 {
		t.Errorf("the handler ran %d times, want the two deliveries this test set up", got)
	}

	// And both retries are in the trail, because an operator action that is not
	// recorded may as well not have been authorised.
	//
	// Read inside a transaction scoped to the operator, because audit.events
	// forces row-level security and an untenanted row belongs to the actor who
	// wrote it. Reading it any other way would prove the row exists somewhere
	// the operator cannot see it, which is not what auditing an action means.
	audit, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = audit.Rollback(ctx) }()
	if err := database.SetUser(ctx, audit, retryOperator); err != nil {
		t.Fatalf("scoping the audit read: %v", err)
	}

	var audited int
	if err := audit.QueryRow(ctx,
		`SELECT count(*) FROM audit.events
		 WHERE subject_type = 'outbox_event' AND subject_id = $1
		   AND action = 'operations.work_retried' AND outcome = 'allowed'`,
		eventID).Scan(&audited); err != nil {
		t.Fatalf("counting audit rows: %v", err)
	}
	if audited != 2 {
		t.Errorf("%d retries are audited, want both of them", audited)
	}
}

// The backlog the operator was looking at is the one the alert fires on: the
// same measurement, judged by the same budgets, so a screen and a pager cannot
// disagree about whether anything is wrong.
func TestDeadLetteredWorkBreachesTheBacklogAssessment(t *testing.T) {
	ctx := context.Background()

	eventID := publishCompletion(t)
	exhaust(t, eventID)

	assessment, err := operations.NewConsole(pool, wiring.NewBacklog(outbox.New(pool))).Backlog(ctx)
	if err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	if !assessment.FailedBreached {
		t.Errorf("work is dead lettered and the assessment reads healthy: %s", assessment.Summary())
	}
}
