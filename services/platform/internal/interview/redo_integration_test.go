//go:build integration

package interview_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
)

// PRC-03 against real PostgreSQL: the original survives a redo intact,
// one retake per question, practice with results only.

// reviewedSession walks a session through completion and on to
// review_ready, the state a redo may be asked from.
func reviewedSession(t *testing.T) interview.Session {
	t.Helper()
	ctx := context.Background()
	store := interview.NewStore(pool)
	completer := interview.NewCompleter(store)
	session := runningSession(t)
	if _, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 5); err != nil {
		t.Fatalf("complete: %v", err)
	}
	evaluating, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	reviewed, err := store.Transition(ctx, evaluating, interview.StateReviewReady, interview.Effects{},
		interview.Actor{ID: candidateID, Type: "service"})
	if err != nil {
		t.Fatalf("to review_ready: %v", err)
	}
	return reviewed
}

func TestARedoLeavesTheOriginalIntactAndLinksToIt(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	parent := reviewedSession(t)
	before, err := events.AssembleTranscript(ctx, parent.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("transcript: %v", err)
	}
	beforeSeal, _ := interview.NewCompleter(store).SealOf(ctx, parent.ID, "practice", candidateID, "")

	child, err := store.CreateRedo(ctx, events, parent, 3, candidate)
	if err != nil {
		t.Fatalf("redo: %v", err)
	}

	// The retake is its own session with its origin in its config.
	if child.ID == parent.ID || child.Mode != "practice" || child.State != interview.StateDraft {
		t.Fatalf("child = %+v", child)
	}
	var config struct {
		Minutes int              `json:"minutes"`
		RedoOf  interview.RedoOf `json:"redo_of"`
	}
	if err := json.Unmarshal(child.Config, &config); err != nil {
		t.Fatalf("config: %v", err)
	}
	if config.RedoOf.SessionID != parent.ID || config.RedoOf.Sequence != 3 || config.RedoOf.Question == "" || config.Minutes != interview.RedoMinutes {
		t.Fatalf("redo_of = %+v", config)
	}

	// The original: transcript, seal, state, all exactly as before.
	after, _ := events.AssembleTranscript(ctx, parent.ID, "practice", candidateID, "")
	if !reflect.DeepEqual(before, after) {
		t.Fatal("the original transcript changed")
	}
	afterSeal, _ := interview.NewCompleter(store).SealOf(ctx, parent.ID, "practice", candidateID, "")
	if !reflect.DeepEqual(beforeSeal, afterSeal) {
		t.Fatal("the original seal changed")
	}
	reread, _ := store.Get(ctx, parent.ID, "practice", candidateID, "")
	if reread.State != interview.StateReviewReady || !reflect.DeepEqual(reread.Config, parent.Config) {
		t.Fatalf("the original session changed: %+v", reread)
	}

	redos, err := store.Redos(ctx, parent)
	if err != nil {
		t.Fatalf("redos: %v", err)
	}
	if len(redos) != 1 || redos[0].Sequence != 3 || redos[0].RedoSessionID != child.ID {
		t.Fatalf("redos = %+v", redos)
	}
}

func TestOneRetakePerQuestion(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)
	parent := reviewedSession(t)
	if _, err := store.CreateRedo(ctx, events, parent, 3, candidate); err != nil {
		t.Fatalf("first redo: %v", err)
	}
	if _, err := store.CreateRedo(ctx, events, parent, 3, candidate); !errors.Is(err, interview.ErrRedoExists) {
		t.Fatalf("second redo = %v, want ErrRedoExists", err)
	}
}

func TestARedoNeedsResultsAndAnAnswerOfTheCandidates(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	events := interview.NewEvents(store)

	running := runningSession(t)
	if _, err := store.CreateRedo(ctx, events, running, 3, candidate); !errors.Is(err, interview.ErrRedoNotAllowed) {
		t.Fatalf("redo before results = %v, want ErrRedoNotAllowed", err)
	}

	parent := reviewedSession(t)
	if _, err := store.CreateRedo(ctx, events, parent, 2, candidate); !errors.Is(err, interview.ErrRedoTurnUnknown) {
		t.Fatalf("redo of the interviewer's turn = %v, want ErrRedoTurnUnknown", err)
	}

	screening := parent
	screening.Mode = "screening"
	if _, err := store.CreateRedo(ctx, events, screening, 3, candidate); !errors.Is(err, interview.ErrRedoNotAllowed) {
		t.Fatalf("screening redo = %v, want ErrRedoNotAllowed", err)
	}
}
