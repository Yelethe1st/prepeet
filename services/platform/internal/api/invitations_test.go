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

// stubInvitations records what the handler asked and returns what the test set.
type stubInvitations struct {
	issued  api.Invitation
	list    []api.Invitation
	revoked api.Invitation
	resent  api.Invitation

	issueErr  error
	listErr   error
	revokeErr error
	resendErr error

	sawIssuedBy  string
	sawRecipient string
	sawCampaign  api.Campaign
	sawRevokeID  string
	sawResendID  string
	sawDecideID  string
	decideErr    error
}

func (s *stubInvitations) DecideAccommodation(_ context.Context, tenantID, campaignID, requestID, decidedBy string, granted bool) error {
	s.sawDecideID = requestID
	return s.decideErr
}

func (s *stubInvitations) Issue(_ context.Context, campaign api.Campaign, recipient, issuedBy string) (api.Invitation, error) {
	s.sawCampaign, s.sawRecipient, s.sawIssuedBy = campaign, recipient, issuedBy
	if s.issueErr != nil {
		return api.Invitation{}, s.issueErr
	}
	return s.issued, nil
}

func (s *stubInvitations) ListInvitations(_ context.Context, tenantID, campaignID string) ([]api.Invitation, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.list, nil
}

func (s *stubInvitations) Revoke(_ context.Context, tenantID, campaignID, invitationID string) (api.Invitation, error) {
	s.sawRevokeID = invitationID
	if s.revokeErr != nil {
		return api.Invitation{}, s.revokeErr
	}
	return s.revoked, nil
}

func (s *stubInvitations) Resend(_ context.Context, campaign api.Campaign, invitationID, issuedBy string) (api.Invitation, error) {
	s.sawResendID, s.sawIssuedBy = invitationID, issuedBy
	if s.resendErr != nil {
		return api.Invitation{}, s.resendErr
	}
	return s.resent, nil
}

// defaultStubInvitations satisfies the port for the many server configs that
// do not exercise invitations; it answers a canned invitation so a stray call
// is visible rather than a nil panic.
func defaultStubInvitations() *stubInvitations {
	return &stubInvitations{issued: sampleInvitation(), revoked: sampleInvitation(), resent: sampleInvitation()}
}

func sampleInvitation() api.Invitation {
	return api.Invitation{
		ID:         "00000000-0000-7000-8000-00000000e001",
		CampaignID: "00000000-0000-7000-8000-00000000c123",
		Recipient:  "candidate@example.com",
		Status:     "live",
		IssuedAt:   time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC),
		ExpiresAt:  time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC),
		Delivery:   api.InvitationDelivery{Status: "pending"},
	}
}

// openCampaignDetail is what CampaignForRecruiter returns to admit the caller.
func openCampaignDetail() api.Campaign {
	c := draftCampaign()
	c.Status = "open"
	return c
}

func serveInvitations(t *testing.T, recruiting *stubRecruiting, invitations *stubInvitations) http.Handler {
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
		ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(), RecruiterAccommodations: invitations, Recruiting: recruiting, Invitations: invitations, Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

const invitationsPath = "/api/v1/campaigns/00000000-0000-7000-8000-00000000c123/invitations"

// The issuer is the session, never the body: an invitation is an
// accountability record and a body-supplied issuer would let anyone act in a
// colleague's name.
func TestIssuingNamesTheSessionAsIssuer(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	inv := defaultStubInvitations()
	handler := serveInvitations(t, rec, inv)

	status, _ := campaignRequest(t, handler, http.MethodPost, invitationsPath,
		`{"recipient":"candidate@example.com","issued_by":"00000000-0000-7000-8000-0000000evil0"}`)
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", status)
	}
	if inv.sawIssuedBy != progressionUser {
		t.Fatalf("issuer = %q, want the session user %q", inv.sawIssuedBy, progressionUser)
	}
	if inv.sawRecipient != "candidate@example.com" {
		t.Fatalf("recipient = %q", inv.sawRecipient)
	}
}

// The token never appears in a response. It exists for the one email and is
// never returned, so no response body may carry it.
func TestTheInvitationResponseNeverCarriesAToken(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	handler := serveInvitations(t, rec, defaultStubInvitations())

	_, body := campaignRequest(t, handler, http.MethodPost, invitationsPath,
		`{"recipient":"candidate@example.com"}`)
	for _, key := range []string{"token", "token_hash", "link", "plaintext"} {
		if _, present := body[key]; present {
			t.Fatalf("the response carried a %q field", key)
		}
	}
}

