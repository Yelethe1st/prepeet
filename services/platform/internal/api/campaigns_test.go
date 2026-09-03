package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

/*
 * SCR-01's HTTP surface, which is what its last criterion has been waiting
 * for: recruiter access scoped per campaign, enforced server-side, with a
 * request actually passing through it.
 *
 * The capability story is worth stating because it is not the obvious one.
 * Every operation declares campaign.read, including the writes, and that is a
 * consequence of the catalogue rather than a loosening: campaign.manage is
 * scoped to a campaign, Authorize never names a scope, so declaring manage
 * would deny everybody always. Per-campaign authority lives where it already
 * provably lives, in the campaign_recruiter join, and these tests hold the
 * handlers to routing everything about a particular campaign through it.
 */

const campaignTenant = "00000000-0000-7000-8000-00000000e001"

type stubRecruiting struct {
	created   *api.Campaign
	campaigns []api.Campaign
	detail    api.Campaign
	opened    *api.Campaign
	granted   []string

	detailErr error
	openErr   error
	grantErr  error

	sawTenant string
	sawUser   string
	sawPins   []api.PinChoice
}

func (s *stubRecruiting) CreateDraft(_ context.Context, campaign api.Campaign) (api.Campaign, error) {
	s.created = &campaign
	campaign.ID = "00000000-0000-7000-8000-00000000c123"
	campaign.Status = "draft"
	campaign.CreatedAt = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	return campaign, nil
}

func (s *stubRecruiting) List(_ context.Context, tenantID string) ([]api.Campaign, error) {
	s.sawTenant = tenantID
	return s.campaigns, nil
}

func (s *stubRecruiting) CampaignForRecruiter(_ context.Context, tenantID, campaignID, userID string) (api.Campaign, error) {
	s.sawTenant, s.sawUser = tenantID, userID
	if s.detailErr != nil {
		return api.Campaign{}, s.detailErr
	}
	return s.detail, nil
}

func (s *stubRecruiting) Open(_ context.Context, campaign api.Campaign, pins []api.PinChoice) (api.Campaign, error) {
	s.sawPins = pins
	if s.openErr != nil {
		return api.Campaign{}, s.openErr
	}
	if s.opened != nil {
		return *s.opened, nil
	}
	campaign.Status = "open"
	return campaign, nil
}

func (s *stubRecruiting) GrantAccess(_ context.Context, tenantID, campaignID, userID, grantedBy string) error {
	if s.grantErr != nil {
		return s.grantErr
	}
	s.granted = append(s.granted, userID+" by "+grantedBy)
	return nil
}

func draftCampaign() api.Campaign {
	return api.Campaign{
		ID: "00000000-0000-7000-8000-00000000c123", TenantID: campaignTenant,
		Name: "Backend hiring, Q4", Status: "draft",
		RoleReference: "role/backend", Jurisdiction: "GB",
		CreatedAt: time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
	}
}

func serveCampaigns(t *testing.T, stub *stubRecruiting) http.Handler {
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
		Invitations: defaultStubInvitations(), ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(), RecruiterAccommodations: defaultStubInvitations(), Recruiting: stub, Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

func campaignRequest(t *testing.T, handler http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("content-type", "application/json")
	}
	request.AddCookie(sessionCookie())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder.Code, decoded
}

func TestCreatingADraftNamesTheSessionAsCreator(t *testing.T) {
	// The creator comes from the session, never the body. A request-supplied
	// creator would let anybody put a campaign in a colleague's name, and the
	// creator is who joins the campaign in the creating transaction.
	stub := &stubRecruiting{}
	handler := serveCampaigns(t, stub)

	status, body := campaignRequest(t, handler, http.MethodPost, "/api/v1/campaigns",
		`{"name":"Backend hiring, Q4","role_reference":"role/backend","jurisdiction":"GB"}`)

	if status != http.StatusCreated {
		t.Fatalf("creating answered %d: %v", status, body)
	}
	if stub.created.CreatedBy != progressionUser {
		t.Fatalf("the creator is %q rather than the session", stub.created.CreatedBy)
	}
	if stub.created.TenantID != campaignTenant {
		t.Fatalf("the campaign landed in %q rather than the session's workspace", stub.created.TenantID)
	}
	if body["status"] != "draft" {
		t.Fatalf("a new campaign is %v rather than a draft", body["status"])
	}
}

func TestListingIsTenantWideAndSaysNothingAboutContents(t *testing.T) {
	// campaign.read is unscoped so a recruiter can see which campaigns exist
	// before being assigned to one. What must not leak is anything inside one.
	stub := &stubRecruiting{campaigns: []api.Campaign{draftCampaign()}}
	handler := serveCampaigns(t, stub)

	status, body := campaignRequest(t, handler, http.MethodGet, "/api/v1/campaigns", "")

	if status != http.StatusOK {
		t.Fatalf("listing answered %d", status)
	}
	if stub.sawTenant != campaignTenant {
		t.Fatalf("the list was read from %q rather than the session's workspace", stub.sawTenant)
	}
	entries := body["campaigns"].([]any)
	if len(entries) != 1 {
		t.Fatalf("want one campaign, got %d", len(entries))
	}
	if _, present := entries[0].(map[string]any)["pins"]; present {
		t.Fatal("the tenant-wide list leaks a campaign's pinned configuration")
	}
}

