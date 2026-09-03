//go:build integration

package recruiting_test

// SCR-03 against real PostgreSQL: requirements carry provenance that indexes
// back into the stored job context, a recruiter can correct one while the
// campaign is a draft, and the moment the campaign opens they freeze, which is
// what "pinned into the campaign configuration" is enforced by.

import (
	"context"
	"errors"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

const jobDescription = "Responsibilities:\n- Five years of Go\n- Own the on-call rotation\n"

func TestRequirementsCarryProvenanceIntoTheStoredSource(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	campaign, err := store.CreateDraft(ctx, draftFor(tenantA, "JD "+id.New().String()))
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	requirements, err := store.SubmitJobContext(ctx, tenantA, campaign.ID, jobDescription, recruiting.NewRuleExtractor())
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if len(requirements) != 2 {
		t.Fatalf("got %d requirements, want 2: %+v", len(requirements), requirements)
	}

	// Each stored span must index back into the exact source bytes the campaign
	// holds, read straight from the row.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantA); err != nil {
		t.Fatalf("scope: %v", err)
	}
	var source string
	if err := tx.QueryRow(ctx, `SELECT source_text FROM recruiting.job_context WHERE campaign_id = $1`, campaign.ID).Scan(&source); err != nil {
		t.Fatalf("read source: %v", err)
	}
	if source != jobDescription {
		t.Fatalf("source stored altered: %q", source)
	}
	for _, req := range requirements {
		if source[req.SpanStart:req.SpanEnd] == "" {
			t.Fatalf("span [%d,%d) resolves to nothing", req.SpanStart, req.SpanEnd)
		}
	}
}

func TestCorrectingARequirementKeepsItsSpan(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	campaign, _ := store.CreateDraft(ctx, draftFor(tenantA, "JD "+id.New().String()))
	requirements, err := store.SubmitJobContext(ctx, tenantA, campaign.ID, jobDescription, recruiting.NewRuleExtractor())
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	original := requirements[0]

	corrected, err := store.CorrectRequirement(ctx, tenantA, campaign.ID, original.ID, "5+ years of Go in production", recruiting.RequirementCorrected)
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if corrected.Text != "5+ years of Go in production" || corrected.Status != recruiting.RequirementCorrected {
		t.Fatalf("correction not applied: %+v", corrected)
	}
	if corrected.SpanStart != original.SpanStart || corrected.SpanEnd != original.SpanEnd {
		t.Fatalf("correction moved the span: %+v vs %+v", corrected, original)
	}

	// A requirement on another campaign is not found.
	other, _ := store.CreateDraft(ctx, draftFor(tenantA, "JD "+id.New().String()))
	if _, err := store.CorrectRequirement(ctx, tenantA, other.ID, original.ID, "x", recruiting.RequirementCorrected); !errors.Is(err, recruiting.ErrRequirementNotFound) {
		t.Fatalf("cross-campaign correct error = %v, want ErrRequirementNotFound", err)
	}
}

// Opening the campaign freezes its requirements: neither a correction nor a
// resubmission can change them afterwards. This is the pin.
func TestRequirementsFreezeWhenTheCampaignOpens(t *testing.T) {
	ctx := context.Background()
	store := recruiting.NewStore(pool)
	determinationID := seedDetermination(t, "GB-req-"+id.New().String()[:8])

	campaign, _ := store.CreateDraft(ctx, recruiting.Campaign{
		TenantID: tenantA, Name: "JD freeze " + id.New().String(),
		RoleReference: "role/backend", Jurisdiction: "GB", CreatedBy: id.New().String(),
	})
	requirements, err := store.SubmitJobContext(ctx, tenantA, campaign.ID, jobDescription, recruiting.NewRuleExtractor())
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if _, err := store.Open(ctx, campaign, recruiting.Opening{
		Determination: recruiting.Determination{ID: determinationID, Jurisdiction: "GB"},
		Pins: []recruiting.Pin{
			{Type: "rubric", Reference: "rubric/backend", Version: "3.0.0", Digest: "sha256:" + repeat("a")},
			{Type: "calibration", Reference: "calibration/backend", Version: "1.0.0", Digest: "sha256:" + repeat("b")},
			{Type: "persona", Reference: "persona/neutral", Version: "1.0.0", Digest: "sha256:" + repeat("c")},
			{Type: "plan", Reference: "plan/standard", Version: "2.0.0", Digest: "sha256:" + repeat("d")},
		},
	}); err != nil {
		t.Fatalf("open: %v", err)
	}

	// A correction after open is refused by the freeze trigger.
	if _, err := store.CorrectRequirement(ctx, tenantA, campaign.ID, requirements[0].ID, "changed", recruiting.RequirementCorrected); err == nil {
		t.Fatal("a requirement was corrected after the campaign opened")
	}
	// A resubmission after open is refused too (it would delete frozen rows).
	if _, err := store.SubmitJobContext(ctx, tenantA, campaign.ID, "New JD\n- Different\n", recruiting.NewRuleExtractor()); err == nil {
		t.Fatal("the job context was resubmitted after the campaign opened")
	}

	// The requirements still read as they were pinned.
	frozen, err := store.RequirementsForCampaign(ctx, tenantA, campaign.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(frozen) != len(requirements) {
		t.Fatalf("requirement count changed after open: %d vs %d", len(frozen), len(requirements))
	}
}
