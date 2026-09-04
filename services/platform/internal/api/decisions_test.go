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

// REV-03 from the wire's side: the decider is the session, never the body,
// every refusal keeps its own code, and the history serves each decision
// with its true actor, its overrides and the evidence version it read.

type stubDecisions struct {
	recorded api.ReviewDecisionView
	history  []api.ReviewDecisionView
	err      error

	sawDecidedBy string
	sawDecision  string
	sawReason    string
	sawOverrides []api.OverrideInput
}

func (s *stubDecisions) Record(_ context.Context, tenantID, campaignID, sessionID, decidedBy, decision, reason string, overrides []api.OverrideInput) (api.ReviewDecisionView, error) {
	s.sawDecidedBy, s.sawDecision, s.sawReason = decidedBy, decision, reason
	s.sawOverrides = overrides
	if s.err != nil {
		return api.ReviewDecisionView{}, s.err
	}
	return s.recorded, nil
}

func (s *stubDecisions) History(_ context.Context, tenantID, campaignID, sessionID string) ([]api.ReviewDecisionView, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.history, nil
}

func sampleDecision() api.ReviewDecisionView {
	return api.ReviewDecisionView{
		ID:           "00000000-0000-7000-8000-00000000d001",
		SessionID:    "00000000-0000-7000-8000-00000000a001",
		DecidedBy:    progressionUser,
		Decision:     "hold",
		Reason:       "waiting on the take-home",
		EvaluationID: "00000000-0000-7000-8000-00000000ee01",
		ResultDigest: "sha256:result",
		RubricDigest: "sha256:rubric",
		Overrides: []api.BandOverrideDecided{{
			CompetencyID: "sd", RecordedBand: "strong", OverrideBand: "solid",
			Rationale: "one incident restated three times",
		}},
		DecidedAt: time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC),
	}
}

func serveDecisions(t *testing.T, recruiting *stubRecruiting, decisions *stubDecisions) http.Handler {
	t.Helper()
	identity := &fakeIdentity{
		principal: api.Principal{UserID: progressionUser, ActiveTenantID: campaignTenant},
		allowed: []authz.Capability{
			authz.EvaluationReview, authz.EvaluationReadScreen,
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
		Invitations: defaultStubInvitations(), Decisions: decisions,
		Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

const decisionsPath = "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123" +
	"/sessions/00000000-0000-7000-8000-00000000a001/decisions"

func TestTheDeciderIsTheSessionNeverTheBody(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	decisions := &stubDecisions{recorded: sampleDecision()}
	handler := serveDecisions(t, rec, decisions)

	status, body := campaignRequest(t, handler, http.MethodPost, decisionsPath,
		`{"decision":"hold","reason":"waiting on the take-home",
		  "overrides":[{"competency_id":"sd","band":"solid","rationale":"one incident restated"}]}`)
	if status != http.StatusCreated {
		t.Fatalf("status = %d: %v", status, body)
	}
	// The port saw the session's user; the contract has no decider field a
	// body could even try to supply.
	if decisions.sawDecidedBy != progressionUser {
		t.Fatalf("decider = %q, want the session user %q", decisions.sawDecidedBy, progressionUser)
	}
	if decisions.sawDecision != "hold" || decisions.sawReason != "waiting on the take-home" {
		t.Fatalf("port saw %q %q", decisions.sawDecision, decisions.sawReason)
	}
	if len(decisions.sawOverrides) != 1 || decisions.sawOverrides[0].Rationale != "one incident restated" {
		t.Fatalf("overrides = %+v", decisions.sawOverrides)
	}

	// The recorded band the reviewer disagreed with is on the answer.
	overrides, _ := body["overrides"].([]any)
	first, _ := overrides[0].(map[string]any)
	if first["recorded_band"] != "strong" || first["override_band"] != "solid" {
		t.Fatalf("override = %v", first)
	}
}

func TestEachDecisionRefusalKeepsItsCode(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{api.ErrDecisionNotReady, "REVIEW_NOT_READY"},
		{api.ErrDecisionReasonMissing, "REASON_REQUIRED"},
		{api.ErrOverrideIncompleteAPI, "OVERRIDE_INCOMPLETE"},
		{api.ErrOverrideUnknownCompetency, "OVERRIDE_UNKNOWN_COMPETENCY"},
		{api.ErrDecisionInvalid, "DECISION_INVALID"},
	}
	for _, test := range cases {
		handler := serveDecisions(t, &stubRecruiting{detail: openCampaignDetail()},
			&stubDecisions{err: test.err})

		status, body := campaignRequest(t, handler, http.MethodPost, decisionsPath,
			`{"decision":"hold","reason":"r"}`)
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

func TestTheHistoryServesEveryDecisionWithItsTrueActor(t *testing.T) {
	second := sampleDecision()
	second.ID = "00000000-0000-7000-8000-00000000d002"
	second.DecidedBy = "00000000-0000-7000-8000-00000000beef"
	second.Decision = "advance"
	handler := serveDecisions(t, &stubRecruiting{detail: openCampaignDetail()},
		&stubDecisions{history: []api.ReviewDecisionView{sampleDecision(), second}})

	status, body := campaignRequest(t, handler, http.MethodGet, decisionsPath, "")
	if status != http.StatusOK {
		t.Fatalf("status = %d: %v", status, body)
	}
	decisions, _ := body["decisions"].([]any)
	if len(decisions) != 2 {
		t.Fatalf("history = %d, want both decisions kept", len(decisions))
	}
	first, _ := decisions[0].(map[string]any)
	replaced, _ := decisions[1].(map[string]any)
	if first["decided_by"] == replaced["decided_by"] {
		t.Fatalf("actors collapsed: %v vs %v", first["decided_by"], replaced["decided_by"])
	}
	// The evidence version each decision was informed by survives to the
	// wire: what an appeal reads.
	if first["result_digest"] != "sha256:result" || first["evaluation_id"] == nil {
		t.Fatalf("evidence version missing: %v", first)
	}
}

func TestDecidingRequiresBeingOnTheCampaign(t *testing.T) {
	decisions := &stubDecisions{recorded: sampleDecision()}
	handler := serveDecisions(t, &stubRecruiting{detailErr: api.ErrCampaignNoAccess}, decisions)

	status, _ := campaignRequest(t, handler, http.MethodPost, decisionsPath,
		`{"decision":"advance","reason":"r"}`)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: existence is not answered across campaigns", status)
	}
	if decisions.sawDecision != "" {
		t.Fatalf("the port recorded for a campaign the caller is not on")
	}
}
