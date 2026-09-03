package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// The member administration surface. What matters here: every operation
// authorizes through the one policy path with the capability it needs -
// reading with member_read, changing with member_manage - the active tenant
// and the acting administrator reach the port from the session and never
// from the request, and each refusal keeps its own status.

type fakeMembers struct {
	members []api.TenantMember
	member  api.TenantMember
	err     error

	invited []string
	changed []string
	revoked []string
	tenants []string
}

func (f *fakeMembers) List(_ context.Context, tenantID string) ([]api.TenantMember, error) {
	f.tenants = append(f.tenants, tenantID)
	return f.members, f.err
}

func (f *fakeMembers) Invite(_ context.Context, tenantID, actorID, email, role string) (api.TenantMember, error) {
	f.tenants = append(f.tenants, tenantID)
	f.invited = append(f.invited, actorID+":"+email+":"+role)
	return f.member, f.err
}

func (f *fakeMembers) ChangeRole(_ context.Context, tenantID, actorID, membershipID, role string, expectedVersion int) (api.TenantMember, error) {
	f.tenants = append(f.tenants, tenantID)
	f.changed = append(f.changed, membershipID+":"+role)
	return f.member, f.err
}

func (f *fakeMembers) Revoke(_ context.Context, tenantID, actorID, membershipID string, expectedVersion int) error {
	f.tenants = append(f.tenants, tenantID)
	f.revoked = append(f.revoked, membershipID)
	return f.err
}

// authorizingIdentity records which capability each request asked for.
type authorizingIdentity struct {
	fakeIdentity
	asked  []string
	refuse error
}

func (f *authorizingIdentity) Authorize(_ context.Context, _ string, capability string) (api.Principal, error) {
	f.asked = append(f.asked, capability)
	if f.refuse != nil {
		return api.Principal{}, f.refuse
	}
	return f.principal, nil
}

func serveMembers(t *testing.T, identity *authorizingIdentity, members *fakeMembers) http.Handler {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		Identity:    identity,
		Candidates:  &fakeCandidates{},
		Documents:   &fakeDocuments{},
		Catalog:     &fakeCatalog{},
		Interviews:  &fakeInterviews{},
		Members:     members,
		Billing:     &fakeBilling{},
		Settings:    &stubSettings{},
		Invitations: defaultStubInvitations(), ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(), ReInvitations: defaultStubInvitations(), Requirements: defaultStubRequirements(), RecruiterAccommodations: defaultStubInvitations(), Recruiting: &stubRecruiting{},
		SensitiveReads: &recordingAuditor{},
		Progression:    &stubProgression{},
		Environment:    config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

func adminIdentity() *authorizingIdentity {
	return &authorizingIdentity{fakeIdentity: fakeIdentity{principal: api.Principal{
		UserID: "00000000-0000-7000-8000-0000000000a1", ActiveTenantID: "00000000-0000-7000-8000-0000000010e1",
	}}}
}

func aMember() api.TenantMember {
	return api.TenantMember{
		MembershipID: "00000000-0000-7000-8000-0000000000b1",
		UserID:       "00000000-0000-7000-8000-0000000000c1",
		Email:        "priya@example.com", Role: "recruiter", Status: "invited",
		Version: 1, CreatedAt: time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC),
	}
}

