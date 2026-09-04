//go:build integration

package recruiting_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// REV-06 against real PostgreSQL: the freeze happens at raise, the
// original reviewer cannot answer their own appeal at any layer, and a
// resolution is whole, once, and immutable - even the migrator cannot
// rewrite one.

// appealedSession records one decision so there is something to appeal.
func appealedSession(t *testing.T, campaign recruiting.Campaign) (sessionID, reviewer string, evidence recruiting.EvidenceVersion) {
	t.Helper()
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	sessionID = id.New().String()
	reviewer = seedCandidate(t)
	evidence = evidenceFixture()
	if _, err := store.RecordReviewDecision(ctx, tenantA, campaign.ID, sessionID,
		reviewer, "reject", "the contradictions stand unresolved", evidence, nil); err != nil {
		t.Fatalf("decision: %v", err)
	}
	return sessionID, reviewer, evidence
}

func TestRaisingFreezesTheEvidenceTheDecisionRead(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	campaign := openCampaignFor(t, tenantA)
	sessionID, reviewer, evidence := appealedSession(t, campaign)
	requester := seedCandidate(t)

	// Nothing decided, nothing to appeal.
	if _, err := store.RaiseReReview(ctx, tenantA, campaign.ID, id.New().String(),
		requester, "the outcome reads wrong to me", "sha256:bundle"); !errors.Is(err, recruiting.ErrAppealNoDecision) {
		t.Fatalf("appeal without decision: %v", err)
	}

	appeal, err := store.RaiseReReview(ctx, tenantA, campaign.ID, sessionID,
		requester, "the second competency's evidence looks misread", "sha256:bundle")
	if err != nil {
		t.Fatalf("raise: %v", err)
	}

	// Frozen at the moment of raising: the decision's own evidence version
	// and the bundle, plus the person whose decision is under appeal.
	if appeal.Frozen != evidence {
		t.Fatalf("frozen = %+v, want the decision's %+v", appeal.Frozen, evidence)
	}
	if appeal.BundleDigest != "sha256:bundle" || appeal.OriginalReviewer != reviewer {
		t.Fatalf("appeal = %+v", appeal)
	}
	if !appeal.DueAt.After(appeal.RaisedAt) {
		t.Fatalf("due %v is not after raised %v: an appeal has an answer-by time", appeal.DueAt, appeal.RaisedAt)
	}

	// A NEWER decision after the appeal changes nothing frozen: the appeal
	// still reads the evidence its decision read.
	if _, err := store.RecordReviewDecision(ctx, tenantA, campaign.ID, sessionID,
		reviewer, "hold", "holding pending the appeal", recruiting.EvidenceVersion{
			EvaluationID: id.New().String(), ResultDigest: "sha256:newer", RubricDigest: "sha256:newer",
		}, nil); err != nil {
		t.Fatalf("newer decision: %v", err)
	}
	appeals, err := store.ReReviewsForSession(ctx, tenantA, campaign.ID, sessionID)
	if err != nil || len(appeals) != 1 {
		t.Fatalf("appeals = %d (%v)", len(appeals), err)
	}
	if appeals[0].Frozen != evidence {
		t.Fatalf("the freeze drifted: %+v", appeals[0].Frozen)
	}
}

func TestTheOriginalReviewerCannotAnswerTheirOwnAppeal(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	campaign := openCampaignFor(t, tenantA)
	sessionID, reviewer, _ := appealedSession(t, campaign)

	appeal, err := store.RaiseReReview(ctx, tenantA, campaign.ID, sessionID,
		seedCandidate(t), "please look again", "sha256:bundle")
	if err != nil {
		t.Fatalf("raise: %v", err)
	}

	// Assignment refuses them in code...
	if _, err := store.AssignReReview(ctx, tenantA, campaign.ID, appeal.ID, reviewer); !errors.Is(err, recruiting.ErrAppealSelfReview) {
		t.Fatalf("self-assignment: %v", err)
	}
	// ...and the schema refuses them even if code is bypassed.
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx,
		`UPDATE recruiting.re_review SET assigned_to = $1 WHERE id = $2`,
		reviewer, appeal.ID); err == nil {
		t.Fatalf("the schema seated the original reviewer")
	}

	// An independent reviewer is seated; somebody unassigned cannot answer;
	// the assignee, not being the original reviewer, can.
	independent := seedCandidate(t)
	if _, err := store.AssignReReview(ctx, tenantA, campaign.ID, appeal.ID, independent); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if _, err := store.ResolveReReview(ctx, tenantA, campaign.ID, appeal.ID,
		seedCandidate(t), "upheld", "the evidence reads as recorded", "Your re-review is complete."); !errors.Is(err, recruiting.ErrAppealResolverNotAssigned) {
		t.Fatalf("unassigned resolver: %v", err)
	}
	resolved, err := store.ResolveReReview(ctx, tenantA, campaign.ID, appeal.ID,
		independent, "upheld", "the evidence reads as recorded", "Your re-review is complete; the outcome stands.")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Resolution == nil || resolved.Resolution.Outcome != "upheld" {
		t.Fatalf("resolution = %+v", resolved.Resolution)
	}
}

func TestAResolutionIsWholeOnceAndImmutable(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	campaign := openCampaignFor(t, tenantA)
	sessionID, _, _ := appealedSession(t, campaign)

	appeal, err := store.RaiseReReview(ctx, tenantA, campaign.ID, sessionID,
		seedCandidate(t), "please look again", "sha256:bundle")
	if err != nil {
		t.Fatalf("raise: %v", err)
	}
	independent := seedCandidate(t)
	if _, err := store.AssignReReview(ctx, tenantA, campaign.ID, appeal.ID, independent); err != nil {
		t.Fatalf("assign: %v", err)
	}

	// A resolution missing its rationale or disclosure is refused whole.
	if _, err := store.ResolveReReview(ctx, tenantA, campaign.ID, appeal.ID,
		independent, "revised", "", "disclosure"); !errors.Is(err, recruiting.ErrAppealOutcomeInvalid) {
		t.Fatalf("no rationale: %v", err)
	}
	if _, err := store.ResolveReReview(ctx, tenantA, campaign.ID, appeal.ID,
		independent, "escalated", "r", "d"); !errors.Is(err, recruiting.ErrAppealOutcomeInvalid) {
		t.Fatalf("unknown outcome: %v", err)
	}

	if _, err := store.ResolveReReview(ctx, tenantA, campaign.ID, appeal.ID,
		independent, "revised", "the misread span changes the band", "Your re-review is complete; the outcome changed."); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	// Once answered, answered: a second resolution converges on refusal,
	// and even the migrator cannot rewrite or delete the record.
	if _, err := store.ResolveReReview(ctx, tenantA, campaign.ID, appeal.ID,
		independent, "upheld", "second thoughts", "d"); !errors.Is(err, recruiting.ErrAppealResolved) {
		t.Fatalf("second resolution: %v", err)
	}
	conn, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("admin connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx,
		`UPDATE recruiting.re_review SET outcome_rationale = 'rewritten' WHERE id = $1`,
		appeal.ID); err == nil {
		t.Fatalf("a resolved re-review was rewritten")
	}
	if _, err := conn.Exec(ctx,
		`DELETE FROM recruiting.re_review WHERE id = $1`, appeal.ID); err == nil {
		t.Fatalf("a re-review was deleted")
	}

	// The SLA the row carries is the platform default at raise.
	if appeal.DueAt.Sub(appeal.RaisedAt).Round(time.Hour) != recruiting.ReReviewSLA {
		t.Fatalf("SLA = %v", appeal.DueAt.Sub(appeal.RaisedAt))
	}
}
