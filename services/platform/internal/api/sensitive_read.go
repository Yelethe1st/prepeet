package api

import (
	"context"
	"fmt"
	"net/http"
	"reflect"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"
	"github.com/Yelethe1st/prepeet/services/platform/platform/config"
	"github.com/Yelethe1st/prepeet/services/platform/platform/httpserver"
)

// Which reads are events rather than queries, taken from the contract.
//
// authorization-model.md is unambiguous about one case: "Reading
// transcript/audio is independently authorized and audited." Its other mentions
// of the subject say "auditable", "where required" and "may be", so this covers
// what is actually required and claims nothing wider. Widening it is a line in
// the document, which a reviewer sees, rather than a call somebody adds.
//
// Declared in the contract for the reason CTR-01 put capabilities there: a
// handler deciding for itself whether it was sensitive would be a second source
// of truth, and the copy that drifts is always the one nobody re-reads.

// sensitiveReadExtension is where an operation declares that reading it is an
// auditable event.
const sensitiveReadExtension = "x-prepeet-sensitive-read"

// SensitiveRead is one recorded access to restricted content.
//
// Identifiers and outcome only. The row says a person read a transcript, never
// what the transcript said: audit.events is read by support and exported to
// tenants, and neither is a place for restricted content. Same rule as
// telemetry and workflow payloads.
type SensitiveRead struct {
	// ActorID is who read. Never empty: a request with no resolvable actor is
	// not recorded here at all, because it never reached the content.
	ActorID string
	// TenantID is the workspace the read happened in, empty for a candidate
	// reading their own data outside any tenant.
	TenantID string
	// Action names the operation, taken from the contract's operationId so the
	// row and the document share one vocabulary.
	Action string
	// SubjectType and SubjectID say what was read.
	SubjectType string
	SubjectID   string
	// Outcome is allowed, denied or failed, matching audit.events' constraint.
	// A refused attempt is the more interesting of the three, so it is recorded
	// rather than dropped.
	Outcome string
	// RequestID ties the row to the trace of the same moment, so a question
	// that starts in the audit log can be finished in the trace.
	RequestID string
}

// SensitiveReadAuditor records access to restricted content.
//
// A consumer-defined port: internal/api knows nothing about the audit schema,
// and the adapter that writes the row lives in cmd.
type SensitiveReadAuditor interface {
	RecordSensitiveRead(ctx context.Context, read SensitiveRead) error
}

// sensitiveOperations reads the embedded contract and answers which operations
// are declared auditable, indexed by operation id.
func sensitiveOperations() (map[string]bool, error) {
	spec, err := prepeetapi.GetSwagger()
	if err != nil {
		return nil, fmt.Errorf("api: reading the embedded contract: %w", err)
	}
	return sensitiveOperationsIn(spec)
}

// sensitiveOperationsIn is the part of the above that does not need the
// embedded document, so a test can hand it one it built.
func sensitiveOperationsIn(spec *openapi3.T) (map[string]bool, error) {
	declared := map[string]bool{}
	for path, item := range spec.Paths.Map() {
		for method, operation := range item.Operations() {
			raw, present := operation.Extensions[sensitiveReadExtension]
			if !present {
				continue
			}
			value, ok := raw.(bool)
			if !ok {
				// A string "true" reads as declared to a person and as absent
				// to a parser, which is the worst of both.
				return nil, fmt.Errorf("api: %s %s declares %s as %T, want a boolean",
					method, path, sensitiveReadExtension, raw)
			}
			if value {
				declared[operation.OperationID] = true
			}
		}
	}
	return declared, nil
}

