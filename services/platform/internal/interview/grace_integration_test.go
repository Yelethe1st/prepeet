//go:build integration

package interview_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
)

// SES-06's grace expiry against real PostgreSQL: a reconnection window that
// lapses finalizes what was captured and records why, and an expiry that
// arrives late - after a resume, or before its time - touches nothing.

func graceInputFor(session interview.Session) interview.GraceInput {
	return interview.GraceInput{
		SessionID: session.ID, Mode: "practice", CandidateID: candidateID,
		ActorID: candidateID,
	}
}

// ageDrop backdates the reconnecting transition, as the migrator: the clock
// is the one input a test may not wait for.
func ageDrop(t *testing.T, sessionID string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx,
		`UPDATE interview.sessions SET state_changed_at = now() - interval '10 minutes' WHERE id = $1`,
		sessionID); err != nil {
		t.Fatalf("aging the drop: %v", err)
	}
}

func TestExpiredGraceFinalizesWithTheInterruptionRecorded(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	completer := interview.NewCompleter(store)
	session := inProgressPractice(t)

	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(2, "connection.lost")}); err != nil {
		t.Fatalf("dropping: %v", err)
	}
	ageDrop(t, session.ID)

	activities := interview.NewGraceActivities(store, completer)
	check, err := activities.RemainingGrace(ctx, graceInputFor(session))
	if err != nil {
		t.Fatalf("remaining grace: %v", err)
	}
	if !check.Reconnecting || check.RemainingSeconds != 0 {
		t.Fatalf("check = %+v, want reconnecting and due", check)
	}
	if err := activities.ExpireGrace(ctx, graceInputFor(session)); err != nil {
		t.Fatalf("expire: %v", err)
	}

	// Why is recorded: the fact names grace_expired, the epoch it befell,
	// and how long the candidate had been gone when the window closed.
	facts, err := store.Interruptions(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("reading interruptions: %v", err)
	}
	if len(facts) != 1 || facts[0].Cause != interview.CauseGraceExpired {
		t.Fatalf("facts = %+v, want one grace_expired", facts)
	}
	if facts[0].ConnectionEpoch != 1 || facts[0].DurationSeconds < 590 {
		t.Fatalf("fact = %+v, want epoch 1 and roughly the ten minutes aged", facts[0])
	}

	// Cleanly finalized: sealed at the cursor the record actually holds,
	// moved through finalizing into evaluating by the same completion path
	// a candidate's own completes through.
	receipt, err := completer.SealOf(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("receipt: %v", err)
	}
	if receipt.SealedEpoch != 1 || receipt.SealedSequence != 2 {
		t.Fatalf("sealed at epoch %d seq %d, want 1/2", receipt.SealedEpoch, receipt.SealedSequence)
	}
	current, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if current.State != interview.StateEvaluating {
		t.Fatalf("state = %s, want evaluating", current.State)
	}

	// The completion announcement says how it ended: expired, not completed.
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer conn.Close(ctx)
	var completion string
	if err := conn.QueryRow(ctx, `
		SELECT payload->>'completion' FROM integration.outbox
		WHERE event_type = 'interview.session_completed.v1'
		  AND payload->>'session_id' = $1`, session.ID).Scan(&completion); err != nil {
		t.Fatalf("outbox read: %v", err)
	}
	if completion != "expired" {
		t.Fatalf("completion = %q, want expired", completion)
	}

	// A second expiry converges: no second fact, no second seal, no error.
	if err := activities.ExpireGrace(ctx, graceInputFor(session)); err != nil {
		t.Fatalf("second expire: %v", err)
	}
	if facts, err = store.Interruptions(ctx, session.ID, "practice", candidateID, ""); err != nil || len(facts) != 1 {
		t.Fatalf("facts after retry = %d (err %v), want still 1", len(facts), err)
	}
}

func TestExpiryStandsDownWhenResumedOrNotYetDue(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	activities := interview.NewGraceActivities(store, interview.NewCompleter(store))

	// Not yet due: the window is still open, and the check says how long.
	waiting := inProgressPractice(t)
	if _, err := events.Ingest(ctx, waiting.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(2, "connection.lost")}); err != nil {
		t.Fatalf("dropping: %v", err)
	}
	check, err := activities.RemainingGrace(ctx, graceInputFor(waiting))
	if err != nil {
		t.Fatalf("remaining grace: %v", err)
	}
	if !check.Reconnecting || check.RemainingSeconds <= 0 {
		t.Fatalf("check = %+v, want reconnecting with time left", check)
	}
	if err := activities.ExpireGrace(ctx, graceInputFor(waiting)); err != nil {
		t.Fatalf("early expire: %v", err)
	}
	if current, _ := store.Get(ctx, waiting.ID, "practice", candidateID, ""); current.State != interview.StateReconnecting {
		t.Fatalf("state = %s: an expiry before its time must touch nothing", current.State)
	}

	// Recovered: the session resumed before the timer fired.
	recovered := inProgressPractice(t)
	if _, err := events.Ingest(ctx, recovered.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(2, "connection.lost")}); err != nil {
		t.Fatalf("dropping: %v", err)
	}
	if _, err := events.Ingest(ctx, recovered.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(3, "connection.resumed")}); err != nil {
		t.Fatalf("recovering: %v", err)
	}
	check, err = activities.RemainingGrace(ctx, graceInputFor(recovered))
	if err != nil {
		t.Fatalf("remaining grace: %v", err)
	}
	if check.Reconnecting {
		t.Fatalf("check = %+v: a recovered session has no window to wait", check)
	}
	if err := activities.ExpireGrace(ctx, graceInputFor(recovered)); err != nil {
		t.Fatalf("expire on recovered: %v", err)
	}
	if current, _ := store.Get(ctx, recovered.ID, "practice", candidateID, ""); current.State != interview.StateInProgress {
		t.Fatalf("state = %s: the interview goes on", current.State)
	}
	if facts, _ := store.Interruptions(ctx, recovered.ID, "practice", candidateID, ""); len(facts) != 0 {
		t.Fatalf("facts = %+v: nothing expired, nothing recorded here", facts)
	}
}
