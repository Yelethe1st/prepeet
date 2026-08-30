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

// fakeCandidates serves the profile port for tests, recording and scripted
// like the identity fake.
type fakeCandidates struct {
	profile api.Profile
	saved   []api.Profile
	err     error
}

func (f *fakeCandidates) GetProfile(_ context.Context, _ string) (api.Profile, error) {
	return f.profile, f.err
}

func (f *fakeCandidates) SaveProfile(_ context.Context, _ string, p api.Profile) (api.Profile, error) {
	if f.err != nil {
		return api.Profile{}, f.err
	}
	f.saved = append(f.saved, p)
	f.profile = p
	return p, nil
}

// fakeDocuments serves the document port; the flows' truth lives in the
// candidate integration suite, so this only needs to answer.
type fakeDocuments struct {
	started api.StartedUpload
	stored  api.Document
	listed  []api.Document
	facts   []api.Fact
	fact    api.Fact
	err     error

	reviewedID     string
	reviewedStatus string
	reviewedValue  json.RawMessage
}

func (f *fakeDocuments) StartUpload(_ context.Context, _, _ string, _ int64, _ int) (api.StartedUpload, error) {
	return f.started, f.err
}

func (f *fakeDocuments) CompleteUpload(_ context.Context, _, _, _, _ string, _ []api.UploadPart, _ int64) (api.Document, error) {
	return f.stored, f.err
}

func (f *fakeDocuments) AbortUpload(_ context.Context, _, _ string) error { return f.err }

func (f *fakeDocuments) DeleteDocument(_ context.Context, _, _ string) error { return f.err }

func (f *fakeDocuments) ListDocuments(_ context.Context, _ string) ([]api.Document, error) {
	return f.listed, f.err
}

func (f *fakeDocuments) ListFacts(_ context.Context, _, _ string) ([]api.Fact, error) {
	return f.facts, f.err
}

func (f *fakeDocuments) ReviewFact(_ context.Context, _, factID, status string, corrected json.RawMessage) (api.Fact, error) {
	f.reviewedID, f.reviewedStatus, f.reviewedValue = factID, status, corrected
	return f.fact, f.err
}

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

	oauthProviders []string
	oauthStart     api.OAuthStart
	oauthStartErr  error
	oauthBegun     []string
	oauthSession   api.Session
	oauthRedirect  string
	oauthErr       error
	oauthCompleted []string

	tokenRequests   []string
	tokenRequestErr error
	confirmedTokens []string
	confirmErr      error

	selected  []string
	selectErr error
}

// Authorize answers like Lookup by default: the capability-refusal paths are
// the members suite's subject, through authorizingIdentity.
func (f *fakeIdentity) Authorize(_ context.Context, presented, _ string) (api.Principal, error) {
	f.lookedUp = append(f.lookedUp, presented)
	return f.principal, f.lookupErr
}

// The token flows, recorded and scripted like everything else on the fake.
func (f *fakeIdentity) RequestTokenEmail(_ context.Context, kind, email string) error {
	f.tokenRequests = append(f.tokenRequests, kind+":"+email)
	return f.tokenRequestErr
}

func (f *fakeIdentity) ConfirmEmailVerification(_ context.Context, token string) error {
	f.confirmedTokens = append(f.confirmedTokens, token)
	return f.confirmErr
}

func (f *fakeIdentity) ConfirmPasswordReset(_ context.Context, token, _ string) error {
	f.confirmedTokens = append(f.confirmedTokens, token)
	return f.confirmErr
}

func (f *fakeIdentity) ConsumeMagicLink(_ context.Context, token string) (api.Session, error) {
	f.confirmedTokens = append(f.confirmedTokens, token)
	return f.session, f.confirmErr
}

