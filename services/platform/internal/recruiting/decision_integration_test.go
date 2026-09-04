//go:build integration

package recruiting_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// REV-03 against real PostgreSQL: no outcome without a named human, an
// override never travels without its rationale, and the history is
// append-only where conventions cannot reach - the trigger refuses an
// UPDATE whoever asks.

func evidenceFixture() recruiting.EvidenceVersion {
	return recruiting.EvidenceVersion{
		EvaluationID: id.New().String(),
		ResultDigest: "sha256:result",
		RubricDigest: "sha256:rubric",
	}
}

func TestADecisionIsANamedHumansWithItsReason(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	campaign := openCampaignFor(t, tenantA)
	sessionID := id.New().String()
	reviewer := seedCandidate(t)

	// The refusals first: no reason, no actor, no unknown outcome.
	if _, err := store.RecordReviewDecision(ctx, tenantA, campaign.ID, sessionID,
		reviewer, "advance", "   ", evidenceFixture(), nil); !errors.Is(err, recruiting.ErrDecisionReasonRequired) {
		t.Fatalf("blank reason: %v", err)
	}
	if _, err := store.RecordReviewDecision(ctx, tenantA, campaign.ID, sessionID,
		"", "advance", "strong evidence throughout", evidenceFixture(), nil); !errors.Is(err, recruiting.ErrDecisionActorRequired) {
		t.Fatalf("no actor: %v", err)
	}
	if _, err := store.RecordReviewDecision(ctx, tenantA, campaign.ID, sessionID,
		reviewer, "auto_advance", "r", evidenceFixture(), nil); !errors.Is(err, recruiting.ErrDecisionUnknown) {
		t.Fatalf("unknown decision: %v", err)
	}

	evidence := evidenceFixture()
	decision, err := store.RecordReviewDecision(ctx, tenantA, campaign.ID, sessionID,
		reviewer, "advance", "evidenced on both core competencies", evidence, nil)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if decision.DecidedBy != reviewer || decision.Decision != "advance" {
		t.Fatalf("decision = %+v", decision)
	}
	// The evidence version rides the row, exactly as it was captured.
	if decision.Evidence != evidence {
		t.Fatalf("evidence = %+v, want %+v", decision.Evidence, evidence)
	}

	// The catalogued announcement committed with the row: who, on what,
	// informed by which evaluation, and no reasoning text.
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer conn.Close(ctx)
	var decidedBy, payload string
	if err := conn.QueryRow(ctx, `
		SELECT payload->>'decided_by', payload::text FROM integration.outbox
		WHERE event_type = 'review.decision_recorded.v1'
		  AND payload->>'review_id' = $1`, decision.ID).Scan(&decidedBy, &payload); err != nil {
		t.Fatalf("outbox read: %v", err)
	}
	if decidedBy != reviewer {
		t.Fatalf("announced decider = %q, want %q", decidedBy, reviewer)
	}
	if strings.Contains(payload, "evidenced on both") || strings.Contains(payload, "reason") {
		t.Fatalf("the announcement carries reasoning text: %s", payload)
	}
}

func TestAnOverrideNeverTravelsWithoutItsRationale(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	campaign := openCampaignFor(t, tenantA)
	sessionID := id.New().String()
	reviewer := seedCandidate(t)

	// Missing rationale: refused whole, nothing recorded.
	if _, err := store.RecordReviewDecision(ctx, tenantA, campaign.ID, sessionID,
		reviewer, "hold", "the systems design band reads high to me", evidenceFixture(),
		[]recruiting.BandOverride{{
			CompetencyID: "sd", RecordedBand: "strong", OverrideBand: "solid",
		}}); !errors.Is(err, recruiting.ErrOverrideIncomplete) {
		t.Fatalf("override without rationale: %v", err)
	}

	recorded, err := store.RecordReviewDecision(ctx, tenantA, campaign.ID, sessionID,
		reviewer, "hold", "holding pending the override question", evidenceFixture(),
		[]recruiting.BandOverride{{
			CompetencyID: "sd", RecordedBand: "strong", OverrideBand: "solid",
			Rationale: "the supporting quotes restate one incident three times",
		}})
	if err != nil {
		t.Fatalf("record with override: %v", err)
	}
	if len(recorded.Overrides) != 1 || recorded.Overrides[0].RecordedBand != "strong" {
		t.Fatalf("override = %+v: the band disagreed with is part of the record", recorded.Overrides)
	}
}

func TestTheHistoryIsAppendOnlyWithEveryTrueActor(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	campaign := openCampaignFor(t, tenantA)
	sessionID := id.New().String()
	first := seedCandidate(t)
	second := seedCandidate(t)

	if _, err := store.RecordReviewDecision(ctx, tenantA, campaign.ID, sessionID,
		first, "hold", "waiting on the take-home", evidenceFixture(), nil); err != nil {
		t.Fatalf("first decision: %v", err)
	}
	if _, err := store.RecordReviewDecision(ctx, tenantA, campaign.ID, sessionID,
		second, "advance", "take-home confirmed the evidence", evidenceFixture(), nil); err != nil {
		t.Fatalf("second decision: %v", err)
	}

	history, err := store.ReviewDecisionsForSession(ctx, tenantA, campaign.ID, sessionID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history = %d rows, want both decisions kept", len(history))
	}
	if history[0].DecidedBy != first || history[1].DecidedBy != second {
		t.Fatalf("actors = %q then %q: the true actor of each decision survives",
			history[0].DecidedBy, history[1].DecidedBy)
	}

	// The trigger refuses a rewrite whoever asks: even the migrator cannot
	// edit a decision, because a history that can be rewritten is not one.
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx,
		`UPDATE recruiting.review_decision SET decision = 'advance' WHERE id = $1`,
		history[0].ID); err == nil {
		t.Fatalf("a decision row was rewritten")
	}
	if _, err := conn.Exec(ctx,
		`DELETE FROM recruiting.review_decision WHERE id = $1`,
		history[0].ID); err == nil {
		t.Fatalf("a decision row was deleted")
	}
}
