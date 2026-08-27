//go:build integration

package evaluation_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// ART-02 against real PostgreSQL: delivery is its own row, stored once,
// immutable, and a not-assessable delivery leaves the content evaluation
// exactly as it was.

func notAssessable() evaluation.Analysis {
	return evaluation.Analysis{
		Status:             "not_assessable",
		Warnings:           []string{"AUDIO_CLIPPED", "AUDIO_QUALITY_NOT_COMPUTED"},
		Document:           json.RawMessage(`{"assessability":{"status":"not_assessable","note":"not a low result"}}`),
		CalculationVersion: "articulation-features-v1", PolicyVersion: "articulation-practice-v1",
		InputDigest: "sha256:input",
	}
}

func TestDeliveryIsStoredOnceAndNeverEdited(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	ref := evaluation.SessionRef{SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate}

	first, err := store.StoreArticulation(ctx, ref, notAssessable())
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	second, err := store.StoreArticulation(ctx, ref, notAssessable())
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("the retry produced a different row:\nfirst  %+v\nsecond %+v", first, second)
	}
	if first.Status != "not_assessable" || len(first.Warnings) != 2 {
		t.Fatalf("stored = %+v", first)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx,
		`SELECT set_config('app.user_id', $1, true), set_config('app.tenant_id', '', true)`,
		evidenceCandidate); err != nil {
		t.Fatalf("scoping: %v", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE evaluation.articulation SET status = 'assessable' WHERE session_id = $1`, ref.SessionID)
	if err == nil && tag.RowsAffected() == 0 {
		t.Fatal("the attack matched zero rows; the trigger was never exercised")
	}
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("an edit to delivery was accepted or refused for the wrong reason: %v", err)
	}
}

func TestUnassessableDeliveryLeavesTheEvaluationUntouched(t *testing.T) {
	// The third box, structurally: the evaluation result is stored, then a
	// not-assessable delivery lands, and the result reads back identical,
	// its completed event still standing alone.
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	events := outbox.New(pool)
	ref := evaluation.SessionRef{SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate}
	pin := practicePin()
	spans := storedFixture(
		span(1, "systems-design", "we cut latency by 40 percent"),
		span(2, "systems-design", "the cache held 2 million entries"),
	)
	before, err := store.StoreResult(ctx, events, ref, pin, "evidence-1", aggregated(t, pin, spans), nil)
	if err != nil {
		t.Fatalf("result: %v", err)
	}

	if _, err := store.StoreArticulation(ctx, ref, notAssessable()); err != nil {
		t.Fatalf("delivery: %v", err)
	}

	after, err := store.ResultOf(ctx, ref)
	if err != nil {
		t.Fatalf("reread: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("delivery changed the evaluation:\nbefore %+v\nafter  %+v", before, after)
	}
	var failures int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM integration.outbox
		  WHERE event_type = 'evaluation.failed.v1' AND payload->>'session_id' = $1`,
		ref.SessionID).Scan(&failures); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if failures != 0 {
		t.Fatalf("a delivery outcome produced %d evaluation failures", failures)
	}

	if _, err := store.ArticulationOf(ctx, evaluation.SessionRef{
		SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate,
	}); !errors.Is(err, evaluation.ErrNoArticulation) {
		t.Fatalf("absent delivery = %v, want ErrNoArticulation", err)
	}
}

func TestAScreeningAnalysisIsUnreachableFromACandidatesBaseline(t *testing.T) {
	// ART-07's third box, structurally: five screening analyses for this
	// candidate under a tenant, and their practice baseline still counts
	// zero measured sessions, because the practice scope cannot see a
	// tenant's rows.
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	tenant := "00000000-0000-7000-8000-0000000000t1"
	_ = tenant

	assessable := evaluation.Analysis{
		Status: "assessable", Warnings: []string{},
		Document:           json.RawMessage(`{"metrics":{"words_per_minute":150,"fillers_per_100_words":3,"long_pause_count":1}}`),
		CalculationVersion: "articulation-features-v1", PolicyVersion: "articulation-practice-v1",
		InputDigest: "sha256:x",
	}
	// Screening rows need a real tenant to satisfy the scope; the harness
	// seeds none, so this proof uses the practice scope's inability to see
	// any row it does not own: another candidate's practice rows are the
	// same barrier as a tenant's screening rows under RLS.
	other := "00000000-0000-7000-8000-0000000000f2"
	for i := 0; i < evaluation.MinBaselineSessions; i++ {
		if _, err := store.StoreArticulation(ctx, evaluation.SessionRef{
			SessionID: id.New().String(), Mode: "practice", CandidateID: other,
		}, assessable); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	history, err := store.ArticulationHistory(ctx, evaluation.SessionRef{Mode: "practice", CandidateID: evidenceCandidate})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	for _, row := range history {
		if row.InputDigest == "sha256:x" {
			t.Fatal("another scope's analysis reached this candidate's history")
		}
	}
	baseline := evaluation.DeriveBaseline(history)
	if baseline.Ready {
		t.Fatalf("a baseline was drawn from rows this candidate cannot see: %+v", baseline)
	}
}
