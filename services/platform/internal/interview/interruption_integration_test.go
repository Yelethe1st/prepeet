//go:build integration

package interview_test

// SES-06 and SCR-08: an interruption is recorded with its cause and duration,
// as a fact independent of the session's state, and read only by whoever may
// read the session.

import (
	"context"
	"errors"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// runningScreeningSession seeds a screening session and drives it to in_progress,
// the state an interruption can befall.
func runningScreeningSession(t *testing.T) interview.Session {
	t.Helper()
	ready := readyScreening(t)
	store := interview.NewStore(pool)
	ctx := context.Background()
	connecting, err := store.Transition(ctx, ready, interview.StateConnecting, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	inProgress, err := store.Transition(ctx, connecting, interview.StateInProgress, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("in progress: %v", err)
	}
	return inProgress
}

func TestAnInterruptionIsRecordedWithCauseAndDuration(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	session := runningScreeningSession(t)

	recorded, err := store.RecordInterruption(ctx, session, interview.CauseConnectionLost, 42, candidate)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if recorded.Cause != interview.CauseConnectionLost || recorded.DurationSeconds != 42 {
		t.Fatalf("recorded wrong: %+v", recorded)
	}

	// It reads back for the tenant that owns the screening session.
	interruptions, err := store.Interruptions(ctx, session.ID, "screening", candidateID, tenantID)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(interruptions) != 1 || interruptions[0].Cause != interview.CauseConnectionLost {
		t.Fatalf("read back %d: %+v", len(interruptions), interruptions)
	}

	// The candidate reads their own interruption untenanted.
	own, err := store.Interruptions(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("candidate read: %v", err)
	}
	if len(own) != 1 {
		t.Fatalf("candidate saw %d of their own interruptions", len(own))
	}
	// Another candidate sees none.
	other, err := store.Interruptions(ctx, session.ID, "practice", id.New().String(), "")
	if err != nil {
		t.Fatalf("other read: %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("another candidate saw %d interruptions", len(other))
	}
}

// An interruption cannot be recorded against a session that is not running: it
// would be inventing an event.
func TestRecordingRefusesANonRunningSession(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	ready := readyScreening(t) // ready, not started

	if _, err := store.RecordInterruption(ctx, ready, interview.CauseGraceExpired, 10, candidate); !errors.Is(err, interview.ErrNotInterruptible) {
		t.Fatalf("error = %v, want ErrNotInterruptible", err)
	}
}

// An unknown cause is refused before anything is written.
func TestRecordingRefusesAnUnknownCause(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	session := runningScreeningSession(t)

	if _, err := store.RecordInterruption(ctx, session, interview.InterruptionCause("gremlins"), 1, candidate); !errors.Is(err, interview.ErrUnknownCause) {
		t.Fatalf("error = %v, want ErrUnknownCause", err)
	}
}
