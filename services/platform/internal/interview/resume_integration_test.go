//go:build integration

package interview_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// SES-06's resume against real PostgreSQL: a dropped session continues as
// itself. The next epoch opens with the recovery cursor the browser rebuilds
// on, the superseded tab is refused by name, the grace window is enforced
// from the stamped policy, and a session with no interview in flight cannot
// be resumed into one.

func newResumer() *interview.Resumer {
	return interview.NewResumer(interview.NewStore(pool), &grantRecorder{})
}

func TestResumeOpensTheNextEpochWithTheRecoveryCursor(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	session := inProgressPractice(t)

	// The record so far: 1 and 2 contiguous, 4 stored beyond a gap at 3.
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(2, "turn.boundary"), event(4, "preference.captions")}); err != nil {
		t.Fatalf("seeding the timeline: %v", err)
	}

	// A refresh: the server never saw a drop, the tab is simply gone.
	resumed, err := newResumer().Resume(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if resumed.Session.ConnectionEpoch != 2 {
		t.Fatalf("epoch = %d, want 2", resumed.Session.ConnectionEpoch)
	}
	if resumed.PreviousEpoch != 1 || resumed.AcceptedSequence != 2 {
		t.Fatalf("recovery cursor = epoch %d seq %d, want epoch 1 seq 2",
			resumed.PreviousEpoch, resumed.AcceptedSequence)
	}
	if len(resumed.Missing) != 1 || resumed.Missing[0].From != 3 || resumed.Missing[0].To != 3 {
		t.Fatalf("missing = %+v, want exactly [3,3]", resumed.Missing)
	}
	if resumed.Grant.Token == "" || resumed.Grant.Room != session.ID {
		t.Fatalf("grant = %+v: reconnection mints a fresh grant, scoped to this room", resumed.Grant)
	}
	if resumed.Timing.ReconnectGraceSeconds <= 0 {
		t.Fatalf("timing = %+v: the stamped policy rides the answer", resumed.Timing)
	}

	// The superseded tab is refused whole, by name: one live connection.
	stale, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(5, "turn.boundary")})
	if !errors.Is(err, interview.ErrEpochStale) {
		t.Fatalf("stale tab: ack %+v err %v, want EPOCH_STALE", stale, err)
	}
}

func TestResumeReturnsAReconnectingSessionWithinGrace(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	session := inProgressPractice(t)

	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(2, "connection.lost")}); err != nil {
		t.Fatalf("dropping: %v", err)
	}

	resumed, err := newResumer().Resume(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("resume within grace: %v", err)
	}

	// The session is still reconnecting until the new connection actually
	// establishes: resume authorizes recovery, it does not pretend one.
	if resumed.Session.State != interview.StateReconnecting {
		t.Fatalf("state after resume = %s, want reconnecting until established", resumed.Session.State)
	}
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", resumed.Session.ConnectionEpoch,
		[]interview.ControlEvent{{
			EventID: id.New().String(), Epoch: resumed.Session.ConnectionEpoch, Sequence: 1,
			Type: "connection.established", OccurredAt: time.Date(2026, 8, 26, 12, 6, 0, 0, time.UTC),
		}}); err != nil {
		t.Fatalf("establishing the recovery: %v", err)
	}
	current, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if current.State != interview.StateInProgress {
		t.Fatalf("state = %s, want in_progress: the interview resumed", current.State)
	}
}

func TestResumeRefusesAfterGraceExpiry(t *testing.T) {
	ctx := context.Background()
	events := interview.NewEvents(interview.NewStore(pool))
	session := inProgressPractice(t)

	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(2, "connection.lost")}); err != nil {
		t.Fatalf("dropping: %v", err)
	}

	// Age the drop past the stamped window (seeded v1 grants 120s), as the
	// migrator: the clock is the one input a test may not wait for.
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx,
		`UPDATE interview.sessions SET state_changed_at = now() - interval '10 minutes' WHERE id = $1`,
		session.ID); err != nil {
		t.Fatalf("aging the drop: %v", err)
	}

	_, err = newResumer().Resume(ctx, session.ID, "practice", candidateID, "")
	if !errors.Is(err, interview.ErrResumeGraceExpired) {
		t.Fatalf("resume after expiry: %v, want ErrResumeGraceExpired", err)
	}
}

func TestResumeHealsAConnectingSessionWithoutAnAttempt(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)

	// The crash start.go names: the transition to connecting committed and
	// the process died before the attempt opened. No epoch exists.
	ready := readySession(t)
	connecting, err := store.Transition(ctx, ready, interview.StateConnecting, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("to connecting: %v", err)
	}
	if connecting.ConnectionEpoch != 0 {
		t.Fatalf("epoch = %d, want 0 before any attempt", connecting.ConnectionEpoch)
	}

	resumed, err := newResumer().Resume(ctx, connecting.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("healing resume: %v", err)
	}
	if resumed.Session.ConnectionEpoch != 1 {
		t.Fatalf("epoch = %d, want 1: the heal opens the first attempt", resumed.Session.ConnectionEpoch)
	}
	if resumed.PreviousEpoch != 0 || resumed.AcceptedSequence != 0 || len(resumed.Missing) != 0 {
		t.Fatalf("recovery = %+v, want an empty history", resumed)
	}
}

func TestResumeRefusesASessionWithNoInterviewInFlight(t *testing.T) {
	ctx := context.Background()

	// Ready: nothing started, nothing to resume. Start is the front door.
	ready := readySession(t)
	if _, err := newResumer().Resume(ctx, ready.ID, "practice", candidateID, ""); !errors.Is(err, interview.ErrResumeNotResumable) {
		t.Fatalf("ready resume: %v, want ErrResumeNotResumable", err)
	}

	// Sealed and finalizing: the record is frozen; resuming would reopen it.
	events := interview.NewEvents(interview.NewStore(pool))
	completer := interview.NewCompleter(interview.NewStore(pool))
	session := inProgressPractice(t)
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(2, "turn.boundary")}); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	if _, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 2); err != nil {
		t.Fatalf("completing: %v", err)
	}
	if _, err := newResumer().Resume(ctx, session.ID, "practice", candidateID, ""); !errors.Is(err, interview.ErrResumeNotResumable) {
		t.Fatalf("sealed resume: %v, want ErrResumeNotResumable", err)
	}

	// Somebody else's session is not found, never refused: existence is not
	// answered across owners.
	other := inProgressPractice(t)
	if _, err := newResumer().Resume(ctx, other.ID, "practice", id.New().String(), ""); !errors.Is(err, interview.ErrNotFound) {
		t.Fatalf("stranger resume: %v, want ErrNotFound", err)
	}
}
