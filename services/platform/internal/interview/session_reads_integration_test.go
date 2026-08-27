//go:build integration

package interview_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
)

// The reads and the sealed-input write that cmd depends on: the bundle a
// session was composed against, the owner's own history, whether a
// session is sealed, and the evaluation input document completion stores
// before the seal records its digest.

// fakeInputWriter records what completion wrote.
type fakeInputWriter struct {
	body []byte
	key  string
}

func (f *fakeInputWriter) PutSealedInput(_ context.Context, _ interview.Session, body []byte) (string, error) {
	f.body = append([]byte(nil), body...)
	f.key = "candidate/u/session/s/transcript/evaluation-input.json"
	return f.key, nil
}

func TestTheSealedInputCarriesTheCompetenciesAndEveryTurn(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	writer := &fakeInputWriter{}
	competencies := func(context.Context, interview.Session) ([]interview.Competency, error) {
		return []interview.Competency{{ID: "systems-design", Name: "Systems design"}}, nil
	}
	completer := interview.NewCompleter(store).WithEvaluationInput(writer, competencies)
	session := runningSession(t)

	// Before completion there is no seal, and the completer says so
	// rather than failing.
	sealed, err := completer.Sealed(ctx, session)
	if err != nil {
		t.Fatalf("sealed: %v", err)
	}
	if sealed {
		t.Fatal("a running session reported itself sealed")
	}

	receipt, err := completer.Complete(ctx, session.ID, "practice", candidateID, "", 1, 5)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if receipt.EvaluationInputDigest == "" {
		t.Fatal("the seal records no evaluation input digest")
	}

	var input struct {
		SessionID    string                     `json:"session_id"`
		Competencies []interview.Competency     `json:"competencies"`
		Turns        []interview.EvaluationTurn `json:"turns"`
	}
	if err := json.Unmarshal(writer.body, &input); err != nil {
		t.Fatalf("the stored document does not decode: %v", err)
	}
	if input.SessionID != session.ID || len(input.Competencies) != 1 {
		t.Fatalf("input = %+v", input)
	}
	// Both conversational turns, and nothing else: the connection event
	// is not testimony.
	if len(input.Turns) != 2 {
		t.Fatalf("%d turns in the sealed input", len(input.Turns))
	}
	for _, turn := range input.Turns {
		if turn.Text == "" || turn.EndMs <= turn.StartMs {
			t.Fatalf("turn = %+v", turn)
		}
	}

	// And now it is sealed.
	current, err := store.Get(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	sealed, err = completer.Sealed(ctx, current)
	if err != nil {
		t.Fatalf("sealed: %v", err)
	}
	if !sealed {
		t.Fatal("a completed session does not report itself sealed")
	}
}

func TestTheBundleReadsBackUnderTheOwnersScope(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)

	// A bundle exists only where readiness wrote one, so this session
	// carries the document itself rather than only its digest.
	composing, err := store.Transition(ctx, createPractice(t), interview.StateComposing, interview.Effects{}, candidate)
	if err != nil {
		t.Fatalf("to composing: %v", err)
	}
	document := []byte(`{"schema_version":"1.0","blueprint_id":"plan/shape_technical","pinned_inputs":[]}`)
	effects := interview.Effects{
		BundleRef: "bundles/" + composing.ID, BundleDigest: "sha256:d", BundleRevision: 1,
		BundleBody: document,
	}
	event, err := interview.ReadyEvent(composing, effects, candidate)
	if err != nil {
		t.Fatalf("ready event: %v", err)
	}
	effects.Event = event
	session, err := store.Transition(ctx, composing, interview.StateReady, effects, candidate)
	if err != nil {
		t.Fatalf("to ready: %v", err)
	}

	body, err := store.Bundle(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	var bundle map[string]any
	if err := json.Unmarshal(body, &bundle); err != nil {
		t.Fatalf("the bundle is not a document: %v", err)
	}
	if bundle["blueprint_id"] != "plan/shape_technical" {
		t.Fatalf("bundle = %v", bundle)
	}

	// Somebody else's session has no bundle to read.
	if _, err := store.Bundle(ctx, session.ID, "practice", "00000000-0000-7000-8000-0000000000f2", ""); err == nil {
		t.Fatal("a stranger read the bundle")
	}
}

func TestTheHistoryIsTheOwnersOwnNewestFirst(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	mine := createPractice(t)

	history, err := store.ListMine(ctx, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("the owner's own session is missing from their history")
	}
	found := false
	for _, session := range history {
		if session.ID == mine.ID {
			found = true
		}
		if session.CandidateID != candidateID {
			t.Fatalf("a session belonging to %s reached this history", session.CandidateID)
		}
	}
	if !found {
		t.Fatal("the session just created is not in the history")
	}
	// Newest first: creation order reversed.
	for i := 1; i < len(history); i++ {
		if history[i].CreatedAt.After(history[i-1].CreatedAt) {
			t.Fatal("the history is not newest first")
		}
	}

	stranger, err := store.ListMine(ctx, "practice", "00000000-0000-7000-8000-0000000000f2", "")
	if err != nil {
		t.Fatalf("stranger history: %v", err)
	}
	for _, session := range stranger {
		if session.ID == mine.ID {
			t.Fatal("a stranger's history contains this candidate's session")
		}
	}
}