// A recruiter not on the campaign cannot issue: the join refuses first, as a
// 404 that does not reveal the campaign exists.
func TestIssuingRequiresBeingOnTheCampaign(t *testing.T) {
	rec := &stubRecruiting{detailErr: api.ErrCampaignNoAccess}
	inv := defaultStubInvitations()
	handler := serveInvitations(t, rec, inv)

	status, _ := campaignRequest(t, handler, http.MethodPost, invitationsPath,
		`{"recipient":"candidate@example.com"}`)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if inv.sawRecipient != "" {
		t.Fatal("the invitation was issued despite the caller not being on the campaign")
	}
}

// Issuing against a campaign that is not open is a 409 with its own code, so
// the recruiter is told to open it rather than shown a generic failure.
func TestIssuingANotOpenCampaignIs409(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	inv := defaultStubInvitations()
	inv.issueErr = api.ErrCampaignNotOpen
	handler := serveInvitations(t, rec, inv)

	status, body := campaignRequest(t, handler, http.MethodPost, invitationsPath,
		`{"recipient":"candidate@example.com"}`)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if code, _ := body["error"].(map[string]any)["code"].(string); code != "CAMPAIGN_NOT_OPEN" {
		t.Fatalf("code = %q, want CAMPAIGN_NOT_OPEN", code)
	}
}

// Listing requires being on the campaign, the same join as reading it.
func TestListingRequiresBeingOnTheCampaign(t *testing.T) {
	rec := &stubRecruiting{detailErr: api.ErrCampaignNoAccess}
	handler := serveInvitations(t, rec, defaultStubInvitations())

	status, _ := campaignRequest(t, handler, http.MethodGet, invitationsPath, "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// Revoking requires being on the campaign; a member of the tenant who is not on
// this campaign cannot revoke by knowing an id.
func TestRevokingRequiresBeingOnTheCampaign(t *testing.T) {
	rec := &stubRecruiting{detailErr: api.ErrCampaignNoAccess}
	inv := defaultStubInvitations()
	handler := serveInvitations(t, rec, inv)

	status, _ := campaignRequest(t, handler, http.MethodPost,
		invitationsPath+"/00000000-0000-7000-8000-00000000e001/revoke", "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
	if inv.sawRevokeID != "" {
		t.Fatal("a revoke reached the port for a caller not on the campaign")
	}
}

// Revoking a live invitation returns it, now revoked.
func TestRevokeReturnsTheRevokedInvitation(t *testing.T) {
	revoked := sampleInvitation()
	revoked.Status = "revoked"
	rec := &stubRecruiting{detail: openCampaignDetail()}
	inv := defaultStubInvitations()
	inv.revoked = revoked
	handler := serveInvitations(t, rec, inv)

	status, body := campaignRequest(t, handler, http.MethodPost,
		invitationsPath+"/00000000-0000-7000-8000-00000000e001/revoke", "")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if got, _ := body["status"].(string); got != "revoked" {
		t.Fatalf("status = %q, want revoked", got)
	}
}

// Resend refuses an invitation the candidate has already answered: a 409 with
// its own code, because a new link there would overwrite a recorded decision.
func TestResendRefusesAnAnsweredInvitation(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	inv := defaultStubInvitations()
	inv.resendErr = api.ErrInvitationNotResendable
	handler := serveInvitations(t, rec, inv)

	status, body := campaignRequest(t, handler, http.MethodPost,
		invitationsPath+"/00000000-0000-7000-8000-00000000e001/resend", "")
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if code, _ := body["error"].(map[string]any)["code"].(string); code != "INVITATION_NOT_RESENDABLE" {
		t.Fatalf("code = %q, want INVITATION_NOT_RESENDABLE", code)
	}
}

// Resend of an unknown invitation is a 404.
func TestResendMissingInvitationIs404(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	inv := defaultStubInvitations()
	inv.resendErr = api.ErrInvitationMissing
	handler := serveInvitations(t, rec, inv)

	status, _ := campaignRequest(t, handler, http.MethodPost,
		invitationsPath+"/00000000-0000-7000-8000-00000000e001/resend", "")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// A resend that succeeds returns the fresh invitation, issued by the session.
func TestResendReturnsAFreshInvitation(t *testing.T) {
	rec := &stubRecruiting{detail: openCampaignDetail()}
	inv := defaultStubInvitations()
	handler := serveInvitations(t, rec, inv)

	status, _ := campaignRequest(t, handler, http.MethodPost,
		invitationsPath+"/00000000-0000-7000-8000-00000000e001/resend", "")
	if status != http.StatusCreated {
		t.Fatalf("status = %d, want 201", status)
	}
	if inv.sawIssuedBy != progressionUser {
		t.Fatalf("resend issuer = %q, want session user", inv.sawIssuedBy)
	}
}
