package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
	"github.com/Yelethe1st/prepeet/services/platform/internal/api"
)

// The caching conventions in ADR-0004, checked against the contract rather than
// asserted from memory.
//
// A cacheability that exists only in a handler is a property nobody can review
// and no client can rely on, and one that exists only in the document is a
// comment. These read the embedded contract and compare it to what the server
// actually sends, so neither can move without the other.

// Every response must say what an intermediary may do with it. A response that
// says nothing is not neutral: an intermediary applies its own heuristics, and
// the ones for an authenticated JSON endpoint are not something to leave to a
// CDN's defaults.
func TestEveryResponseInTheContractDeclaresItsCacheability(t *testing.T) {
	t.Parallel()

	spec, err := prepeetapi.GetSwagger()
	if err != nil {
		t.Fatalf("reading the embedded contract: %v", err)
	}

	for path, item := range spec.Paths.Map() {
		for method, operation := range item.Operations() {
			for status, ref := range operation.Responses.Map() {
				response := ref.Value
				if response == nil {
					t.Fatalf("%s %s %s has no resolved response", method, path, status)
				}
				if _, declared := response.Headers["Cache-Control"]; !declared {
					t.Errorf("%s %s %s does not declare Cache-Control, so what an intermediary "+
						"may do with it is left to that intermediary", method, path, status)
				}
			}
		}
	}
}

// And what is declared must be what is sent. This is the half that makes the
// declaration more than a comment.
func TestWhatTheServerSendsMatchesWhatTheContractDeclares(t *testing.T) {
	t.Parallel()

	handler := serve(t, workingIdentity())

	// One request per operation this phase serves, chosen to reach the status
	// named. Each is a real round trip through the router, so the assertion is
	// about the wire rather than about a handler's return value.
	cases := []struct {
		name     string
		path     string
		status   int
		response func() *http.Response
	}{
		{
			name: "liveness", path: "/healthz", status: 200,
			response: func() *http.Response {
				return &http.Response{Header: get(t, handler, "/api/v1/healthz").Header()}
			},
		},
		{
			name: "readiness", path: "/readyz", status: 200,
			response: func() *http.Response {
				return &http.Response{Header: get(t, handler, "/api/v1/readyz").Header()}
			},
		},
		{
			name: "register", path: "/auth/register", status: 202,
			response: func() *http.Response {
				return &http.Response{Header: post(t, handler, "/api/v1/auth/register",
					`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password","account_type":"candidate"}`).Header()}
			},
		},
		{
			name: "login", path: "/auth/login", status: 200,
			response: func() *http.Response {
				return &http.Response{Header: post(t, handler, "/api/v1/auth/login",
					`{"email":"daniel.okonkwo@example.com","password":"a-long-enough-password"}`).Header()}
			},
		},
		{
			name: "refresh", path: "/auth/refresh", status: 200,
			response: func() *http.Response {
				return &http.Response{Header: post(t, handler, "/api/v1/auth/refresh", "",
					&http.Cookie{Name: "prepeet_refresh", Value: "ref_presented"}).Header()}
			},
		},
		{
			name: "logout", path: "/auth/logout", status: 204,
			response: func() *http.Response {
				return &http.Response{Header: post(t, handler, "/api/v1/auth/logout", "",
					&http.Cookie{Name: "prepeet_session", Value: "ses_current"}).Header()}
			},
		},
		{
			name: "current user", path: "/me", status: 200,
			response: func() *http.Response {
				return &http.Response{Header: get(t, handler, "/api/v1/me",
					&http.Cookie{Name: "prepeet_session", Value: "ses_current"}).Header()}
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			sent := testCase.response().Header.Get("Cache-Control")
			if sent == "" {
				t.Fatalf("%s sent no Cache-Control, while the contract declares one", testCase.path)
			}
			// The contract declares that a value is present and describes what
			// it means; the convention decides which value. Everything this
			// phase serves is derived from one person's data or is a probe, and
			// both are no-store.
			if sent != "no-store" {
				t.Errorf("%s sent Cache-Control %q, want no-store per ADR-0004", testCase.path, sent)
			}
		})
	}
}

// A failure must not be cached either. A cached 401 outlives the session that
// caused it, and the person then cannot sign in without clearing their cache.
func TestFailuresAreNotCacheable(t *testing.T) {
	t.Parallel()

	identity := workingIdentity()
	identity.authenticateErr = api.ErrCredentialsRejected

	handler := serve(t, identity)

	for name, response := range map[string]*httptest.ResponseRecorder{
		"rejected login": post(t, handler, "/api/v1/auth/login",
			`{"email":"daniel.okonkwo@example.com","password":"wrong"}`),
		"unauthenticated /me": get(t, handler, "/api/v1/me"),
		"unknown path":        get(t, handler, "/api/v1/no/such/thing"),
		"malformed body":      post(t, handler, "/api/v1/auth/register", `{not json`),
	} {
		if control := response.Header().Get("Cache-Control"); !strings.Contains(control, "no-store") {
			t.Errorf("%s sent Cache-Control %q, want no-store: a cached failure outlives its cause",
				name, control)
		}
	}
}
