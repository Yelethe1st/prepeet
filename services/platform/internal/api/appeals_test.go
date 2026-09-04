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

// REV-06 from the wire's side: the requester and the resolver are the
// session, the freeze rides the answer, and every refusal keeps its code.

type stubAppeals struct {
	appeal api.ReReviewView
	list   []api.ReReviewView
	err    error

	sawRequestedBy string
	sawResolvedBy  string
	sawAssignee    string
	sawOutcome     string
}

func (s *stubAppeals) Raise(_ context.Context, tenantID, campaignID, sessionID, requestedBy, reason string) (api.ReReviewView, error) {
	s.sawRequestedBy = requestedBy
	if s.err != nil {
		return api.ReReviewView{}, s.err
	}
	return s.appeal, nil
}

func (s *stubAppeals) Assign(_ context.Context, tenantID, campaignID, appealID, assignee string) (api.ReReviewView, error) {
	s.sawAssignee = assignee
	if s.err != nil {
		return api.ReReviewView{}, s.err
	}
	return s.appeal, nil
}

func (s *stubAppeals) Resolve(_ context.Context, tenantID, campaignID, appealID, resolvedBy, outcome, rationale, disclosure string) (api.ReReviewView, error) {
	s.sawResolvedBy, s.sawOutcome = resolvedBy, outcome
	if s.err != nil {
		return api.ReReviewView{}, s.err
	}
	return s.appeal, nil
}

func (s *stubAppeals) List(_ context.Context, tenantID, campaignID, sessionID string) ([]api.ReReviewView, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.list, nil
}

func sampleAppeal() api.ReReviewView {
	return api.ReReviewView{
		ID:               "00000000-0000-7000-8000-00000000ee11",
		SessionID:        "00000000-0000-7000-8000-00000000a001",
		RequestedBy:      progressionUser,
		Reason:           "the second competency reads misjudged",
		AppealedDecision: "00000000-0000-7000-8000-00000000d001",
		OriginalReviewer: "00000000-0000-7000-8000-00000000ab01",
		Frozen: api.FrozenEvidenceView{
			EvaluationID: "00000000-0000-7000-8000-00000000ee01",
			ResultDigest: "sha256:result", RubricDigest: "sha256:rubric",
			BundleDigest: "sha256:bundle",
		},
		RaisedAt: time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC),
		DueAt:    time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC),
	}
}