func (f *fakeIdentity) ConsumeOTP(_ context.Context, email, code string) (api.Session, error) {
	f.confirmedTokens = append(f.confirmedTokens, email+":"+code)
	return f.session, f.confirmErr
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

func (f *fakeIdentity) SelectTenant(_ context.Context, _, tenantID string) (api.Principal, error) {
	f.selected = append(f.selected, tenantID)
	if f.selectErr != nil {
		return api.Principal{}, f.selectErr
	}
	principal := f.principal
	principal.ActiveTenantID = tenantID
	return principal, nil
}

func (f *fakeIdentity) Describe(_ context.Context, _ string) (api.User, error) {
	if f.describeErr != nil {
		return api.User{}, f.describeErr
	}
	return f.user, nil
}

func (f *fakeIdentity) DescribeSession(_ context.Context, _, _ string) (api.User, error) {
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

func serveWith(t *testing.T, identity api.Identity, candidates api.CandidateProfiles) http.Handler {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		Identity:    identity,
		Candidates:  candidates,
		Documents:   &fakeDocuments{},
		Catalog:     &fakeCatalog{},
		Interviews:  &fakeInterviews{},
		Members:     &fakeMembers{},
		Billing:     &fakeBilling{},
		Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

func serveWithLimiter(t *testing.T, identity api.Identity, limiter api.Limiter) http.Handler {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		Identity:           identity,
		Candidates:         &fakeCandidates{},
		Documents:          &fakeDocuments{},
		Catalog:            &fakeCatalog{},
		Interviews:         &fakeInterviews{},
		Members:            &fakeMembers{},
		Billing:            &fakeBilling{},
		AttemptsPerAddress: limiter,
		AttemptsPerNetwork: limiter,
		Environment:        config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

func serveIn(t *testing.T, identity api.Identity, environment config.Environment) http.Handler {
	t.Helper()

	handler, err := api.NewServer(api.ServerConfig{
		Identity:    identity,
		Candidates:  &fakeCandidates{},
		Documents:   &fakeDocuments{},
		Catalog:     &fakeCatalog{},
		Interviews:  &fakeInterviews{},
		Members:     &fakeMembers{},
		Billing:     &fakeBilling{},
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

func put(t *testing.T, handler http.Handler, path, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
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

// decodeInto decodes without rejecting unknown fields, for a test that cares
// about one part of a response rather than its whole shape.
func decodeInto(t *testing.T, response *httptest.ResponseRecorder, into any) {
	t.Helper()

	if err := json.Unmarshal(response.Body.Bytes(), into); err != nil {
		t.Fatalf("decoding %s: %v", response.Body, err)
	}
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
		ActiveTenantID *string  `json:"active_tenant_id"`
		Email          *string  `json:"email"`
		EmailVerified  bool     `json:"email_verified"`
		Capabilities   []string `json:"capabilities"`
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

func (f *fakeIdentity) ConfiguredOAuthProviders() []string { return f.oauthProviders }

func (f *fakeIdentity) BeginOAuth(_ context.Context, provider, redirectTo string) (api.OAuthStart, error) {
	f.oauthBegun = append(f.oauthBegun, provider+":"+redirectTo)
	if f.oauthStartErr != nil {
		return api.OAuthStart{}, f.oauthStartErr
	}
	return f.oauthStart, nil
}

func (f *fakeIdentity) CompleteOAuth(_ context.Context, provider, state, code string) (api.Session, string, error) {
	f.oauthCompleted = append(f.oauthCompleted, provider+":"+state+":"+code)
	if f.oauthErr != nil {
		return api.Session{}, "", f.oauthErr
	}
	return f.oauthSession, f.oauthRedirect, nil
}

// IAM-08 at the edge. The properties worth asserting here are the ones the
// domain cannot: that the callback issues the same cookies login does, and
// that each refusal reaches the person as its own sentence.

func TestTheSignInScreenIsToldWhichProvidersExist(t *testing.T) {
	identity := workingIdentity()
	identity.oauthProviders = []string{"google", "microsoft"}

	response := get(t, serve(t, identity), "/api/v1/auth/oauth/providers")

	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		Providers []struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Providers) != 2 || body.Providers[0].ID != "google" {
		t.Fatalf("providers came back as %+v", body.Providers)
	}
	// Labelled for people, not by key.
	if body.Providers[0].Label != "Google" {
		t.Fatalf("label = %q, want Google", body.Providers[0].Label)
	}
}

// A deployment with none configured must answer an empty list rather than
// null, so the screen renders email and password alone instead of crashing.
func TestNoProvidersIsAnEmptyListNotNull(t *testing.T) {
	response := get(t, serve(t, workingIdentity()), "/api/v1/auth/oauth/providers")

	if !strings.Contains(response.Body.String(), `"providers":[]`) {
		t.Fatalf("want an empty list, got %s", response.Body)
	}
}

func TestStartingASignInAnswersWhereToSendTheBrowser(t *testing.T) {
	identity := workingIdentity()
	identity.oauthStart = api.OAuthStart{
		AuthorizationURL: "https://accounts.google.example/authorize?state=abc",
		State:            "abc",
	}

	response := post(t, serve(t, identity), "/api/v1/auth/oauth/google/start",
		`{"redirect_to":"/practice"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), "accounts.google.example") {
		t.Fatalf("no authorization url: %s", response.Body)
	}
	if len(identity.oauthBegun) != 1 || identity.oauthBegun[0] != "google:/practice" {
		t.Fatalf("the provider and destination did not arrive: %v", identity.oauthBegun)
	}
}

func TestAnUnknownProviderIsNotFound(t *testing.T) {
	identity := workingIdentity()
	identity.oauthStartErr = api.ErrOAuthProviderUnknown

	response := post(t, serve(t, identity), "/api/v1/auth/oauth/myspace/start", `{}`)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), "OAUTH_PROVIDER_UNKNOWN") {
		t.Fatalf("want the refusal named, got %s", response.Body)
	}
}

// The third criterion, at the edge: the callback sets the same two cookies
// login sets, so an OAuth session is indistinguishable downstream.
func TestTheCallbackIssuesTheSameCookiesLoginDoes(t *testing.T) {
	identity := workingIdentity()
	identity.oauthSession = identity.session

	response := post(t, serve(t, identity), "/api/v1/auth/oauth/google/callback",
		`{"state":"abc","code":"auth-code"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("want two cookies as login sets, got %d", len(cookies))
	}
	for _, cookie := range cookies {
		if !cookie.HttpOnly {
			t.Fatalf("%s is readable by script", cookie.Name)
		}
	}
	if len(identity.oauthCompleted) != 1 || identity.oauthCompleted[0] != "google:abc:auth-code" {
		t.Fatalf("the callback did not pass through intact: %v", identity.oauthCompleted)
	}
}

func TestEachOAuthRefusalGetsItsOwnAnswer(t *testing.T) {
	for _, refusal := range []struct {
		err    error
		status int
		code   string
	}{
		{api.ErrOAuthStateRejected, http.StatusConflict, "OAUTH_STATE_INVALID"},
		{api.ErrOAuthStateExpired, http.StatusConflict, "OAUTH_STATE_EXPIRED"},
		{api.ErrOAuthAddressUnverified, http.StatusConflict, "OAUTH_EMAIL_UNVERIFIED"},
	} {
		t.Run(refusal.code, func(t *testing.T) {
			identity := workingIdentity()
			identity.oauthErr = refusal.err

			response := post(t, serve(t, identity), "/api/v1/auth/oauth/google/callback",
				`{"state":"abc","code":"auth-code"}`)

			if response.Code != refusal.status {
				t.Fatalf("status %d: %s", response.Code, response.Body)
			}
			if !strings.Contains(response.Body.String(), refusal.code) {
				t.Fatalf("want %s, got %s", refusal.code, response.Body)
			}
			// No cookies on a refusal: a failed sign-in must not leave
			// anything behind that looks like a session.
			if len(response.Result().Cookies()) != 0 {
				t.Fatal("a refused sign-in set a cookie")
			}
		})
	}
}

// The unverified-address refusal must not confirm that an account exists.
func TestTheUnverifiedRefusalDoesNotConfirmAnAccount(t *testing.T) {
	identity := workingIdentity()
	identity.oauthErr = api.ErrOAuthAddressUnverified

	response := post(t, serve(t, identity), "/api/v1/auth/oauth/google/callback",
		`{"state":"abc","code":"auth-code"}`)

	body := strings.ToLower(response.Body.String())
	for _, leak := range []string{"already", "exists", "registered", "taken"} {
		if strings.Contains(body, leak) {
			t.Fatalf("the message says %q, which confirms an account: %s", leak, response.Body)
		}
	}
}

// The counter is per network, not per provider. Keyed on the provider, one
// attacker starting sign-ins would exhaust the allowance for everybody using
// that provider, which is a lockout dressed as a rate limit.
func TestStartingASignInIsNotCountedPerProvider(t *testing.T) {
	identity := workingIdentity()
	limiter := newCountingLimiter(50)
	handler := serveWithLimiter(t, identity, limiter)

	post(t, handler, "/api/v1/auth/oauth/google/start", `{}`)

	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	for key := range limiter.seen {
		if strings.Contains(key, "google") {
			t.Fatalf("the allowance is keyed on the provider: %q", key)
		}
	}
}

// Regression from the IAM-08 review: the handler took the destination the
// start had recorded and assigned it to _, so a sign-in begun from anywhere
// other than the default landed on the default anyway.
func TestTheCallbackReturnsWhereTheSignInWasStartedFrom(t *testing.T) {
	identity := workingIdentity()
	identity.oauthSession = identity.session
	identity.oauthRedirect = "/session/abc/results"

	response := post(t, serve(t, identity), "/api/v1/auth/oauth/google/callback",
		`{"state":"abc","code":"auth-code"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body)
	}
	var body struct {
		RedirectTo string `json:"redirect_to"`
		Session    struct {
			UserID string `json:"user_id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.RedirectTo != "/session/abc/results" {
		t.Fatalf("redirect_to = %q, want the destination the start recorded", body.RedirectTo)
	}
	// And the session is still there, in its own field: the callback answers
	// with both, not one instead of the other.
	if body.Session.UserID == "" {
		t.Fatalf("the session did not survive carrying the destination: %s", response.Body)
	}
}

// No destination is an empty string rather than an absent field, so a client
// can use its own default without distinguishing null from missing.
func TestNoDestinationIsAnEmptyString(t *testing.T) {
	identity := workingIdentity()
	identity.oauthSession = identity.session

	response := post(t, serve(t, identity), "/api/v1/auth/oauth/google/callback",
		`{"state":"abc","code":"auth-code"}`)

	if !strings.Contains(response.Body.String(), `"redirect_to":""`) {
		t.Fatalf("want an empty destination, got %s", response.Body)
	}
}

// The cookies still arrive, which is IAM-08's third criterion and the thing
// most easily lost when a response body changes shape.
func TestTheCallbackStillSetsBothCookiesWithTheNewBody(t *testing.T) {
	identity := workingIdentity()
	identity.oauthSession = identity.session
	identity.oauthRedirect = "/practice"

	response := post(t, serve(t, identity), "/api/v1/auth/oauth/google/callback",
		`{"state":"abc","code":"auth-code"}`)

	cookies := response.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("want two cookies as login sets, got %d", len(cookies))
	}
	for _, cookie := range cookies {
		if !cookie.HttpOnly {
			t.Fatalf("%s is readable by script", cookie.Name)
		}
	}
}