func TestACampaignTheCallerIsNotOnIsNotFound(t *testing.T) {
	// The same answer as a campaign that never existed. A member who can tell
	// "not yours" from "no such thing" can enumerate who is hiring for what.
	stub := &stubRecruiting{detailErr: api.ErrCampaignNoAccess}
	handler := serveCampaigns(t, stub)

	status, _ := campaignRequest(t, handler, http.MethodGet,
		"/api/v1/campaigns/00000000-0000-7000-8000-00000000c123", "")

	if status != http.StatusNotFound {
		t.Fatalf("a campaign the caller is not on answered %d, want 404", status)
	}
}

func TestReadingACampaignGoesThroughTheRecruiterJoin(t *testing.T) {
	stub := &stubRecruiting{detail: draftCampaign()}
	handler := serveCampaigns(t, stub)

	status, body := campaignRequest(t, handler, http.MethodGet,
		"/api/v1/campaigns/00000000-0000-7000-8000-00000000c123", "")

	if status != http.StatusOK {
		t.Fatalf("reading answered %d", status)
	}
	if stub.sawUser != progressionUser {
		t.Fatalf("the read was scoped to %q rather than the session's user", stub.sawUser)
	}
	if body["name"] != "Backend hiring, Q4" {
		t.Fatalf("the campaign did not survive: %v", body["name"])
	}
}

func TestOpeningRefusesWhereThereIsNoDetermination(t *testing.T) {
	// ADR-0020's refusal reaching the wire. Its own code, because the
	// recruiter's next step is different from every other failure here:
	// nothing they can do in the product will fix it.
	stub := &stubRecruiting{detail: draftCampaign(), openErr: api.ErrCampaignNoDetermination}
	handler := serveCampaigns(t, stub)

	status, body := campaignRequest(t, handler, http.MethodPost,
		"/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/open",
		`{"pins":[{"type":"rubric","reference":"rubric/backend"}]}`)

	if status != http.StatusUnprocessableEntity {
		t.Fatalf("a missing determination answered %d, want 422", status)
	}
	// The failure envelope nests under "error", per public-api.md's one shape
	// for every refusal.
	if code := body["error"].(map[string]any)["code"]; code != "NO_DETERMINATION" {
		t.Fatalf("the code is %v", code)
	}
}

func TestOpeningNamesTheUnpublishedArtifact(t *testing.T) {
	stub := &stubRecruiting{detail: draftCampaign(), openErr: api.ErrCampaignArtifactUnpublished}
	handler := serveCampaigns(t, stub)

	status, body := campaignRequest(t, handler, http.MethodPost,
		"/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/open",
		`{"pins":[{"type":"rubric","reference":"rubric/backend"}]}`)

	if status != http.StatusUnprocessableEntity {
		t.Fatalf("an unpublished artifact answered %d, want 422", status)
	}
	if code := body["error"].(map[string]any)["code"]; code != "ARTIFACT_NOT_PUBLISHED" {
		t.Fatalf("the code is %v", code)
	}
}

func TestOpeningTwiceIsAConflict(t *testing.T) {
	stub := &stubRecruiting{detail: draftCampaign(), openErr: api.ErrCampaignNotDraft}
	handler := serveCampaigns(t, stub)

	status, _ := campaignRequest(t, handler, http.MethodPost,
		"/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/open",
		`{"pins":[]}`)

	if status != http.StatusConflict {
		t.Fatalf("opening an open campaign answered %d, want 409", status)
	}
}

func TestOpeningACampaignTheCallerIsNotOnIsNotFound(t *testing.T) {
	// The join guards the write exactly as it guards the read. Without this, a
	// member could open a colleague's draft by knowing its identifier.
	stub := &stubRecruiting{detailErr: api.ErrCampaignNoAccess}
	handler := serveCampaigns(t, stub)

	status, _ := campaignRequest(t, handler, http.MethodPost,
		"/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/open",
		`{"pins":[]}`)

	if status != http.StatusNotFound {
		t.Fatalf("opening somebody else's campaign answered %d, want 404", status)
	}
	if stub.sawPins != nil {
		t.Fatal("the open reached the store though the caller is not on the campaign")
	}
}

func TestGrantingGoesThroughTheJoinAndNamesTheGranter(t *testing.T) {
	stub := &stubRecruiting{detail: draftCampaign()}
	handler := serveCampaigns(t, stub)

	status, _ := campaignRequest(t, handler, http.MethodPost,
		"/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/recruiters",
		`{"user_id":"00000000-0000-7000-8000-00000000feed"}`)

	if status != http.StatusNoContent {
		t.Fatalf("granting answered %d", status)
	}
	if len(stub.granted) != 1 ||
		stub.granted[0] != "00000000-0000-7000-8000-00000000feed by "+progressionUser {
		t.Fatalf("the grant is %v", stub.granted)
	}
}

func TestGrantingOnACampaignTheCallerIsNotOnIsNotFound(t *testing.T) {
	stub := &stubRecruiting{detailErr: api.ErrCampaignNoAccess}
	handler := serveCampaigns(t, stub)

	status, _ := campaignRequest(t, handler, http.MethodPost,
		"/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/recruiters",
		`{"user_id":"00000000-0000-7000-8000-00000000feed"}`)

	if status != http.StatusNotFound {
		t.Fatalf("granting on somebody else's campaign answered %d, want 404", status)
	}
	if len(stub.granted) != 0 {
		t.Fatal("the grant reached the store though the caller is not on the campaign")
	}
}

func TestCampaignsRefuseWithoutASession(t *testing.T) {
	handler := serveCampaigns(t, &stubRecruiting{})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/campaigns", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("no session answered %d", recorder.Code)
	}
}
