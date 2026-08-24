package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
)

// Choosing a workspace.
//
// Every request operates under exactly one explicit active tenant, never one
// inferred from a resource identifier. These assert the HTTP half of that: that
// the selection is an act of its own, that refusing it does not sign anybody
// out, and that refusing it does not say whether the workspace exists.

const otherTenantID = "01a0301d-aa10-7000-8f3e-999999999999"

func TestListingMembershipsRequiresASession(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := get(t, serve(t, identity), "/api/v1/me/memberships")

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if len(identity.lookedUp) != 0 {
		t.Error("a request with no session cookie still reached the service")
	}
}

func TestMembershipsAreAnEmptyArrayRatherThanNull(t *testing.T) {
	t.Parallel()

	response := get(t, serve(t, workingIdentity()), "/api/v1/me/memberships",
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_live"})

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body)
	}

	var body struct {
		Memberships []struct {
			TenantID   string `json:"tenant_id"`
			TenantName string `json:"tenant_name"`
			Status     string `json:"status"`
		} `json:"memberships"`
	}
	decode(t, response, &body)

	// Null and [] are two shapes for the same fact, and a client should not
	// have to handle both.
	if body.Memberships == nil {
		t.Error("memberships is null rather than an empty array")
	}
}

func TestMembershipsAreListed(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.user.Memberships = []api.Membership{
		{TenantID: tenantID, TenantName: "Northwind Recruiting", Status: "active"},
	}

	response := get(t, serve(t, identity), "/api/v1/me/memberships",
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_live"})

	var body struct {
		Memberships []struct {
			TenantID   string `json:"tenant_id"`
			TenantName string `json:"tenant_name"`
			Status     string `json:"status"`
		} `json:"memberships"`
	}
	decode(t, response, &body)

	if len(body.Memberships) != 1 || body.Memberships[0].TenantName != "Northwind Recruiting" {
		t.Errorf("memberships = %+v", body.Memberships)
	}
}

// ────────────────────────────────────────────────── selecting a workspace

func TestSelectingATenantRequiresASession(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := put(t, serve(t, identity), "/api/v1/me/active-tenant",
		`{"tenant_id":"`+tenantID+`"}`)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if len(identity.selected) != 0 {
		t.Error("a request with no session still reached the service")
	}
}

func TestSelectingATenantReportsTheNewScope(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := put(t, serve(t, identity), "/api/v1/me/active-tenant",
		`{"tenant_id":"`+tenantID+`"}`,
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_live"})

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body)
	}

	var session struct {
		ActiveTenantID  *string `json:"active_tenant_id"`
		AuthenticatedAt string  `json:"authenticated_at"`
		ExpiresAt       string  `json:"expires_at"`
		UserID          string  `json:"user_id"`
	}
	decode(t, response, &session)

	if session.ActiveTenantID == nil || *session.ActiveTenantID != tenantID {
		t.Errorf("active_tenant_id = %v, want %q", session.ActiveTenantID, tenantID)
	}
}

// A null tenant clears the selection, which is how somebody leaves a workspace
// without signing out.
func TestANullTenantClearsTheSelection(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := put(t, serve(t, identity), "/api/v1/me/active-tenant", `{"tenant_id":null}`,
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_live"})

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body)
	}
	if len(identity.selected) != 1 || identity.selected[0] != "" {
		t.Errorf("the service was asked for %v, want the selection cleared", identity.selected)
	}
}

/*
The status code here is the whole decision.

403 rather than 404, because the workspace may well exist and answering "no such
thing" to somebody who cannot use it would be a way to test which identifiers
are real. 403 rather than 401, because the session is fine: answering 401 would
sign somebody out for clicking a workspace that is not theirs.
*/
func TestSelectingAWorkspaceYouDoNotBelongToIsForbidden(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.selectErr = api.ErrForbidden

	response := put(t, serve(t, identity), "/api/v1/me/active-tenant",
		`{"tenant_id":"`+otherTenantID+`"}`,
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_live"})

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", response.Code, response.Body)
	}
}

