package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
	"github.com/Yelethe1st/prepeet/services/platform/platform/authz"
)

/*
The capability each operation requires, checked against the contract rather
than against a handler's string literal.

CTR-01 requires every operation to declare the authority it needs, and the
capability catalogue IAM-04 published is what it declares it in. A declaration
nothing reads is a comment, so these tests bind it three ways: the value must
name a capability that exists, it must agree with the operation's own security
block, and the server must actually refuse a caller who does not hold it.

The alternative - each handler naming its own capability in Go - is what this
replaces. It puts the answer to "what authority reaches a candidate's practice
history" in forty-nine places, none of which a reviewer without Go can read.
*/

// capabilityExtension is where an operation declares its required capability.
//
// An OpenAPI extension rather than a description convention, because a
// description is prose and this has to be machine-checkable in both the lint
// and the test below.
const capabilityExtension = "x-prepeet-capability"

// The three declarations that are not capability names.
//
// Kept as reserved words rather than invented capabilities, because a
// catalogue entry called "public" would be authority nobody holds and every
// role would have to be written not to grant it.
const (
	// publicOperation needs no credential at all.
	publicOperation = "public"
	// authenticatedOperation needs a valid session and nothing further: the
	// answer is the same for everyone who holds one.
	authenticatedOperation = "authenticated"
	// serviceOperation is reached with the deployment's own service token and
	// never with a person's session (ADR-0019).
	serviceOperation = "service"
)

// declaredCapability reads the extension off an operation.
//
// The second result distinguishes "absent" from "empty", because those fail
// for different reasons and an operation declaring an empty string is a
// mistake worth naming rather than treating as public.
func declaredCapability(t *testing.T, operation *openapi3.Operation) (string, bool) {
	t.Helper()

	raw, present := operation.Extensions[capabilityExtension]
	if !present {
		return "", false
	}
	switch value := raw.(type) {
	case string:
		return value, true
	case json.RawMessage:
		var decoded string
		if err := json.Unmarshal(value, &decoded); err != nil {
			t.Fatalf("%s on %s is not a string: %v", capabilityExtension, operation.OperationID, err)
		}
		return decoded, true
	default:
		t.Fatalf("%s on %s is %T, want a string", capabilityExtension, operation.OperationID, raw)
		return "", false
	}
}

// contract is the document the server was generated from, so these tests read
// the same bytes the router does rather than a file that could have moved.
func contract(t *testing.T) *openapi3.T {
	t.Helper()

	spec, err := prepeetapi.GetSwagger()
	if err != nil {
		t.Fatalf("reading the embedded contract: %v", err)
	}
	return spec
}

// Every operation says what authority it needs, and says it in the vocabulary
// the catalogue defines.
//
// This is the half that covers operations no handler serves yet: an operation
// added to the contract for a later ticket still has to answer the question
// before anyone implements it, which is the point of writing the contract
// first.
func TestEveryOperationDeclaresTheCapabilityItRequires(t *testing.T) {
	t.Parallel()

	for path, item := range contract(t).Paths.Map() {
		for method, operation := range item.Operations() {
			declared, present := declaredCapability(t, operation)
			if !present {
				t.Errorf("%s %s declares no %s, so what authority it needs is left to whoever implements it",
					method, path, capabilityExtension)
				continue
			}

			switch declared {
			case publicOperation, authenticatedOperation, serviceOperation:
				continue
			}
			if _, known := authz.Describe(authz.Capability(declared)); !known {
				t.Errorf("%s %s requires %q, which is not in the capability catalogue",
					method, path, declared)
			}
		}
	}
}

// The declaration and the security block are two statements about the same
// thing, and a document where they disagree is worse than one that says
// nothing: a reader believes the one they happened to look at.
func TestTheDeclaredCapabilityAgreesWithTheOperationsSecurity(t *testing.T) {
	t.Parallel()

	spec := contract(t)
	for path, item := range spec.Paths.Map() {
		for method, operation := range item.Operations() {
			declared, present := declaredCapability(t, operation)
			if !present {
				continue // Reported by the test above; not repeated here.
			}

			schemes, optional := securityOf(spec, operation)
			switch declared {
			case publicOperation:
				if len(schemes) > 0 && !optional {
					t.Errorf("%s %s is declared public but requires %v", method, path, schemes)
				}
			case serviceOperation:
				if optional || len(schemes) != 1 || schemes[0] != "serviceToken" {
					t.Errorf("%s %s is declared a service operation but accepts %v", method, path, schemes)
				}
			default:
				if len(schemes) == 0 || optional {
					t.Errorf("%s %s requires %q but does not require a credential", method, path, declared)
				}
				for _, scheme := range schemes {
					if scheme == "serviceToken" {
						t.Errorf("%s %s requires %q, which is a person's authority, but accepts the service token",
							method, path, declared)
					}
				}
			}
		}
	}
}

// securityOf names the credentials an operation accepts and whether presenting
// one is optional.
//
// An operation with no security block of its own inherits the document's, and
// reading nil as "none" would report most of the API as public. An empty
// requirement among several is OpenAPI's way of saying the credential is
// accepted rather than demanded, which is a different claim from either
// requiring it or refusing to look at it.
func securityOf(spec *openapi3.T, operation *openapi3.Operation) (schemes []string, optional bool) {
	requirements := spec.Security
	if operation.Security != nil {
		requirements = *operation.Security
	}

	for _, requirement := range requirements {
		if len(requirement) == 0 {
			optional = true
			continue
		}
		for name := range requirement {
			schemes = append(schemes, name)
		}
	}
	return schemes, optional
}

