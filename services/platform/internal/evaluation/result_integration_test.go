//go:build integration

package evaluation_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// EVL-02's second and third boxes against real PostgreSQL: one result per
// session with its notification in the same transaction, retries that
// converge instead of duplicating, and a stored result that carries the
// PINNED rubric coordinates so the judgment is reconstructable after the
// registry moves on.

// practicePin is the rubric exactly as a bundle would pin it.
func practicePin() evaluation.RubricPin {
	return evaluation.RubricPin{
		Reference: "rubric/practice-default", Version: "1.0.0",
		Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Body: json.RawMessage(`{"sufficiency":{"min_supporting":2},"bands":[` +
			`{"id":"developing","min_ratio":0},{"id":"solid","min_ratio":0.55},{"id":"strong","min_ratio":0.8}],` +
			`"confidence":{"high":{"min_supporting":4,"max_contradictory":0},` +
			`"medium":{"min_supporting":2,"max_contradictory":1}}}`),
	}
}

// storedFixture wraps plain spans with deterministic ids for aggregation.
func storedFixture(spans ...evaluation.Span) []evaluation.StoredSpan {
	stored := make([]evaluation.StoredSpan, 0, len(spans))
	for i, span := range spans {
		stored = append(stored, evaluation.StoredSpan{
			ID: fmt.Sprintf("sp-%d", i), Span: span,
		})
	}
	return stored
}

func aggregated(t *testing.T, pin evaluation.RubricPin, spans []evaluation.StoredSpan) evaluation.Aggregation {
	t.Helper()
	rubric, err := evaluation.ParseRubric(pin.Body)
	if err != nil {
		t.Fatalf("the pin's own body refuses to parse: %v", err)
	}
	return evaluation.Aggregate(rubric,
		[]evaluation.Competency{{ID: "systems-design", Name: "Systems design"}}, spans)
}

func TestStoreResultIsExactlyOncePerSession(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	events := outbox.New(pool)
	ref := evaluation.SessionRef{
		SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate,
	}
	pin := practicePin()
	spans := storedFixture(
		span(1, "systems-design", "we cut latency by 40 percent"),
		span(2, "systems-design", "the cache held 2 million entries"),
	)
	aggregation := aggregated(t, pin, spans)

	first, err := store.StoreResult(ctx, events, ref, pin, "evidence-1", aggregation, nil)
	if err != nil {
		t.Fatalf("first store: %v", err)
	}

	// The retry: same session, same inputs, run again as a workflow retry
	// would. It converges on the stored result rather than duplicating.
	second, err := store.StoreResult(ctx, events, ref, pin, "evidence-1", aggregation, nil)
	if err != nil {
		t.Fatalf("the retry errored instead of converging: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("the retry produced a different result:\nfirst  %+v\nsecond %+v", first, second)
	}

	// Exactly one notification: same transaction as the row, once ever.
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM integration.outbox
		  WHERE event_type = 'evaluation.completed.v1' AND payload->>'session_id' = $1`,
		ref.SessionID).Scan(&count); err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if count != 1 {
		t.Fatalf("two stores produced %d evaluation.completed.v1 events, want exactly 1", count)
	}
}

func TestTheResultRecordsThePinNotThePublishedVersion(t *testing.T) {
	// The reconstruction property. The pin the session composed with says
	// version 1.0.0; by evaluation time the registry may be publishing
	// 2.0.0. The stored result must carry the pin's coordinates so the
	// judgment can be re-derived from exactly what judged it.
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	events := outbox.New(pool)
	ref := evaluation.SessionRef{
		SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate,
	}
	pin := practicePin()
	spans := storedFixture(
		span(1, "systems-design", "throughput doubled to 10k rps"),
		span(2, "systems-design", "we halved the p99 to 80ms"),
	)

	stored, err := store.StoreResult(ctx, events, ref, pin, "evidence-1", aggregated(t, pin, spans), nil)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if stored.RubricReference != pin.Reference || stored.RubricVersion != pin.Version || stored.RubricDigest != pin.Digest {
		t.Fatalf("the result's rubric coordinates are %s@%s (%s), want the pin's %s@%s (%s)",
			stored.RubricReference, stored.RubricVersion, stored.RubricDigest,
			pin.Reference, pin.Version, pin.Digest)
	}
	if stored.AggregationVersion != evaluation.AggregationVersion || stored.ExtractionVersion != "evidence-1" {
		t.Fatalf("the versions are %s/%s", stored.AggregationVersion, stored.ExtractionVersion)
	}
	// aggregate-1 is a deterministic floor: no model, no policy. Recorded
	// honestly as none, not left empty or invented.
	if stored.ModelVersion != "none" || stored.PolicyVersion != "none" {
		t.Fatalf("model/policy versions are %s/%s, want none/none", stored.ModelVersion, stored.PolicyVersion)
	}

	read, err := store.ResultOf(ctx, ref)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if !reflect.DeepEqual(stored, read) {
		t.Fatalf("the read-back result differs:\nstored %+v\nread   %+v", stored, read)
	}
}

func TestAStoredResultRefusesToChange(t *testing.T) {
	// Immutability by trigger, proven by attacking it directly.
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	events := outbox.New(pool)
	ref := evaluation.SessionRef{
		SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate,
	}
	pin := practicePin()
	spans := storedFixture(
		span(1, "systems-design", "the migration moved 3 billion rows"),
		span(2, "systems-design", "downtime was 0 minutes"),
	)
	if _, err := store.StoreResult(ctx, events, ref, pin, "evidence-1", aggregated(t, pin, spans), nil); err != nil {
		t.Fatalf("store: %v", err)
	}

	// The attack runs inside the owner's own scope: without it RLS hides
	// the row, zero rows match, and no row trigger fires - which would
	// prove nothing. The trigger must refuse a write that CAN see the row.
	attack := func(statement string) error {
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
		tag, err := tx.Exec(ctx, statement, ref.SessionID)
		if err == nil && tag.RowsAffected() == 0 {
			t.Fatal("the attack matched zero rows; the trigger was never exercised")
		}
		return err
	}

	err := attack(`UPDATE evaluation.results SET model_version = 'gpt-x' WHERE session_id = $1`)
	if err == nil {
		t.Fatal("an UPDATE on a stored result was accepted; results are immutable")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("the refusal does not name immutability: %v", err)
	}
	if err := attack(`DELETE FROM evaluation.results WHERE session_id = $1`); err == nil {
		t.Fatal("a DELETE on a stored result was accepted; results are immutable")
	}
}