func TestARefusedSelectionDoesNotClearTheCookies(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.selectErr = api.ErrForbidden

	response := put(t, serve(t, identity), "/api/v1/me/active-tenant",
		`{"tenant_id":"`+otherTenantID+`"}`,
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_live"})

	// Being refused a workspace is not being signed out, and clearing the
	// cookies would make it look like one.
	if len(cookiesOf(t, response)) != 0 {
		t.Errorf("a refused selection cleared the session cookies: %v", cookiesOf(t, response))
	}
}

// The refusal must not say whether the workspace exists, which is the reason it
// is 403 rather than 404 in the first place.
func TestARefusedSelectionSaysNothingAboutWhetherTheWorkspaceExists(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.selectErr = api.ErrForbidden

	response := put(t, serve(t, identity), "/api/v1/me/active-tenant",
		`{"tenant_id":"`+otherTenantID+`"}`,
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_live"})

	body := strings.ToLower(response.Body.String())
	for _, leak := range []string{"not found", "no such", "does not exist", "unknown workspace"} {
		if strings.Contains(body, leak) {
			t.Errorf("the refusal says %q, which tells the caller whether the identifier is real: %s",
				leak, response.Body)
		}
	}
}

// A dead session is still a dead session, and must answer as one rather than as
// a forbidden workspace.
func TestSelectingWithADeadSessionIsUnauthenticated(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.selectErr = api.ErrSessionRejected

	response := put(t, serve(t, identity), "/api/v1/me/active-tenant",
		`{"tenant_id":"`+tenantID+`"}`,
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_revoked"})

	if response.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", response.Code)
	}
}

// ─────────────────────────────────────────────────────── what a session may do

/*
Capabilities on /me.

They exist so an interface can avoid offering somebody a control that will
refuse them. They are never what stops access: the server authorises every
request against the same set, and one for something not held is denied whether
or not anything was rendered.

The set is a property of the session, not of the person, because it depends on
which workspace they are acting under.
*/

func TestCurrentUserReportsWhatTheSessionMayDo(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.user.Capabilities = []string{"campaign.read", "invitation.read"}

	response := get(t, serve(t, identity), "/api/v1/me",
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_live"})

	var body struct {
		Capabilities []string `json:"capabilities"`
	}
	decodeInto(t, response, &body)

	if len(body.Capabilities) != 2 {
		t.Fatalf("capabilities = %v", body.Capabilities)
	}
}

// Empty rather than null, for the same reason memberships are: a client should
// not have to handle two shapes for "may do nothing here".
func TestCapabilitiesAreAnEmptyArrayRatherThanNull(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.user.Capabilities = nil

	response := get(t, serve(t, identity), "/api/v1/me",
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_live"})

	var body struct {
		Capabilities []string `json:"capabilities"`
	}
	decodeInto(t, response, &body)

	if body.Capabilities == nil {
		t.Error("capabilities is null rather than an empty array")
	}
}

/*
The active tenant on /me comes from the session, not from the person's record.

It was null after a successful selection, while the capabilities in the same
response were the workspace ones. An interface reading that would show somebody
recruiter controls and no indication of which workspace they were in, which is
the worst combination: authority without context.

Found on the wire rather than by any test, because both halves were individually
correct and only the response was wrong.
*/
func TestCurrentUserReportsTheSessionsActiveTenant(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.principal.ActiveTenantID = tenantID
	// Deliberately absent from the person's record, which is where it was being
	// read from and where it never is.
	identity.user.ActiveTenantID = ""

	response := get(t, serve(t, identity), "/api/v1/me",
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_live"})

	var body struct {
		ActiveTenantID *string `json:"active_tenant_id"`
	}
	decodeInto(t, response, &body)

	if body.ActiveTenantID == nil || *body.ActiveTenantID != tenantID {
		t.Errorf("active_tenant_id = %v, want %q", body.ActiveTenantID, tenantID)
	}
}

func TestCurrentUserReportsNoTenantWhenNoneIsChosen(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.principal.ActiveTenantID = ""

	response := get(t, serve(t, identity), "/api/v1/me",
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_live"})

	var body struct {
		ActiveTenantID *string `json:"active_tenant_id"`
	}
	decodeInto(t, response, &body)

	if body.ActiveTenantID != nil {
		t.Errorf("active_tenant_id = %q for a session that has chosen nothing", *body.ActiveTenantID)
	}
}