// auditSensitiveReads records a declared read before its response is written.
//
// After the handler and before the response, deliberately. Recording first
// would log reads that never happened and could not say how they ended;
// recording after the response was written would let restricted content reach
// somebody with no record of it, which is the one outcome the obligation
// exists to prevent. Between the two, the handler has produced an answer and
// nothing has been sent, so a failure to record refuses the read instead.
func auditSensitiveReads(
	auditor SensitiveReadAuditor,
	identity Identity,
	declared map[string]bool,
	environment config.Environment,
) prepeetapi.StrictMiddlewareFunc {
	return func(f prepeetapi.StrictHandlerFunc, operationID string) prepeetapi.StrictHandlerFunc {
		if !declared[operationID] {
			return f
		}
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
			response, err := f(ctx, w, r, request)

			// No resolvable actor means the request never got past
			// authentication and never reached the content. That is
			// unauthenticated traffic, which the request log and the rate
			// limiter already describe; putting it here would fill the audit
			// trail with probes while telling a reviewer nothing about who did
			// what. The event worth recording is an actor who was refused, and
			// an actor is exactly what that has.
			//
			// It also keeps audit.events keyed. A policy admitting a row with
			// no actor would decide nothing about who is asking, and PostgreSQL
			// ORs permissive policies, so one such policy re-opens the table
			// however well the others are written. internal/isolation caught
			// exactly that when this was tried the other way.
			actor, tenant, resolved := readerFrom(ctx, identity)
			if !resolved {
				return response, err
			}

			read := SensitiveRead{
				Action:      operationID,
				SubjectType: "session",
				SubjectID:   subjectOf(request),
				Outcome:     outcomeOf(response, err),
				RequestID:   httpserver.RequestIDFrom(ctx),
				ActorID:     actor,
				TenantID:    tenant,
			}
			if auditErr := auditor.RecordSensitiveRead(ctx, read); auditErr != nil {
				return failure{
					status:      http.StatusServiceUnavailable,
					code:        "AUDIT_UNAVAILABLE",
					message:     "This cannot be read right now. Please try again shortly.",
					retryable:   true,
					environment: environment,
				}, nil
			}
			return response, err
		}
	}
}

// readerFrom resolves who is asking.
//
// A second lookup, on declared reads only. The handler resolved the same
// principal but cannot hand its context back out, and a row that cannot name
// who read is most of the value gone.
func readerFrom(ctx context.Context, identity Identity) (actor, tenant string, resolved bool) {
	token := sessionTokenFromContext(ctx)
	if token == "" {
		return "", "", false
	}
	principal, err := identity.Lookup(ctx, token)
	if err != nil {
		return "", "", false
	}
	return principal.UserID, principal.ActiveTenantID, true
}

// outcomeOf reads how the handler answered, in audit.events' vocabulary.
//
// Every refusal in this package is a failure value, so the type is the answer
// and no status parsing is needed. A transport error that produced no response
// is recorded as failed: something was attempted and did not complete, which is
// different from being turned away.
func outcomeOf(response any, err error) string {
	if err != nil {
		return "failed"
	}
	refusal, refused := response.(failure)
	if !refused {
		return "allowed"
	}
	if refusal.status >= http.StatusInternalServerError {
		return "failed"
	}
	return "denied"
}

// subjectOf finds what was read, from the generated request object.
//
// By field name rather than by type assertion, so declaring a second operation
// sensitive needs no change here. A request with no such field records an empty
// subject rather than refusing: the read still happened and is still worth a
// row, and an audit that dropped events it could not fully describe would be
// missing exactly the unusual ones.
func subjectOf(request any) string {
	value := reflect.ValueOf(request)
	if value.Kind() != reflect.Struct {
		return ""
	}
	// Both spellings, because the casing follows the generator's initialism
	// rules rather than anything stable, and a subject silently lost to a
	// renamed field would be a row saying somebody read something without
	// saying what.
	for _, name := range []string{"SessionID", "SessionId"} {
		field := value.FieldByName(name)
		if !field.IsValid() || !field.CanInterface() {
			continue
		}
		// A path parameter is not always a string. The contract types this one
		// as a UUID, which the generator renders as a byte array whose String
		// method is the readable form, so the interface is asked rather than
		// the kind assumed.
		switch typed := field.Interface().(type) {
		case string:
			return typed
		case fmt.Stringer:
			return typed.String()
		}
	}
	return ""
}
