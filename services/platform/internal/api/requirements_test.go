package api_test

import (
	"net/http"
	"testing"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

const (
	jobContextPath   = "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/job-context"
	requirementsPath = "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/requirements"
	requirementPath  = "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/requirements/00000000-0000-7000-8000-00000000ee01"
)

// serveRequirements wires the recruiter requirement surface with a captured stub.
func serveRequirements(t *testing.T, rec *stubRecruiting, req *stubRequirements) http.Handler {
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
		Requirements: req, RecruiterAccommodations: defaultStubInvitations(),
		Recruiting: rec, Invitations: defaultStubInvitations(), Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

// Submitting a job description returns the extracted requirements, each with its
// provenance span.
func TestSubmitJobContextReturnsRequirements(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	req := &stubRequirements{requirements: []api.Requirement{
		{ID: "00000000-0000-7000-8000-00000000ee01", Text: "Five years of Go", SpanStart: 2, SpanEnd: 18, Status: "proposed"},
	}}
	handler := serveRequirements(t, rec, req)

	status, body := campaignRequest(t, handler, http.MethodPut, jobContextPath, `{"source_text":"- Five years of Go"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	list, _ := body["requirements"].([]any)
	if len(list) != 1 {
		t.Fatalf("requirements = %v", body["requirements"])
	}
	if req.sawSource != "- Five years of Go" {
		t.Fatalf("source not forwarded: %q", req.sawSource)
	}
	first := list[0].(map[string]any)
	if first["span_start"].(float64) != 2 || first["span_end"].(float64) != 18 {
		t.Fatalf("provenance span not served: %v", first)
	}
}

// Submitting to a campaign that has opened is a 409: its requirements are frozen.
func TestSubmitToAFrozenCampaignIs409(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	req := &stubRequirements{submitErr: api.ErrRequirementsFrozen}
	handler := serveRequirements(t, rec, req)

	status, body := campaignRequest(t, handler, http.MethodPut, jobContextPath, `{"source_text":"x"}`)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if code, _ := body["error"].(map[string]any)["code"].(string); code != "REQUIREMENTS_FROZEN" {
		t.Fatalf("code = %q", code)
	}
}

// Correcting a requirement forwards the id and returns it corrected.
func TestCorrectRequirement(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	req := &stubRequirements{corrected: api.Requirement{
		ID: "00000000-0000-7000-8000-00000000ee01", Text: "5+ years of Go", SpanStart: 2, SpanEnd: 18, Status: "corrected",
	}}
	handler := serveRequirements(t, rec, req)

	status, body := campaignRequest(t, handler, http.MethodPatch, requirementPath, `{"text":"5+ years of Go","status":"corrected"}`)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if body["status"] != "corrected" {
		t.Fatalf("status = %v", body["status"])
	}
	if req.sawCorrectID != "00000000-0000-7000-8000-00000000ee01" {
		t.Fatalf("requirement id not forwarded: %q", req.sawCorrectID)
	}
}

// Correcting a requirement not on the campaign is a 404.
func TestCorrectMissingRequirementIs404(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	req := &stubRequirements{correctErr: api.ErrRequirementMissing}
	handler := serveRequirements(t, rec, req)

	status, _ := campaignRequest(t, handler, http.MethodPatch, requirementPath, `{"status":"rejected"}`)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// The requirement surface requires being on the campaign: the join refuses first.
func TestRequirementsRequireBeingOnTheCampaign(t *testing.T) {
	rec := &stubRecruiting{detailErr: api.ErrCampaignNoAccess}
	req := defaultStubRequirements()
	handler := serveRequirements(t, rec, req)

	status, _ := campaignRequest(t, handler, http.MethodGet, requirementsPath, "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}
