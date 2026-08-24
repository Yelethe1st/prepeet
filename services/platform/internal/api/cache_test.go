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

/*
Tenant is never inferred from a resource identifier.

docs/architecture/authorization-model.md requires every request to operate under
exactly one explicit active tenant, chosen deliberately and stored on the
session. The way that rule is broken is not by somebody deciding to break it: it
is by an endpoint like /tenants/{tenantId}/sessions looking natural, and the
handler scoping itself to the path parameter because it is right there.

Checked against the contract rather than against the handlers, because the
contract is where such a path would be added first, and because a path that does
not exist cannot be read from.
*/
func TestNoPathParameterCarriesATenant(t *testing.T) {
	t.Parallel()

	spec, err := prepeetapi.GetSwagger()
	if err != nil {
		t.Fatalf("reading the embedded contract: %v", err)
	}

	for path, item := range spec.Paths.Map() {
		for method, operation := range item.Operations() {
			for _, parameter := range operation.Parameters {
				if parameter.Value == nil || parameter.Value.In != "path" {
					continue
				}
				if looksLikeTenant(parameter.Value.Name) {
					t.Errorf("%s %s takes tenant %q in its path\n"+
						"    Tenant comes from the session, chosen through PUT /me/active-tenant.\n"+
						"    A tenant in the path is a value the caller supplies, and a handler\n"+
						"    that scopes itself to one is scoping itself to whatever it was sent.",
						method, path, parameter.Value.Name)
				}
			}
		}

		// The template itself, since a path can name a parameter the operation
		// does not declare.
		for _, segment := range strings.Split(path, "/") {
			if !strings.HasPrefix(segment, "{") {
				continue
			}
			if looksLikeTenant(strings.Trim(segment, "{}")) {
				t.Errorf("the path %s contains a tenant identifier", path)
			}
		}
	}
}

// looksLikeTenant matches the names a tenant parameter would plausibly have.
//
// A list rather than an exact match, because the point is to catch the shape
// before it becomes a habit, and the second one somebody adds will not be
// spelled the same as the first.
func looksLikeTenant(name string) bool {
	lowered := strings.ToLower(strings.ReplaceAll(name, "_", ""))
	for _, shape := range []string{"tenantid", "tenant", "workspaceid", "organisationid", "organizationid"} {
		if lowered == shape {
			return true
		}
	}
	return false
}