func serveAppeals(t *testing.T, recruiting *stubRecruiting, appeals *stubAppeals) http.Handler {
	t.Helper()
	identity := &fakeIdentity{
		principal: api.Principal{UserID: progressionUser, ActiveTenantID: campaignTenant},
		allowed: []authz.Capability{
			authz.AppealRaise, authz.AppealManage, authz.EvaluationReadScreen,
		},
	}
	handler, err := api.NewServer(api.ServerConfig{
		Identity: identity, Candidates: &fakeCandidates{}, Documents: &fakeDocuments{},
		Catalog: &fakeCatalog{}, Interviews: &fakeInterviews{}, Members: &fakeMembers{},
		Billing: &fakeBilling{}, Progression: &stubProgression{},
		SensitiveReads: &recordingAuditor{}, Settings: &stubSettings{},
		ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(),
		ReInvitations: defaultStubInvitations(), Requirements: defaultStubRequirements(),
		RecruiterAccommodations: defaultStubInvitations(), Recruiting: recruiting,
		Invitations: defaultStubInvitations(), Appeals: appeals,
		Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

const appealsPath = "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123" +
	"/sessions/00000000-0000-7000-8000-00000000a001/re-reviews"

const appealActPath = "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123" +
	"/re-reviews/00000000-0000-7000-8000-00000000ee11"

func TestRaisingNamesTheSessionAndCarriesTheFreeze(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	appeals := &stubAppeals{appeal: sampleAppeal()}
	handler := serveAppeals(t, rec, appeals)

	status, body := campaignRequest(t, handler, http.MethodPost, appealsPath,
		`{"reason":"the second competency reads misjudged","requested_by":"00000000-0000-7000-8000-0000000evil0"}`)
	if status != http.StatusCreated {
		t.Fatalf("status = %d: %v", status, body)
	}
	// The requester is the session, whatever the body tried to claim.
	if appeals.sawRequestedBy != progressionUser {
		t.Fatalf("requester = %q, want the session user", appeals.sawRequestedBy)
	}
	// The freeze rides the answer: what the appealed decision read.
	frozen, _ := body["frozen"].(map[string]any)
	if frozen["result_digest"] != "sha256:result" || frozen["bundle_digest"] != "sha256:bundle" {
		t.Fatalf("frozen = %v", frozen)
	}
	if body["original_reviewer"] == nil || body["due_at"] == nil {
		t.Fatalf("body = %v", body)
	}
}

func TestResolvingNamesTheSessionAsResolver(t *testing.T) {
	resolved := sampleAppeal()
	resolution := api.ResolutionView{
		Outcome: "upheld", Rationale: "the evidence reads as recorded",
		Disclosure: "Your re-review is complete; the outcome stands.",
		ResolvedBy: progressionUser,
		ResolvedAt: time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC),
	}
	resolved.Resolution = &resolution
	appeals := &stubAppeals{appeal: resolved}
	handler := serveAppeals(t, &stubRecruiting{detail: openCampaignDetail()}, appeals)

	status, body := campaignRequest(t, handler, http.MethodPost, appealActPath+"/resolution",
		`{"outcome":"upheld","rationale":"the evidence reads as recorded","disclosure":"Your re-review is complete; the outcome stands."}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d: %v", status, body)
	}
	if appeals.sawResolvedBy != progressionUser || appeals.sawOutcome != "upheld" {
		t.Fatalf("port saw %q %q", appeals.sawResolvedBy, appeals.sawOutcome)
	}
	answer, _ := body["resolution"].(map[string]any)
	if answer["candidate_disclosure"] != "Your re-review is complete; the outcome stands." {
		t.Fatalf("resolution = %v", answer)
	}
}

func TestEachAppealRefusalKeepsItsCode(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{api.ErrAppealNothingDecided, "APPEAL_NO_DECISION"},
		{api.ErrAppealReasonMissing, "REASON_REQUIRED"},
		{api.ErrAppealSelfReviewAPI, "SELF_REVIEW_FORBIDDEN"},
		{api.ErrAppealDone, "APPEAL_RESOLVED"},
		{api.ErrAppealNotAssignee, "RESOLVER_NOT_ASSIGNED"},
		{api.ErrAppealIncomplete, "RESOLUTION_INCOMPLETE"},
	}
	for _, test := range cases {
		handler := serveAppeals(t, &stubRecruiting{detail: openCampaignDetail()},
			&stubAppeals{err: test.err})

		status, body := campaignRequest(t, handler, http.MethodPost, appealActPath+"/resolution",
			`{"outcome":"upheld","rationale":"r","disclosure":"d"}`)
		if status != http.StatusConflict {
			t.Errorf("%s answered %d, want 409", test.want, status)
			continue
		}
		errorBody, _ := body["error"].(map[string]any)
		if errorBody["code"] != test.want {
			t.Errorf("code = %v, want %q", errorBody["code"], test.want)
		}
	}
}

func TestAssignmentPassesTheAssigneeThrough(t *testing.T) {
	appeals := &stubAppeals{appeal: sampleAppeal()}
	handler := serveAppeals(t, &stubRecruiting{detail: openCampaignDetail()}, appeals)

	status, _ := campaignRequest(t, handler, http.MethodPost, appealActPath+"/assignment",
		`{"assignee":"00000000-0000-7000-8000-00000000cafe"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if appeals.sawAssignee != "00000000-0000-7000-8000-00000000cafe" {
		t.Fatalf("assignee = %q", appeals.sawAssignee)
	}
}
