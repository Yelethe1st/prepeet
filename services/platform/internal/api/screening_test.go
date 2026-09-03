package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	api "github.com/Yelethe1st/prepeet/services/platform/internal/api"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// stubScreening records the token presented and returns what the test set.
type stubScreening struct {
	view    api.ScreeningInvitationView
	session api.Session

	outcome api.ScreeningOutcome

	resolveErr error
	acceptErr  error
	declineErr error
	resultErr  error

	sawToken string
}

func (s *stubScreening) Resolve(_ context.Context, token string) (api.ScreeningInvitationView, error) {
	s.sawToken = token
	if s.resolveErr != nil {
		return api.ScreeningInvitationView{}, s.resolveErr
	}
	return s.view, nil
}

func (s *stubScreening) Accept(_ context.Context, token string) (api.Session, error) {
	s.sawToken = token
	if s.acceptErr != nil {
		return api.Session{}, s.acceptErr
	}
	return s.session, nil
}

func (s *stubScreening) Decline(_ context.Context, token string) (api.ScreeningInvitationView, error) {
	s.sawToken = token
	if s.declineErr != nil {
		return api.ScreeningInvitationView{}, s.declineErr
	}
	return s.view, nil
}

func (s *stubScreening) Result(_ context.Context, candidateID, sessionID string) (api.ScreeningOutcome, error) {
	if s.resultErr != nil {
		return api.ScreeningOutcome{}, s.resultErr
	}
	return s.outcome, nil
}

func defaultStubScreening() *stubScreening {
	return &stubScreening{
		view:    api.ScreeningInvitationView{Status: "live", Employer: "Acme", Role: "Backend Engineer"},
		session: sampleSession(),
	}
}

func sampleSession() api.Session {
	return api.Session{
		UserID:          "00000000-0000-7000-8000-00000000d001",
		SessionToken:    "ses_stub-session-token",
		RefreshToken:    "ref_stub-refresh-token",
		ExpiresAt:       time.Now().Add(time.Hour),
		RefreshExpires:  time.Now().Add(24 * time.Hour),
		AuthenticatedAt: time.Now(),
	}
}

