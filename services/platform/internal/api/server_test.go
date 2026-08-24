package api_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
)

// Where the API sits, and what it does with what it does not serve.
//
// These are about the router rather than any handler, and they are the
// assertions most likely to be broken by a change nobody thought was risky:
// mounting at a different prefix, or letting the generated router answer for
// itself.

func TestTheAPIIsServedUnderTheVersionedPath(t *testing.T) {
	t.Parallel()

	handler := serve(t, workingIdentity())

	// ADR-0004 puts the version in the path because a new version is a project
	// rather than a release. An unversioned route would be a second, permanent
	// API that nobody decided to publish.
	unversioned := post(t, handler, "/auth/login",
		`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password"}`)

	if unversioned.Code != http.StatusNotFound {
		t.Errorf("POST /auth/login answered %d, so the API is reachable without its version",
			unversioned.Code)
	}
}

// The probes stay unversioned as well as versioned. The orchestrator is
// configured with a path and does not read the contract, so moving the probes
// under /api/v1 would make every deploy fail its health check.
func TestTheProbesRemainReachableWithoutTheVersion(t *testing.T) {
	t.Parallel()

	handler := serve(t, workingIdentity())

	for _, path := range []string{"/healthz", "/readyz", "/api/v1/healthz", "/api/v1/readyz"} {
		if response := get(t, handler, path); response.Code != http.StatusOK {
			t.Errorf("GET %s answered %d, want 200", path, response.Code)
		}
	}
}

// An unknown path must produce the API envelope rather than Go's default text,
// or the one response a confused client is most likely to hit is the one it
// cannot parse.
func TestAnUnknownPathProducesTheErrorEnvelope(t *testing.T) {
	t.Parallel()

	response := get(t, serve(t, workingIdentity()), "/api/v1/no/such/thing")

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"code":"NOT_FOUND"`) {
		t.Errorf("an unknown path did not produce the envelope: %s", response.Body)
	}
}

func TestTheWrongMethodIsDistinguishedFromAnUnknownPath(t *testing.T) {
	t.Parallel()

	response := get(t, serve(t, workingIdentity()), "/api/v1/auth/login")

	if response.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET on a POST-only endpoint answered %d, want 405", response.Code)
	}
	if allow := response.Header().Get("Allow"); !strings.Contains(allow, http.MethodPost) {
		t.Errorf("Allow = %q, want it to name POST", allow)
	}
}

// Every response carries the correlation identifier, which is what makes a user
// quoting a value from an error message resolvable to one trace.
func TestEveryResponseCarriesACorrelationIdentifier(t *testing.T) {
	t.Parallel()

	handler := serve(t, workingIdentity())

	for _, response := range []*http.Response{
		{Header: post(t, handler, "/api/v1/auth/register",
			`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password","account_type":"candidate"}`).Header()},
		{Header: get(t, handler, "/api/v1/me").Header()},
		{Header: get(t, handler, "/api/v1/no/such/thing").Header()},
	} {
		if response.Header.Get("X-Request-ID") == "" {
			t.Error("a response carries no correlation identifier")
		}
	}
}

// A session description is about one person and must never be stored by an
// intermediary, per the caching conventions in ADR-0004.
func TestAuthenticatedResponsesAreNotCacheable(t *testing.T) {
	t.Parallel()

	handler := serve(t, workingIdentity())

	login := post(t, handler, "/api/v1/auth/login",
		`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password"}`)
	me := get(t, handler, "/api/v1/me",
		&http.Cookie{Name: "prepeet_session", Value: "ses_a_live_session"})

	for name, response := range map[string]string{
		"login": login.Header().Get("Cache-Control"),
		"/me":   me.Header().Get("Cache-Control"),
	} {
		if !strings.Contains(response, "no-store") {
			t.Errorf("%s Cache-Control = %q, want no-store", name, response)
		}
	}
}

// Cookies are marked Secure everywhere except local development, where there is
// no TLS and marking them would mean nobody can log in.
func TestCookiesAreSecureOutsideLocalDevelopment(t *testing.T) {
	t.Parallel()

	for environment, wantSecure := range map[config.Environment]bool{
		config.EnvironmentLocal:      false,
		config.EnvironmentPreview:    true,
		config.EnvironmentStaging:    true,
		config.EnvironmentProduction: true,
	} {
		handler := serveIn(t, workingIdentity(), environment)

		response := post(t, handler, "/api/v1/auth/login",
			`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password"}`)

		for name, cookie := range cookiesOf(t, response) {
			if cookie.Secure != wantSecure {
				t.Errorf("in %s, %s Secure = %v, want %v", environment, name, cookie.Secure, wantSecure)
			}
		}
	}
}
