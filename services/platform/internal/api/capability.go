package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
)

// The authority each operation requires, taken from the contract.
//
// ADR-0004 makes the OpenAPI document the source and generates everything else
// from it. A handler naming its own capability in a Go string literal is the
// one place that arrangement leaked: the document could say a caller needs
// tenant.member_manage while the code asked for something else, and nothing
// would notice until a person was let through a door the contract had closed.
//
// So the handlers read it from the same document the router was generated
// from. Changing the declaration changes what is enforced, which is what makes
// the declaration worth reviewing.

// capabilityExtension is where an operation declares its required capability.
const capabilityExtension = "x-prepeet-capability"

// The three declarations that name no capability. They are reserved words
// rather than catalogue entries, because a capability called "public" would be
// authority nobody holds and every role would have to be written not to grant
// it. See the header of packages/contracts/api/openapi.yaml.
const (
	publicOperation        = "public"
	authenticatedOperation = "authenticated"
	serviceOperation       = "service"
)

// capabilityKey carries the operation's declared capability into the handler.
type capabilityContextKey struct{}

// requiredCapabilities reads the contract the router was generated from and
// indexes the declared capability by operation.
//
// Read once at startup rather than per request: the embedded document is
// gzipped and parsing it on every call would put a JSON parse in front of every
// authorization decision.
//
// The reserved words are deliberately not in the returned map. A handler that
// asks for the capability of an operation declared public or authenticated gets
// nothing back, and refuses, rather than passing a word the catalogue has never
// heard of into an authorization decision.
func requiredCapabilities() (map[string]string, error) {
	spec, err := prepeetapi.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("api: reading the embedded contract: %w", err)
	}
	return capabilitiesIn(spec)
}

// capabilitiesIn is the part of the above that does not need the embedded
// document, split out so its refusals can be tested.
//
// Every branch here is a refusal to start, and a refusal nobody has exercised
// is a refusal nobody knows the shape of. Building a document that omits a
// declaration is the only way to see what happens when one does.
func capabilitiesIn(spec *openapi3.T) (map[string]string, error) {
	required := map[string]string{}
	for path, item := range spec.Paths.Map() {
		for method, operation := range item.Operations() {
			raw, declared := operation.Extensions[capabilityExtension]
			if !declared {
				return nil, fmt.Errorf("api: %s %s declares no %s", method, path, capabilityExtension)
			}

			capability, err := asString(raw)
			if err != nil {
				return nil, fmt.Errorf("api: %s %s declares an unreadable %s: %w",
					method, path, capabilityExtension, err)
			}
			switch capability {
			case publicOperation, authenticatedOperation, serviceOperation:
				continue
			case "":
				return nil, fmt.Errorf("api: %s %s declares an empty %s", method, path, capabilityExtension)
			}
			required[operation.OperationID] = capability
		}
	}
	return required, nil
}

// asString reads an extension value, which kin-openapi hands back either
// decoded or still as raw JSON depending on how the document was loaded.
func asString(raw any) (string, error) {
	switch value := raw.(type) {
	case string:
		return value, nil
	case json.RawMessage:
		var decoded string
		if err := json.Unmarshal(value, &decoded); err != nil {
			return "", err
		}
		return decoded, nil
	default:
		return "", fmt.Errorf("want a string, got %T", raw)
	}
}

// withRequiredCapability records what this operation needs.
//
// Set by the strict middleware, which is the only layer that knows both the
// operation the router matched and the context the handler will receive.
func withRequiredCapability(ctx context.Context, capability string) context.Context {
	if capability == "" {
		return ctx
	}
	return context.WithValue(ctx, capabilityContextKey{}, capability)
}

// requiredCapability returns the capability the contract declares for the
// operation being served, or empty if it declares none.
//
// Empty is not "allow": a handler that reaches an authorization decision and
// finds no capability has been wired to an operation the contract says needs no
// authority, and refuses rather than asking for permission to do nothing.
func requiredCapability(ctx context.Context) string {
	capability, _ := ctx.Value(capabilityContextKey{}).(string)
	return capability
}
