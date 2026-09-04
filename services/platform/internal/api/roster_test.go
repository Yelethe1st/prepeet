package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// REV-01 from the wire's side: the roster is standings, never a ranking.
// Access is the campaign join, the standing filter runs on the server, and
// a candidate whose record could not support assessment is their own named
// standing awaiting the same review, never a low scorer at the bottom.

type stubRoster struct {
	roster      api.Roster
	err         error
	sawTenant   string
	sawCampaign string
}

func (s *stubRoster) CampaignRoster(_ context.Context, tenantID, campaignID string) (api.Roster, error) {
	s.sawTenant, s.sawCampaign = tenantID, campaignID
	if s.err != nil {
		return api.Roster{}, s.err
	}
	return s.roster, nil
}

func serveRoster(t *testing.T, recruiting *stubRecruiting, roster *stubRoster) http.Handler {
	t.Helper()
	identity := &fakeIdentity{
		principal: api.Principal{UserID: progressionUser, ActiveTenantID: campaignTenant},
		allowed:   []authz.Capability{authz.CampaignRead},
	}
	handler, err := api.NewServer(api.ServerConfig{
		Identity: identity, Candidates: &fakeCandidates{}, Documents: &fakeDocuments{},
		Catalog: &fakeCatalog{}, Interviews: &fakeInterviews{}, Members: &fakeMembers{},
		Billing: &fakeBilling{}, Progression: &stubProgression{},
		SensitiveReads: &recordingAuditor{}, Settings: &stubSettings{},
		ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(),
		ReInvitations: defaultStubInvitations(), Requirements: defaultStubRequirements(),
		RecruiterAccommodations: defaultStubInvitations(), Recruiting: recruiting,
		Invitations: defaultStubInvitations(), Roster: roster,
		Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

const rosterPath = "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/candidates"

func sampleRoster() api.Roster {
	submitted := time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)
	return api.Roster{
		PendingReview: 2,
		Candidates: []api.RosterEntry{
			{
				InvitationID: "00000000-0000-7000-8000-00000000e001",
				Recipient:    "amara@example.com",
				Standing:     "awaiting_review",
				InvitedAt:    time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
				SessionID:    "00000000-0000-7000-8000-00000000a001",
				SubmittedAt:  &submitted,
			},
			{
				InvitationID: "00000000-0000-7000-8000-00000000e002",
				Recipient:    "bela@example.com",
				Standing:     "insufficient_evidence",
				InvitedAt:    time.Date(2026, 9, 1, 11, 0, 0, 0, time.UTC),
				SessionID:    "00000000-0000-7000-8000-00000000a002",
				SubmittedAt:  &submitted,
			},
			{
				InvitationID: "00000000-0000-7000-8000-00000000e003",
				Recipient:    "chen@example.com",
				Standing:     "invited",
				InvitedAt:    time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC),
			},
		},
	}
}

func TestTheRosterAnswersStandingsNeverARanking(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	roster := &stubRoster{roster: sampleRoster()}
	handler := serveRoster(t, rec, roster)

	status, body := campaignRequest(t, handler, http.MethodGet, rosterPath, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %v", status, body)
	}
	if body["pending_review"] != float64(2) {
		t.Fatalf("pending_review = %v, want 2: absence of evidence is still a decision to make", body["pending_review"])
	}

	candidates, _ := body["candidates"].([]any)
	if len(candidates) != 3 {
		t.Fatalf("candidates = %d, want all three in the port's own order", len(candidates))
	}
	// The order the composition chose (invitation recency) survives to the
	// wire untouched: nothing reorders by standing or anything like quality.
	first, _ := candidates[0].(map[string]any)
	second, _ := candidates[1].(map[string]any)
	if first["standing"] != "awaiting_review" || second["standing"] != "insufficient_evidence" {
		t.Fatalf("order changed: %v then %v", first["standing"], second["standing"])
	}
	// Insufficient evidence is its own named standing on the row, and no
	// row carries a band, a score or a ratio to rank by.
	for _, entry := range candidates {
		row, _ := entry.(map[string]any)
		for _, forbidden := range []string{"band", "score", "ratio", "rank", "confidence"} {
			if _, present := row[forbidden]; present {
				t.Fatalf("a roster row carried %q", forbidden)
			}
		}
	}
}

func TestTheStandingFilterRunsServerSide(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	roster := &stubRoster{roster: sampleRoster()}
	handler := serveRoster(t, rec, roster)

	status, body := campaignRequest(t, handler, http.MethodGet,
		rosterPath+"?standing=insufficient_evidence", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %v", status, body)
	}
	candidates, _ := body["candidates"].([]any)
	if len(candidates) != 1 {
		t.Fatalf("filtered candidates = %d, want exactly the one at that standing", len(candidates))
	}
	row, _ := candidates[0].(map[string]any)
	if row["standing"] != "insufficient_evidence" {
		t.Fatalf("standing = %v", row["standing"])
	}
	// The count is the campaign's truth, not the filtered page's: a
	// reviewer filtering must not lose sight of what awaits them.
	if body["pending_review"] != float64(2) {
		t.Fatalf("pending_review = %v under a filter, want the campaign's 2", body["pending_review"])
	}
}

func TestTheRosterRequiresBeingOnTheCampaign(t *testing.T) {
	rec := &stubRecruiting{detailErr: api.ErrCampaignNoAccess}
	roster := &stubRoster{}
	handler := serveRoster(t, rec, roster)

	status, _ := campaignRequest(t, handler, http.MethodGet, rosterPath, "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: existence is not answered across campaigns", status)
	}
	if roster.sawCampaign != "" {
		t.Fatalf("the roster was read for a campaign the caller is not on")
	}
}
