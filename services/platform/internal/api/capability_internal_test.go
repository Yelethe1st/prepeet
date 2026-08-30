package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

/*
What the server does with a contract that does not say what it must.

Every branch here ends in a refusal to start. That is the point: a declaration
the server cannot read is not a warning to log and carry on from, because the
next thing it would do is decide an authorization without knowing what was
required. These exist because a refusal nobody has exercised is a refusal
nobody knows the shape of.

The real document is checked by the tests in capability_test.go, which read the
embedded contract. These build documents on purpose broken, which the embedded
one can never be.
*/

// document builds a one-operation contract with whatever extensions are given.
func document(extensions map[string]any) *openapi3.T {
	operation := &openapi3.Operation{OperationID: "readThing", Extensions: extensions}
	paths := openapi3.NewPaths()
	paths.Set("/thing", &openapi3.PathItem{Get: operation})
	return &openapi3.T{Paths: paths}
}

func TestAnOperationWithNoDeclarationRefusesToStart(t *testing.T) {
	t.Parallel()

	if _, err := capabilitiesIn(document(nil)); err == nil {
		t.Fatal("an operation declaring no capability was accepted, so the server " +
			"would serve it without knowing what it requires")
	}
}

func TestAnEmptyDeclarationRefusesToStart(t *testing.T) {
	t.Parallel()

	// Distinguished from absent on purpose. An empty string would otherwise be
	// carried into the context, where the handlers read it as "no capability
	// declared" and refuse every request, which is a fail-closed outage nobody
	// could explain from the document.
	_, err := capabilitiesIn(document(map[string]any{capabilityExtension: ""}))
	if err == nil {
		t.Fatal("an empty declaration was accepted")
	}
}

func TestADeclarationThatIsNotAStringRefusesToStart(t *testing.T) {
	t.Parallel()

	_, err := capabilitiesIn(document(map[string]any{capabilityExtension: 7}))
	if err == nil {
		t.Fatal("a numeric declaration was accepted")
	}
}

func TestADeclarationStillInRawJSONIsRead(t *testing.T) {
	t.Parallel()

	// kin-openapi hands extensions back decoded or as raw JSON depending on how
	// the document reached it. Both have to work, because which one the
	// embedded document produces is the loader's business and not ours.
	required, err := capabilitiesIn(document(map[string]any{
		capabilityExtension: json.RawMessage(`"tenant.member_read"`),
	}))
	if err != nil {
		t.Fatalf("a raw JSON declaration was refused: %v", err)
	}
	if required["readThing"] != "tenant.member_read" {
		t.Errorf("readThing requires %q, want tenant.member_read", required["readThing"])
	}
}

func TestRawJSONThatIsNotAStringRefusesToStart(t *testing.T) {
	t.Parallel()

	_, err := capabilitiesIn(document(map[string]any{
		capabilityExtension: json.RawMessage(`{"capability":"tenant.member_read"}`),
	}))
	if err == nil {
		t.Fatal("a raw JSON object declaration was accepted")
	}
}

func TestTheReservedWordsAreNotCarriedAsCapabilities(t *testing.T) {
	t.Parallel()

	// A handler asking for the capability of a public operation must get
	// nothing back rather than the word "public", which is not in the catalogue
	// and would be denied with a message about a capability nobody wrote.
	for _, reserved := range []string{publicOperation, authenticatedOperation, serviceOperation} {
		required, err := capabilitiesIn(document(map[string]any{capabilityExtension: reserved}))
		if err != nil {
			t.Fatalf("%s was refused: %v", reserved, err)
		}
		if capability, present := required["readThing"]; present {
			t.Errorf("%s was carried through as the capability %q", reserved, capability)
		}
	}
}

func TestACapabilityIsOnlyReadableWhereTheMiddlewarePutIt(t *testing.T) {
	t.Parallel()

	// The empty declaration is not stored, so a handler reached by any other
	// route than the generated router's finds nothing and refuses. Without this
	// the zero value would be indistinguishable from a real declaration.
	if capability := requiredCapability(withRequiredCapability(t.Context(), "")); capability != "" {
		t.Errorf("an empty declaration was stored as %q", capability)
	}

	ctx := withRequiredCapability(t.Context(), "tenant.billing_read")
	if capability := requiredCapability(ctx); capability != "tenant.billing_read" {
		t.Errorf("the declaration read back as %q", capability)
	}
}

// The embedded document itself must satisfy the reader, or nothing starts.
// This is the one test here that uses the real contract, and it is the reason
// NewServer can treat a failure as a wiring bug rather than a condition.
func TestTheEmbeddedContractDeclaresACapabilityForEveryOperation(t *testing.T) {
	t.Parallel()

	required, err := requiredCapabilities()
	if err != nil {
		t.Fatalf("the embedded contract was refused: %v", err)
	}
	if len(required) == 0 {
		t.Fatal("no operation requires a capability, which cannot be right for this API")
	}
}

// A handler wired to an operation the contract says needs no authority refuses
// rather than asking the policy path for permission to do nothing.
//
// Unreachable through the router, because the router only ever calls a handler
// for the operation it matched, and every operation these two serve declares a
// capability. It is tested anyway: the whole arrangement rests on the handler
// having no fallback, and a branch that cannot be reached today is exactly the
// one that quietly becomes reachable when somebody adds an operation.
func TestTheDecidedHandlersRefuseWhenTheOperationDeclaresNoCapability(t *testing.T) {
	t.Parallel()

	// A nil Identity is safe here precisely because neither handler reaches it:
	// if either did, this test would panic rather than pass, which is a stronger
	// statement than asserting on a fake that was never called.
	authentication := &authentication{identity: nil}

	billing := billingHandlers{authentication: authentication}
	if _, refused := billing.usage(t.Context()); refused == nil {
		t.Error("a billing read with no declared capability was allowed to proceed")
	} else if refused.status != http.StatusUnauthorized {
		t.Errorf("billing status = %d, want 401", refused.status)
	}

	administration := members{authentication: authentication}
	if _, refused := administration.authorized(t.Context()); refused == nil {
		t.Error("member administration with no declared capability was allowed to proceed")
	} else if refused.status != http.StatusUnauthorized {
		t.Errorf("members status = %d, want 401", refused.status)
	}
}