// The wire half. Anything that requires authority must refuse a caller who
// presents none, or the declaration describes an intention rather than the
// server.
//
// Driven off the contract rather than a hand-written table, so an operation
// added later is covered the day it is added rather than the day somebody
// remembers to extend the list.
func TestEveryOperationRequiringAuthorityRefusesAnUnauthenticatedRequest(t *testing.T) {
	t.Parallel()

	handler := serve(t, workingIdentity())
	spec := contract(t)

	for path, item := range spec.Paths.Map() {
		for method, operation := range item.Operations() {
			declared, present := declaredCapability(t, operation)
			if !present || declared == publicOperation {
				continue
			}
			response := anonymous(t, handler, method, path, item, operation)
			if response.Code != http.StatusUnauthorized {
				t.Errorf("%s %s requires %q but answered %d without a credential, want 401",
					method, path, declared, response.Code)
			}
		}
	}
}

// And the capability enforced must be the one declared.
//
// Only the operations decided through the policy path can be checked this way,
// which is the set the Identity port's Authorize covers. Own-data capabilities
// are enforced structurally instead - the port takes only the session's own
// user - and the test above is what holds them.
func TestTheServerEnforcesTheCapabilityTheContractDeclares(t *testing.T) {
	t.Parallel()

	spec := contract(t)
	checked := 0

	for path, item := range spec.Paths.Map() {
		for method, operation := range item.Operations() {
			declared, present := declaredCapability(t, operation)
			if !present {
				continue
			}
			requirement, known := authz.Describe(authz.Capability(declared))
			if !known || requirement.Owner {
				continue
			}

			identity := adminIdentity()
			handler := serveMembers(t, identity, &fakeMembers{})
			asAdmin(t, handler, method, path, item, operation)

			if len(identity.asked) != 1 || identity.asked[0] != declared {
				t.Errorf("%s %s declares %q but asked for %v", method, path, declared, identity.asked)
			}
			checked++
		}
	}

	// A loop that silently matched nothing would pass forever. The count is
	// the assertion that the test still has a subject.
	if checked == 0 {
		t.Fatal("no operation is decided through the policy path, so this test proved nothing")
	}
}

// ─────────────────────────────────────────────────────────── request building

// anonymous makes the operation's request with no credential of any kind.
func anonymous(t *testing.T, handler http.Handler, method, path string, item *openapi3.PathItem, operation *openapi3.Operation) *httptest.ResponseRecorder {
	t.Helper()
	return roundTrip(t, handler, method, path, item, operation, nil)
}

// asAdmin makes it with a session cookie, so the request reaches the
// authorization decision rather than stopping at the missing credential.
func asAdmin(t *testing.T, handler http.Handler, method, path string, item *openapi3.PathItem, operation *openapi3.Operation) *httptest.ResponseRecorder {
	t.Helper()
	return roundTrip(t, handler, method, path, item, operation,
		&http.Cookie{Name: "prepeet_session", Value: "ses_a_live_session"})
}

func roundTrip(t *testing.T, handler http.Handler, method, path string, item *openapi3.PathItem, operation *openapi3.Operation, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()

	target := "/api/v1" + fill(path, item, operation)
	var body *strings.Reader
	if operation.RequestBody != nil {
		body = strings.NewReader("{}")
	} else {
		body = strings.NewReader("")
	}

	request := httptest.NewRequest(method, target, body)
	if operation.RequestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// placeholder is a syntactically valid identifier for any path or query
// parameter. Nothing here asserts on a resource, only on whether the request
// was refused before one was looked up.
const placeholder = "00000000-0000-7000-8000-000000000001"

// fill substitutes every path parameter and every required query parameter, so
// a request reaches the handler rather than stopping at the generated router's
// own validation and reporting 400 where the test is asking about 401.
func fill(path string, item *openapi3.PathItem, operation *openapi3.Operation) string {
	filled := path
	var query []string

	// Path-level parameters as well as the operation's own: a parameter
	// declared once for every method on a path is still one the request has to
	// carry, and reading only the operation's would leave it unsubstituted.
	for _, ref := range append(append(openapi3.Parameters{}, item.Parameters...), operation.Parameters...) {
		parameter := ref.Value
		if parameter == nil {
			continue
		}
		value := valueFor(parameter)
		switch parameter.In {
		case "path":
			filled = strings.ReplaceAll(filled, "{"+parameter.Name+"}", value)
		case "query":
			if parameter.Required {
				query = append(query, parameter.Name+"="+value)
			}
		}
	}

	if len(query) > 0 {
		return filled + "?" + strings.Join(query, "&")
	}
	return filled
}

// valueFor produces something the generated router will bind for a parameter.
//
// Typed rather than one placeholder for everything, because the router refuses
// a badly typed parameter with 400 before any handler runs, and a test asking
// whether a request was refused for want of a credential would then be reading
// the wrong refusal and calling it a pass.
func valueFor(parameter *openapi3.Parameter) string {
	schema := parameter.Schema
	if schema == nil || schema.Value == nil {
		return placeholder
	}
	if len(schema.Value.Enum) > 0 {
		if first, ok := schema.Value.Enum[0].(string); ok {
			return first
		}
	}
	if schema.Value.Type != nil {
		switch {
		case schema.Value.Type.Is("integer"), schema.Value.Type.Is("number"):
			return "1"
		case schema.Value.Type.Is("boolean"):
			return "true"
		}
	}
	return placeholder
}