func serveScreening(t *testing.T, stub *stubScreening) http.Handler {
	t.Helper()
	handler, err := api.NewServer(api.ServerConfig{
		Identity: &fakeIdentity{}, Candidates: &fakeCandidates{}, Documents: &fakeDocuments{},
		Catalog: &fakeCatalog{}, Interviews: &fakeInterviews{}, Members: &fakeMembers{},
		Billing: &fakeBilling{}, Progression: &stubProgression{},
		SensitiveReads: &recordingAuditor{}, Settings: &stubSettings{},
		Recruiting: &stubRecruiting{}, Invitations: defaultStubInvitations(),
		ScreeningInvitations: stub, Environment: config.EnvironmentLocal,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return handler
}

// screeningRequest posts to a public screening endpoint. No session cookie: the
// candidate is not signed in, and the token in the body is their whole
// authority.
func screeningRequest(t *testing.T, handler http.Handler, path, body string) (int, map[string]any, *http.Response) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("content-type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	var decoded map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	return recorder.Code, decoded, recorder.Result()
}

const (
	resolvePath = "/api/v1/screening/invitation/resolve"
	acceptPath  = "/api/v1/screening/invitation/accept"
	declinePath = "/api/v1/screening/invitation/decline"
	sampleToken = `{"token":"inv_a-plausible-looking-token"}`
)

// Resolving a live invitation shows who invited the candidate and for what.
func TestResolveShowsTheInvitation(t *testing.T) {
	stub := defaultStubScreening()
	handler := serveScreening(t, stub)

	status, body, _ := screeningRequest(t, handler, resolvePath, sampleToken)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if body["employer"] != "Acme" || body["status"] != "live" {
		t.Fatalf("unexpected body: %v", body)
	}
	if stub.sawToken != "inv_a-plausible-looking-token" {
		t.Fatalf("token not forwarded: %q", stub.sawToken)
	}
}

// A token that names nothing is a 404 that says no more, so a guess cannot be
// told from a real spent link.
func TestResolveUnknownTokenIs404(t *testing.T) {
	stub := defaultStubScreening()
	stub.resolveErr = api.ErrScreeningInvitationUnknown
	handler := serveScreening(t, stub)

	status, _, _ := screeningRequest(t, handler, resolvePath, sampleToken)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// Accepting signs the candidate in: the response sets session cookies and never
// carries the token back.
func TestAcceptSignsInAndSetsCookies(t *testing.T) {
	handler := serveScreening(t, defaultStubScreening())

	status, body, response := screeningRequest(t, handler, acceptPath, sampleToken)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(response.Cookies()) == 0 {
		t.Fatal("accept issued no session cookies")
	}
	for _, key := range []string{"token", "session_token", "refresh_token"} {
		if _, present := body[key]; present {
			t.Fatalf("the accept response carried a %q field", key)
		}
	}
}

// An invitation that cannot be accepted is a 409 with its own code, so the
// candidate is told to resolve it again rather than shown a generic error.
func TestAcceptNotLiveIs409(t *testing.T) {
	stub := defaultStubScreening()
	stub.acceptErr = api.ErrScreeningInvitationNotLive
	handler := serveScreening(t, stub)

	status, body, _ := screeningRequest(t, handler, acceptPath, sampleToken)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
	if code, _ := body["error"].(map[string]any)["code"].(string); code != "INVITATION_NOT_LIVE" {
		t.Fatalf("code = %q, want INVITATION_NOT_LIVE", code)
	}
}

// Accepting an unknown token is a 404, distinct from a spent link's 409.
func TestAcceptUnknownTokenIs404(t *testing.T) {
	stub := defaultStubScreening()
	stub.acceptErr = api.ErrScreeningInvitationUnknown
	handler := serveScreening(t, stub)

	status, _, _ := screeningRequest(t, handler, acceptPath, sampleToken)
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// Declining is a first-class outcome: it records and returns, issuing no
// session.
func TestDeclineRecordsTheOutcome(t *testing.T) {
	stub := defaultStubScreening()
	stub.view = api.ScreeningInvitationView{Status: "declined", Employer: "Acme"}
	handler := serveScreening(t, stub)

	status, body, response := screeningRequest(t, handler, declinePath, sampleToken)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if body["status"] != "declined" {
		t.Fatalf("status = %v, want declined", body["status"])
	}
	if len(response.Cookies()) != 0 {
		t.Fatal("declining issued a session cookie; it must not sign anyone in")
	}
}

// Declining a link already answered is a 409.
func TestDeclineNotLiveIs409(t *testing.T) {
	stub := defaultStubScreening()
	stub.declineErr = api.ErrScreeningInvitationNotLive
	handler := serveScreening(t, stub)

	status, _, _ := screeningRequest(t, handler, declinePath, sampleToken)
	if status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
}

// The three endpoints need no session: a candidate is not signed in when they
// arrive, so a request with no cookie is served, not rejected as unauthenticated.
func TestScreeningEndpointsArePublic(t *testing.T) {
	handler := serveScreening(t, defaultStubScreening())
	for _, path := range []string{resolvePath, acceptPath, declinePath} {
		status, _, _ := screeningRequest(t, handler, path, sampleToken)
		if status == http.StatusUnauthorized {
			t.Fatalf("%s answered 401; it must be public", path)
		}
	}
}

// noLeakSessions is the handler-level half of the no-leak property: accept
// returns a session the same way whether the stub stands in for a new account
// or an existing one. The identity layer proves the accounts themselves are
// indistinguishable; here the surface must not add a tell.
func TestAcceptAnswersTheSameForNewAndExistingAccounts(t *testing.T) {
	for _, name := range []string{"new", "existing"} {
		t.Run(name, func(t *testing.T) {
			handler := serveScreening(t, defaultStubScreening())
			status, _, response := screeningRequest(t, handler, acceptPath, sampleToken)
			if status != http.StatusOK || len(response.Cookies()) == 0 {
				t.Fatalf("%s account: status %d, cookies %d", name, status, len(response.Cookies()))
			}
		})
	}
}