func TestListingAsksForMemberReadAndScopesToTheSessionsTenant(t *testing.T) {
	identity := adminIdentity()
	members := &fakeMembers{members: []api.TenantMember{aMember()}}
	handler := serveMembers(t, identity, members)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/tenant/members", nil)
	request.AddCookie(sessionCookie())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}

	if len(identity.asked) != 1 || identity.asked[0] != "tenant.member_read" {
		t.Fatalf("asked for %v, want tenant.member_read", identity.asked)
	}
	if members.tenants[0] != "00000000-0000-7000-8000-0000000010e1" {
		t.Fatalf("the port saw tenant %s; the session decides, never the request", members.tenants[0])
	}
	var body struct {
		Members []struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"members"`
	}
	decodeInto(t, response, &body)
	if len(body.Members) != 1 || body.Members[0].Role != "recruiter" {
		t.Fatalf("body = %+v", body)
	}
}

func TestWritingAsksForMemberManage(t *testing.T) {
	identity := adminIdentity()
	members := &fakeMembers{member: aMember()}
	handler := serveMembers(t, identity, members)

	response := post(t, handler, "/api/v1/tenant/members",
		`{"email":"priya@example.com","role":"recruiter"}`, sessionCookie())
	if response.Code != http.StatusCreated {
		t.Fatalf("invite status %d: %s", response.Code, response.Body)
	}
	if identity.asked[0] != "tenant.member_manage" {
		t.Fatalf("invite asked for %v", identity.asked)
	}
	if members.invited[0] != "00000000-0000-7000-8000-0000000000a1:priya@example.com:recruiter" {
		t.Fatalf("the port saw %v", members.invited)
	}
}

func TestARefusedCapabilityIs403WithoutTheReason(t *testing.T) {
	identity := adminIdentity()
	identity.refuse = api.ErrForbidden
	members := &fakeMembers{}
	handler := serveMembers(t, identity, members)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/tenant/members", nil)
	request.AddCookie(sessionCookie())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", response.Code)
	}
	if len(members.tenants) != 0 {
		t.Fatal("the refused request reached the port")
	}
}

func TestTheRefusalsKeepTheirOwnStatuses(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{api.ErrMemberMissing, http.StatusNotFound},
		{api.ErrMemberConflict, http.StatusConflict},
		{api.Invalid("role", "MEMBER_ROLE_INVALID", "not assignable"), http.StatusBadRequest},
	}
	for _, test := range cases {
		identity := adminIdentity()
		handler := serveMembers(t, identity, &fakeMembers{err: test.err})

		response := doJSON(t, handler, http.MethodPatch,
			"/api/v1/tenant/members/00000000-0000-7000-8000-0000000000b1",
			`{"role":"viewer","expected_version":1}`, sessionCookie())
		if response.Code != test.want {
			t.Errorf("%v answered %d, want %d: %s", test.err, response.Code, test.want, response.Body)
		}
	}
}

func TestRevokeCarriesTheGuardAndAnswersNoContent(t *testing.T) {
	identity := adminIdentity()
	members := &fakeMembers{}
	handler := serveMembers(t, identity, members)

	response := doJSON(t, handler, http.MethodDelete,
		"/api/v1/tenant/members/00000000-0000-7000-8000-0000000000b1?expectedVersion=3", "", sessionCookie())
	if response.Code != http.StatusNoContent {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if members.revoked[0] != "00000000-0000-7000-8000-0000000000b1" {
		t.Fatalf("revoked %v", members.revoked)
	}
}

// doJSON performs one request with a JSON body.
func doJSON(t *testing.T, handler http.Handler, method, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// fakeBilling serves the ledger port.
type fakeBilling struct {
	usage api.TenantUsage
	err   error
}

func (f *fakeBilling) Usage(_ context.Context, _ string) (api.TenantUsage, error) {
	return f.usage, f.err
}

func TestUsageAndQuotaAskForBillingRead(t *testing.T) {
	limit, remaining := 50, 10
	identity := adminIdentity()
	handler, billing := serveBilling(t, identity, &fakeBilling{usage: api.TenantUsage{
		Started: 42, Credited: 2, Billable: 40,
		Limit: &limit, Remaining: &remaining, WarnThreshold: 0.8, Warning: "approaching",
	}})
	_ = billing

	response := doJSON(t, handler, http.MethodGet, "/api/v1/tenant/usage", "", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("usage status %d: %s", response.Code, response.Body)
	}
	if identity.asked[0] != "tenant.billing_read" {
		t.Fatalf("usage asked for %v", identity.asked)
	}
	var usage struct {
		Billable int `json:"billable"`
	}
	decodeInto(t, response, &usage)
	if usage.Billable != 40 {
		t.Fatalf("usage = %+v", usage)
	}

	response = doJSON(t, handler, http.MethodGet, "/api/v1/tenant/quota", "", sessionCookie())
	if response.Code != http.StatusOK {
		t.Fatalf("quota status %d: %s", response.Code, response.Body)
	}
	var quota struct {
		SessionLimit int    `json:"session_limit"`
		Remaining    int    `json:"remaining"`
		Warning      string `json:"warning"`
	}
	decodeInto(t, response, &quota)
	if quota.SessionLimit != 50 || quota.Remaining != 10 || quota.Warning != "approaching" {
		t.Fatalf("quota = %+v", quota)
	}
}

func serveBilling(t *testing.T, identity *authorizingIdentity, billing *fakeBilling) (http.Handler, *fakeBilling) {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		Identity:       identity,
		Candidates:     &fakeCandidates{},
		Documents:      &fakeDocuments{},
		Catalog:        &fakeCatalog{},
		Interviews:     &fakeInterviews{},
		Members:        &fakeMembers{},
		Billing:        billing,
		Progression:    &stubProgression{},
		SensitiveReads: &recordingAuditor{},
		Settings:       &stubSettings{},
		Invitations:    defaultStubInvitations(), ScreeningInvitations: defaultStubScreening(), CandidateAccommodations: defaultStubScreening(), ReInvitations: defaultStubInvitations(), Requirements: defaultStubRequirements(), RecruiterAccommodations: defaultStubInvitations(), Recruiting: &stubRecruiting{},
		Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler, billing
}
