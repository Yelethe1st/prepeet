//go:build integration

package evaluation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// ART-09 against real PostgreSQL. The three properties that matter are
// structural rather than conventional, so each is attacked rather than
// assumed: one verdict per insight and changeable, practice only, and
// nobody else's to read or write.

func verdict(key string, helpful bool) evaluation.InsightVerdict {
	return evaluation.InsightVerdict{
		Kind: evaluation.InsightStrength, Key: key, Dimension: "precision",
		Helpful: helpful, ArtifactDigest: "sha256:coaching-v1", PolicyVersion: "articulation-practice-v1",
	}
}

func TestAVerdictIsOncePerInsightAndChangeable(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	ref := evaluation.SessionRef{SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate}

	if err := store.RecordInsightFeedback(ctx, ref, verdict("precision", false)); err != nil {
		t.Fatalf("first verdict: %v", err)
	}
	// The same person, the same sentence, the other thumb. This must correct
	// the row rather than leave two opinions to be counted separately.
	if err := store.RecordInsightFeedback(ctx, ref, verdict("precision", true)); err != nil {
		t.Fatalf("second verdict: %v", err)
	}

	got, err := store.InsightFeedbackFor(ctx, ref)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want one verdict after pressing both thumbs, got %d", len(got))
	}
	if !got[0].Helpful {
		t.Fatal("the later verdict did not win")
	}
}

func TestVerdictsOnDifferentInsightsCoexist(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	ref := evaluation.SessionRef{SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate}

	for _, key := range []string{"precision", "pace", "structure"} {
		if err := store.RecordInsightFeedback(ctx, ref, verdict(key, false)); err != nil {
			t.Fatalf("verdict on %s: %v", key, err)
		}
	}

	got, err := store.InsightFeedbackFor(ctx, ref)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want three verdicts on three insights, got %d", len(got))
	}
}

// The practice-only guarantee, attacked. A screening session must not be
// able to carry a verdict at all: the candidate rating their own assessment
// is a channel for pressure, so it is refused before the database and has no
// policy that would let it through if it were not.
func TestAScreeningSessionCannotCarryAVerdict(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	ref := evaluation.SessionRef{
		SessionID: id.New().String(), Mode: "screening",
		CandidateID: evidenceCandidate, TenantID: id.New().String(),
	}

	err := store.RecordInsightFeedback(ctx, ref, verdict("precision", true))
	if err == nil {
		t.Fatal("a screening session recorded a verdict")
	}
	if !strings.Contains(err.Error(), "practice only") {
		t.Fatalf("want a refusal naming the reason, got %v", err)
	}
}

// The same attack one level down, with the domain guard bypassed: even if a
// caller reached the table directly under a tenant scope, the policy matches
// nothing, because there is deliberately no tenant policy on this table.
func TestATenantScopeSeesNoVerdicts(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	owner := evaluation.SessionRef{SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate}

	if err := store.RecordInsightFeedback(ctx, owner, verdict("precision", false)); err != nil {
		t.Fatalf("store: %v", err)
	}

	tenantScope := evaluation.SessionRef{
		SessionID: owner.SessionID, Mode: "screening",
		CandidateID: evidenceCandidate, TenantID: id.New().String(),
	}
	got, err := store.InsightFeedbackFor(ctx, tenantScope)
	if err != nil {
		t.Fatalf("read under a tenant scope: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a tenant scope read %d of the candidate's verdicts", len(got))
	}

	// The row has to be there, or the emptiness above proves nothing: an
	// attack that matches zero rows because there were none to match is not
	// evidence that the policy did anything.
	own, err := store.InsightFeedbackFor(ctx, owner)
	if err != nil {
		t.Fatalf("read as the owner: %v", err)
	}
	if len(own) != 1 {
		t.Fatalf("the row the tenant scope was denied does not exist: owner sees %d", len(own))
	}
}

// Another candidate's verdicts are not this candidate's to read. The row
// exists and the session id is known; only the policy stands between them.
func TestAnotherCandidateCannotReadTheseVerdicts(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	owner := evaluation.SessionRef{SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate}

	if err := store.RecordInsightFeedback(ctx, owner, verdict("precision", false)); err != nil {
		t.Fatalf("store: %v", err)
	}

	stranger := evaluation.SessionRef{
		SessionID: owner.SessionID, Mode: "practice", CandidateID: id.New().String(),
	}
	got, err := store.InsightFeedbackFor(ctx, stranger)
	if err != nil {
		t.Fatalf("read as a stranger: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("a stranger read %d verdicts about somebody else's session", len(got))
	}

	// Same reason as above: the owner still sees it, so what the stranger
	// met was the policy and not an empty table.
	own, err := store.InsightFeedbackFor(ctx, owner)
	if err != nil {
		t.Fatalf("read as the owner: %v", err)
	}
	if len(own) != 1 {
		t.Fatalf("the row the stranger was denied does not exist: owner sees %d", len(own))
	}
}

func TestAnUnknownInsightKindIsRefusedBeforeTheDatabase(t *testing.T) {
	ctx := context.Background()
	store := evaluation.NewStore(pool)
	ref := evaluation.SessionRef{SessionID: id.New().String(), Mode: "practice", CandidateID: evidenceCandidate}

	err := store.RecordInsightFeedback(ctx, ref, evaluation.InsightVerdict{
		Kind: "vibe", Key: "precision", Helpful: true,
	})
	if err == nil {
		t.Fatal("an unknown kind was accepted")
	}
	// Named, rather than surfacing a constraint violation that says
	// "insight_feedback_insight_kind_check" and nothing a caller can act on.
	if !strings.Contains(err.Error(), "unknown insight kind") {
		t.Fatalf("want the kind named, got %v", err)
	}
}
