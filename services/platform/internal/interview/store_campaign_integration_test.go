//go:build integration

package interview_test

// SES/SCR-05: a screening session belongs to a campaign, and its candidate can
// read it.
//
// The schema promises two things these prove against the real database: a
// screening session cannot exist without a campaign and a practice session
// cannot have one, both by CHECK; and the candidate who sits a screening
// session can read it as themselves, in a transaction carrying no tenant, while
// another candidate cannot and the tenant still can.

import (
	"context"
	"errors"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// seedCampaign inserts a draft campaign under the tenant, so a screening
// session has a real campaign to reference. It writes through the app pool with
// the tenant set, which is the only scope the campaign's row-level security
// admits, rather than as the migrator, which the forced policy would refuse.
func seedCampaign(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	campaignID := id.New().String()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetTenant(ctx, tx, tenantID); err != nil {
		t.Fatalf("scope: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO recruiting.campaign (id, tenant_id, name, role_reference, jurisdiction, created_by)
		 VALUES ($1, $2, 'Screening run', 'role/backend', 'GB', $3)`,
		campaignID, tenantID, id.New().String()); err != nil {
		t.Fatalf("seeding the campaign: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return campaignID
}

// A screening session must name a campaign, and a practice session must not:
// both are CHECK, so the database refuses the write rather than trusting Go.
func TestTheCampaignLinkIsAModeInvariant(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)

	// Screening with no campaign is refused.
	err := store.Create(ctx, interview.Session{
		ID: id.New().String(), Mode: "screening", CandidateID: candidateID,
		TenantID: tenantID, BlueprintID: "bp_screen",
	}, candidate)
	if err == nil {
		t.Fatal("a screening session with no campaign was created")
	}

	// Practice with a campaign is refused.
	err = store.Create(ctx, interview.Session{
		ID: id.New().String(), Mode: "practice", CandidateID: candidateID,
		CampaignID: seedCampaign(t), BlueprintID: "bp_practice",
	}, candidate)
	if err == nil {
		t.Fatal("a practice session was given a campaign")
	}
}

// The candidate who sits a screening session can read it as themselves; another
// candidate cannot; the tenant still can. This is the owner policy 0058 adds.
func TestACandidateReadsTheirOwnScreeningSession(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	campaignID := seedCampaign(t)

	sessionID := id.New().String()
	if err := store.Create(ctx, interview.Session{
		ID: sessionID, Mode: "screening", CandidateID: candidateID,
		TenantID: tenantID, CampaignID: campaignID, BlueprintID: "bp_screen",
	}, candidate); err != nil {
		t.Fatalf("create screening: %v", err)
	}

	// The candidate, acting as themselves with no tenant, sees it.
	got, err := store.GetScreeningForCandidate(ctx, sessionID, candidateID)
	if err != nil {
		t.Fatalf("candidate read: %v", err)
	}
	if got.CampaignID != campaignID {
		t.Fatalf("campaign not read back: %q", got.CampaignID)
	}

	// A different candidate sees nothing, and existence is not answered.
	other := id.New().String()
	if _, err := store.GetScreeningForCandidate(ctx, sessionID, other); !errors.Is(err, interview.ErrNotFound) {
		t.Fatalf("another candidate read the session: %v", err)
	}

	// The tenant still reads it through the tenant policy.
	if _, err := store.Get(ctx, sessionID, "screening", candidateID, tenantID); err != nil {
		t.Fatalf("tenant read: %v", err)
	}
}

// A candidate cannot reach a practice session through the screening owner
// policy: it is keyed to mode = 'screening', so a practice session of the same
// candidate is invisible to this read even though they own it.
func TestScreeningCandidateReadDoesNotReachPractice(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)

	sessionID := id.New().String()
	if err := store.Create(ctx, interview.Session{
		ID: sessionID, Mode: "practice", CandidateID: candidateID, BlueprintID: "bp_practice",
	}, candidate); err != nil {
		t.Fatalf("create practice: %v", err)
	}

	if _, err := store.GetScreeningForCandidate(ctx, sessionID, candidateID); !errors.Is(err, interview.ErrNotFound) {
		t.Fatalf("the screening read reached a practice session: %v", err)
	}
}

// REV-01's read: the campaign's sessions under the tenant's own scope, one
// narrow row per interview, ordered by creation and never by anything like
// quality.
func TestCampaignSessionsAnswerTheRosterRows(t *testing.T) {
	ctx := context.Background()
	store := interview.NewStore(pool)
	campaignID := seedCampaign(t)

	first := id.New().String()
	if err := store.Create(ctx, interview.Session{
		ID: first, Mode: "screening", CandidateID: candidateID,
		TenantID: tenantID, CampaignID: campaignID, BlueprintID: "bp_screen",
	}, candidate); err != nil {
		t.Fatalf("create first: %v", err)
	}
	// A second campaign's session must not appear in the first's roster.
	elsewhere := seedCampaign(t)
	if err := store.Create(ctx, interview.Session{
		ID: id.New().String(), Mode: "screening", CandidateID: candidateID,
		TenantID: tenantID, CampaignID: elsewhere, BlueprintID: "bp_screen",
	}, candidate); err != nil {
		t.Fatalf("create elsewhere: %v", err)
	}

	rows, err := store.CampaignSessions(ctx, tenantID, campaignID)
	if err != nil {
		t.Fatalf("campaign sessions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want exactly the campaign's own", len(rows))
	}
	if rows[0].ID != first || rows[0].CandidateID != candidateID {
		t.Fatalf("row = %+v", rows[0])
	}
	if rows[0].State != interview.StateDraft {
		t.Fatalf("state = %s, want the lifecycle's own vocabulary", rows[0].State)
	}
	if rows[0].StateChangedAt.IsZero() {
		t.Fatalf("state_changed_at missing: the roster reads submission time from it")
	}
}
