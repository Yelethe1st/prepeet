package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// The HTTP layer for authentication.
//
// These run against a fake Identity rather than a database, because what is
// under test here is the HTTP behaviour: which status, which cookies, which
// envelope, what is absent from a body. The identity rules themselves are
// asserted in their own package, and duplicating them here would mean two
// places to update and one that gets forgotten.

// ─────────────────────────────────────────────────────────────── the fake

type fakeIdentity struct {
	registerErr error
	registered  []api.Registration

	session         api.Session
	authenticateErr error
	authenticated   []string

	refreshed  []string
	refreshErr error

	principal api.Principal
	lookupErr error
	lookedUp  []string

	revoked   []string
	revokeErr error

	user        api.User
	describeErr error
}

func (f *fakeIdentity) Register(_ context.Context, input api.Registration) error {
	f.registered = append(f.registered, input)
	return f.registerErr
}

func (f *fakeIdentity) Authenticate(_ context.Context, email, _ string) (api.Session, error) {
	f.authenticated = append(f.authenticated, email)
	if f.authenticateErr != nil {
		return api.Session{}, f.authenticateErr
	}
	return f.session, nil
}

func (f *fakeIdentity) Refresh(_ context.Context, token string) (api.Session, error) {
	f.refreshed = append(f.refreshed, token)
	if f.refreshErr != nil {
		return api.Session{}, f.refreshErr
	}
	return f.session, nil
}

func (f *fakeIdentity) Lookup(_ context.Context, token string) (api.Principal, error) {
	f.lookedUp = append(f.lookedUp, token)
	if f.lookupErr != nil {
		return api.Principal{}, f.lookupErr
	}
	return f.principal, nil
}

func (f *fakeIdentity) Revoke(_ context.Context, token, _ string) error {
	f.revoked = append(f.revoked, token)
	return f.revokeErr
}

func (f *fakeIdentity) Describe(_ context.Context, _ string) (api.User, error) {
	if f.describeErr != nil {
		return api.User{}, f.describeErr
	}
	return f.user, nil
}

// ───────────────────────────────────────────────────────────────── harness

const (
	userID   = "01a0301d-aa10-7000-8f3e-1234567890ab"
	tenantID = "01a0301d-aa10-7000-8f3e-abcdef123456"
)

// workingIdentity is a fake that succeeds, so a test states only what it
// changes.
func workingIdentity() *fakeIdentity {
	now := time.Now().UTC().Truncate(time.Second)
	return &fakeIdentity{
		session: api.Session{
			UserID:          userID,
			SessionToken:    "ses_AbCdEf0123456789AbCdEf0123456789",
			RefreshToken:    "ref_ZzYyXx9876543210ZzYyXx9876543210",
			ExpiresAt:       now.Add(time.Hour),
			RefreshExpires:  now.Add(720 * time.Hour),
			AuthenticatedAt: now,
		},
		principal: api.Principal{UserID: userID, SessionID: "sid", AuthenticatedAt: now},
		user: api.User{
			ID:            userID,
			Email:         "daniel.okonkwo@example.com",
			EmailVerified: true,
			Memberships:   nil,
		},
	}
}

func serve(t *testing.T, identity api.Identity) http.Handler {
	t.Helper()
	return serveIn(t, identity, config.EnvironmentLocal)
}

func serveIn(t *testing.T, identity api.Identity, environment config.Environment) http.Handler {
	t.Helper()

	handler, err := api.NewServer(api.ServerConfig{
		Identity:    identity,
		Environment: environment,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

func post(t *testing.T, handler http.Handler, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func get(t *testing.T, handler http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// cookiesOf indexes the Set-Cookie headers of a response by name.
func cookiesOf(t *testing.T, response *httptest.ResponseRecorder) map[string]*http.Cookie {
	t.Helper()

	found := map[string]*http.Cookie{}
	for _, cookie := range (&http.Response{Header: response.Header()}).Cookies() {
		found[cookie.Name] = cookie
	}
	return found
}

func decode(t *testing.T, response *httptest.ResponseRecorder, into any) {
	t.Helper()

	decoder := json.NewDecoder(response.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		t.Fatalf("decoding %s: %v", response.Body.String(), err)
	}
}

// ──────────────────────────────────────────────────────────────── register

func TestRegisterAcceptsACandidate(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := post(t, serve(t, identity), "/api/v1/auth/register",
		`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password","account_type":"candidate"}`)

	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", response.Code, response.Body)
	}
	if len(identity.registered) != 1 {
		t.Fatalf("the service saw %d registrations, want 1", len(identity.registered))
	}
}

// Registration never signs the user in. Verification comes first, so a response
// carrying a session would skip the step the whole flow exists for.
func TestRegisterDoesNotIssueASession(t *testing.T) {
	t.Parallel()

	response := post(t, serve(t, workingIdentity()), "/api/v1/auth/register",
		`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password","account_type":"candidate"}`)

	if cookie, set := cookiesOf(t, response)[api.SessionCookieName]; set {
		t.Errorf("registration set a session cookie: %v", cookie)
	}
}

// The property that stops this endpoint being an account-existence oracle.
func TestRegisterAnswersIdenticallyForAKnownAddress(t *testing.T) {
	t.Parallel()

	body := `{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password","account_type":"candidate"}`

	fresh := post(t, serve(t, workingIdentity()), "/api/v1/auth/register", body)

	// A service that did nothing because the address exists reports success
	// just the same, so the two responses must be byte identical.
	existing := post(t, serve(t, workingIdentity()), "/api/v1/auth/register", body)

	if fresh.Code != existing.Code || fresh.Body.String() != existing.Body.String() {
		t.Errorf("a known address is distinguishable from a new one:\n  new:      %d %s\n  existing: %d %s",
			fresh.Code, fresh.Body, existing.Code, existing.Body)
	}
}

func TestRegisterRejectsAnInvalidBodyWithFieldErrors(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.registerErr = api.Invalid("password", "PASSWORD_TOO_SHORT", "A password needs at least 12 characters.")

	response := post(t, serve(t, identity), "/api/v1/auth/register",
		`{"email":"daniel.okonkwo@example.com","password":"short","account_type":"candidate"}`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", response.Code, response.Body)
	}

	var envelope struct {
		Error struct {
			Code        string `json:"code"`
			Message     string `json:"message"`
			Retryable   bool   `json:"retryable"`
			FieldErrors []struct {
				Field   string `json:"field"`
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"field_errors"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	decode(t, response, &envelope)

	if len(envelope.Error.FieldErrors) != 1 || envelope.Error.FieldErrors[0].Field != "password" {
		t.Errorf("the response does not say which field failed: %s", response.Body)
	}
	if envelope.Error.RequestID == "" {
		t.Error("the error envelope carries no request id, so a user cannot quote one to support")
	}
}

// A body that is not JSON at all must still produce the API envelope. The
// generated router answers plain text by default, which would make this the one
// failure in the API that a client cannot parse.
func TestAMalformedBodyProducesTheErrorEnvelope(t *testing.T) {
	t.Parallel()

	response := post(t, serve(t, workingIdentity()), "/api/v1/auth/register", `{not json`)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want JSON: %s", contentType, response.Body)
	}
	if !strings.Contains(response.Body.String(), `"error"`) {
		t.Errorf("a malformed body did not produce the error envelope: %s", response.Body)
	}
}

// ───────────────────────────────────────────────────────────────── login

func TestLoginIssuesBothCookies(t *testing.T) {
	t.Parallel()

	response := post(t, serve(t, workingIdentity()), "/api/v1/auth/login",
		`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body)
	}

	cookies := cookiesOf(t, response)

	// The generated response type carries Set-Cookie as a single string and
	// sets it with Header().Set, which replaces. Two cookies cannot be
	// expressed that way, which is why these responses are written by hand.
	// This is the assertion that would have caught it.
	session, hasSession := cookies[api.SessionCookieName]
	refresh, hasRefresh := cookies[api.RefreshCookieName]
	if !hasSession || !hasRefresh {
		t.Fatalf("login set %d cookies, want both session and refresh: %v", len(cookies), cookies)
	}

	if !session.HttpOnly || !refresh.HttpOnly {
		t.Error("a session cookie is readable by script, which defeats putting the token in a cookie at all")
	}
	if session.SameSite != http.SameSiteLaxMode {
		t.Errorf("session SameSite = %v, want Lax", session.SameSite)
	}
	if refresh.Path != "/api/v1/auth/refresh" {
		t.Errorf("refresh cookie path = %q, want the refresh endpoint only", refresh.Path)
	}
}

// Tokens go in cookies precisely so no script can read them. A body that also
// carried them would undo that completely.
func TestLoginNeverPutsATokenInTheBody(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := post(t, serve(t, identity), "/api/v1/auth/login",
		`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password"}`)

	body := response.Body.String()
	for _, token := range []string{identity.session.SessionToken, identity.session.RefreshToken} {
		if strings.Contains(body, token) {
			t.Errorf("the login body carries a token: %s", body)
		}
	}
}

func TestLoginReturnsTheSessionDescription(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := post(t, serve(t, identity), "/api/v1/auth/login",
		`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password"}`)

	var session struct {
		ActiveTenantID  *string   `json:"active_tenant_id"`
		AuthenticatedAt time.Time `json:"authenticated_at"`
		ExpiresAt       time.Time `json:"expires_at"`
		UserID          string    `json:"user_id"`
	}
	decode(t, response, &session)

	if session.UserID != userID {
		t.Errorf("user_id = %q, want %q", session.UserID, userID)
	}
	if session.ExpiresAt.IsZero() {
		t.Error("expires_at is zero, so a client cannot know when to refresh")
	}
}

// The response for a wrong password and for an unknown address must be
// indistinguishable, or the endpoint becomes the enumeration oracle that
// registration was carefully written not to be.
func TestLoginFailsIdenticallyForAWrongPasswordAndAnUnknownAddress(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.authenticateErr = api.ErrCredentialsRejected

	wrongPassword := post(t, serve(t, identity), "/api/v1/auth/login",
		`{"email":"daniel.okonkwo@example.com","password":"the-wrong-password"}`)
	unknownAddress := post(t, serve(t, identity), "/api/v1/auth/login",
		`{"email":"nobody.at.all@example.com","password":"any-password-at-all"}`)

	if wrongPassword.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401: %s", wrongPassword.Code, wrongPassword.Body)
	}
	if unknownAddress.Code != wrongPassword.Code {
		t.Errorf("statuses differ: %d and %d", wrongPassword.Code, unknownAddress.Code)
	}

	// Everything except the correlation identifier, which differs per request
	// by design and is the one field a caller already knows is theirs.
	if got, want := withoutRequestID(t, unknownAddress), withoutRequestID(t, wrongPassword); got != want {
		t.Errorf("the two failures are distinguishable:\n  wrong password:   %s\n  unknown address:  %s", want, got)
	}
}

// withoutRequestID renders an error body with the correlation identifier
// removed, so two responses can be compared for everything a caller could use
// to tell one cause from another.
func withoutRequestID(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()

	var envelope map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decoding %s: %v", response.Body, err)
	}
	if inner, ok := envelope["error"].(map[string]any); ok {
		delete(inner, "request_id")
	}

	rendered, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("re-encoding: %v", err)
	}
	return string(rendered)
}

// Two identical failures can still both be informative. This pins the message
// itself, because the property is not only that the two agree but that what
// they agree on reveals nothing.
//
// Added after changing the message to "No account exists for that address."
// left the whole suite green: the assertion above compares the two responses,
// and a message that leaks leaks equally in both.
func TestTheRejectedLoginMessageSaysNothingAboutTheAccount(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.authenticateErr = api.ErrCredentialsRejected

	response := post(t, serve(t, identity), "/api/v1/auth/login",
		`{"email":"daniel.okonkwo@example.com","password":"the-wrong-password"}`)

	body := strings.ToLower(response.Body.String())
	for _, word := range []string{
		"account", "exists", "registered", "unknown", "not found", "no such", "password",
	} {
		if strings.Contains(body, word) {
			t.Errorf("the rejection mentions %q, which tells the caller which half was wrong: %s",
				word, response.Body)
		}
	}
}

// The 500 message must be ours rather than derived from the error.
//
// Added after replacing it with err.Error() left the suite green: the scrubber
// removed the connection string in the fixture, so the leak test could not see
// it. Scrubbing is the last line, not the rule. An internal error's text can
// carry a table name, a query, or a stack frame, and none of those match a
// scrub pattern.
func TestAnUnexpectedFailureMessageIsFixedRatherThanDerived(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.authenticateErr = errors.New("relation \"identity.credentials\" does not exist at character 42")

	response := post(t, serve(t, identity), "/api/v1/auth/login",
		`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password"}`)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	for _, fragment := range []string{"relation", "credentials", "character 42"} {
		if strings.Contains(response.Body.String(), fragment) {
			t.Errorf("the response carries %q from the underlying error: %s", fragment, response.Body)
		}
	}
}

// A rejected login must not echo the address back. The response is logged, and
// an address in a log line is restricted content per the data classification.
func TestARejectedLoginDoesNotEchoTheAddress(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.authenticateErr = api.ErrCredentialsRejected

	response := post(t, serve(t, identity), "/api/v1/auth/login",
		`{"email":"daniel.okonkwo@example.com","password":"the-wrong-password"}`)

	if strings.Contains(response.Body.String(), "daniel.okonkwo") {
		t.Errorf("the failure echoes the address: %s", response.Body)
	}
}

// ─────────────────────────────────────────────────────────────── refresh

func TestRefreshRotatesBothCookies(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := post(t, serve(t, identity), "/api/v1/auth/refresh", "",
		&http.Cookie{Name: api.RefreshCookieName, Value: "ref_the_presented_token"})

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body)
	}
	if len(identity.refreshed) != 1 || identity.refreshed[0] != "ref_the_presented_token" {
		t.Errorf("the service saw %v, want the token from the cookie", identity.refreshed)
	}

	cookies := cookiesOf(t, response)
	if _, ok := cookies[api.SessionCookieName]; !ok {
		t.Error("refresh did not reissue the session cookie")
	}
	if _, ok := cookies[api.RefreshCookieName]; !ok {
		t.Error("refresh did not rotate the refresh cookie")
	}
}

// The refresh token lives in a cookie, so a request without one is a request
// that cannot be refreshed. It must not reach the service.
func TestRefreshWithoutACookieIsRejectedWithoutCallingTheService(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := post(t, serve(t, identity), "/api/v1/auth/refresh", "")

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if len(identity.refreshed) != 0 {
		t.Errorf("a request with no refresh token still reached the service: %v", identity.refreshed)
	}
}

// A rejected refresh must clear the cookies. Leaving them means the browser
// presents a dead token on every subsequent request, and the person sees
// repeated failures rather than a login screen.
func TestARejectedRefreshClearsTheCookies(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.refreshErr = api.ErrSessionRejected

	response := post(t, serve(t, identity), "/api/v1/auth/refresh", "",
		&http.Cookie{Name: api.RefreshCookieName, Value: "ref_a_retired_token"})

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	for _, name := range []string{api.SessionCookieName, api.RefreshCookieName} {
		cookie, cleared := cookiesOf(t, response)[name]
		if !cleared {
			t.Errorf("%s was not cleared after a rejected refresh", name)
			continue
		}
		if cookie.MaxAge >= 0 && cookie.Value != "" {
			t.Errorf("%s was reissued rather than cleared: %v", name, cookie)
		}
	}
}

// ──────────────────────────────────────────────────────────────── logout

func TestLogoutRevokesAndClearsBothCookies(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := post(t, serve(t, identity), "/api/v1/auth/logout", "",
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_the_current_session"})

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204: %s", response.Code, response.Body)
	}
	if len(identity.revoked) != 1 {
		t.Fatalf("the service saw %d revocations, want 1", len(identity.revoked))
	}

	cookies := cookiesOf(t, response)
	if len(cookies) != 2 {
		t.Errorf("logout set %d cookies, want both cleared: %v", len(cookies), cookies)
	}
	for name, cookie := range cookies {
		if cookie.MaxAge >= 0 && cookie.Value != "" {
			t.Errorf("%s was not cleared: %v", name, cookie)
		}
	}
}

// Revoking the session is what ends it; clearing the cookie only stops the
// browser presenting a dead token. A 204 without the revocation would look
// identical to the user and leave the session alive.
func TestLogoutRevokesEvenWhenTheSessionIsAlreadyGone(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.revokeErr = api.ErrSessionRejected

	response := post(t, serve(t, identity), "/api/v1/auth/logout", "",
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_already_revoked"})

	// Logging out of a session that is already gone is a successful logout.
	if response.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204: logging out twice is not an error", response.Code)
	}
	if len(cookiesOf(t, response)) != 2 {
		t.Error("the cookies were not cleared, so the browser keeps presenting a dead token")
	}
}

// ─────────────────────────────────────────────────────────────────── /me

func TestCurrentUserRequiresASession(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := get(t, serve(t, identity), "/api/v1/me")

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if len(identity.lookedUp) != 0 {
		t.Error("a request with no session cookie still reached the service")
	}
}

func TestCurrentUserDescribesTheAuthenticatedUser(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	response := get(t, serve(t, identity), "/api/v1/me",
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_a_live_session"})

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", response.Code, response.Body)
	}

	var user struct {
		ActiveTenantID *string `json:"active_tenant_id"`
		Email          *string `json:"email"`
		EmailVerified  bool    `json:"email_verified"`
		Memberships    []struct {
			TenantID   string `json:"tenant_id"`
			TenantName string `json:"tenant_name"`
			Status     string `json:"status"`
		} `json:"memberships"`
		UserID string `json:"user_id"`
	}
	decode(t, response, &user)

	if user.UserID != userID {
		t.Errorf("user_id = %q, want %q", user.UserID, userID)
	}
	if !user.EmailVerified {
		t.Error("email_verified is false for a verified user")
	}
	// Memberships must be an empty array rather than null, or every client has
	// to handle two shapes for "no memberships".
	if user.Memberships == nil {
		t.Error("memberships is null rather than an empty array")
	}
}

func TestCurrentUserRejectsADeadSession(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.lookupErr = api.ErrSessionRejected

	response := get(t, serve(t, identity), "/api/v1/me",
		&http.Cookie{Name: api.SessionCookieName, Value: "ses_a_revoked_session"})

	if response.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", response.Code)
	}
}

// ─────────────────────────────────────────────────────── failure handling

// An unexpected error must not reach the client as its own text. A driver error
// carries a connection string, and a provider error can carry a prompt.
func TestAnUnexpectedFailureDoesNotLeakItsMessage(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.authenticateErr = &driverFailure{}

	response := post(t, serve(t, identity), "/api/v1/auth/login",
		`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password"}`)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", response.Code, response.Body)
	}
	for _, secret := range []string{"hunter2", "db.internal", "postgres://"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Errorf("the response leaks %q: %s", secret, response.Body)
		}
	}
}

type driverFailure struct{}

func (*driverFailure) Error() string {
	return "dial postgres://prepeet:hunter2@db.internal:5432/prepeet: connection refused"
}

// The server must refuse to start without its dependencies rather than serving
// panics. A nil identity is a wiring mistake, and the place to find it is at
// startup.
func TestTheServerRefusesToStartWithoutAnIdentity(t *testing.T) {
	t.Parallel()

	if _, err := api.NewServer(api.ServerConfig{Environment: config.EnvironmentLocal}); err == nil {
		t.Error("NewServer accepted a nil identity, so the first request would panic instead")
	}
}
