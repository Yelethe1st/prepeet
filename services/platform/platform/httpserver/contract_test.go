package httpserver_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetapi"

	"github.com/Yelethe1st/prepeet/services/platform/platform/health"
	"github.com/Yelethe1st/prepeet/services/platform/platform/httpserver"
)

// These tests bind the hand-written server to the generated contract.
//
// ADR-0004 makes the OpenAPI document the source and generates the Go types
// from it. That only means something if the two are actually checked against
// each other: a generated type nobody uses proves nothing.

// Every error this server produces must use a code the contract declares.
// A code invented in a handler is a code no client can handle, because the
// client's enum comes from the same document.
func TestServerErrorCodesAreDeclaredInTheContract(t *testing.T) {
	t.Parallel()

	declared := map[prepeetapi.ErrorCode]struct{}{}
	for _, code := range []prepeetapi.ErrorCode{
		prepeetapi.VALIDATIONFAILED,
		prepeetapi.UNAUTHENTICATED,
		prepeetapi.FORBIDDEN,
		prepeetapi.NOTFOUND,
		prepeetapi.METHODNOTALLOWED,
		prepeetapi.IDEMPOTENCYCONFLICT,
		prepeetapi.RATELIMITED,
		prepeetapi.CREDENTIALSINVALID,
		prepeetapi.SESSIONEXPIRED,
		prepeetapi.REFRESHTOKENREUSED,
		prepeetapi.INTERNAL,
	} {
		declared[code] = struct{}{}
	}

	handler := httpserver.New(httpserver.Config{Health: health.NewRegistry()})

	// Every error the server can currently produce, exercised through the
	// routes that produce it.
	for name, request := range map[string]*http.Request{
		"unknown route": httptest.NewRequest(http.MethodGet, "/nope", nil),
		"wrong method":  httptest.NewRequest(http.MethodPost, "/healthz", nil),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, request)

			var body prepeetapi.Error
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response does not unmarshal into the generated Error type: %v\n%s",
					err, rec.Body.String())
			}
			if _, ok := declared[body.Error.Code]; !ok {
				t.Errorf("code %q is not declared in the contract", body.Error.Code)
			}
		})
	}
}

// The error envelope the server writes must round-trip through the type
// generated from the contract. If the server added a field, renamed one, or
// dropped a required one, this fails.
func TestErrorEnvelopeMatchesTheGeneratedType(t *testing.T) {
	t.Parallel()

	handler := httpserver.New(httpserver.Config{Health: health.NewRegistry()})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))

	var typed prepeetapi.Error
	decoder := json.NewDecoder(strings.NewReader(rec.Body.String()))
	// Unknown fields are a contract violation in this direction: the server
	// must not be sending anything the client's type does not know about.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&typed); err != nil {
		t.Fatalf("the server sent a field the contract does not declare: %v\n%s", err, rec.Body.String())
	}

	if typed.Error.Code == "" {
		t.Error("code is empty")
	}
	if typed.Error.Message == "" {
		t.Error("message is empty")
	}
	if typed.Error.RequestID == "" {
		t.Error("request_id is empty, and it is what makes a support report traceable")
	}
	if typed.Error.FieldErrors == nil {
		t.Error("field_errors is absent, and the contract declares it required")
	}
}

// The readiness response is read by a load balancer, so its shape is a contract
// obligation rather than a convenience.
func TestReadinessMatchesTheGeneratedType(t *testing.T) {
	t.Parallel()

	registry := health.NewRegistry()
	handler := httpserver.New(httpserver.Config{Health: registry})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	decoder := json.NewDecoder(strings.NewReader(rec.Body.String()))
	decoder.DisallowUnknownFields()

	var typed prepeetapi.Readiness
	if err := decoder.Decode(&typed); err != nil {
		t.Fatalf("readiness does not match the contract: %v\n%s", err, rec.Body.String())
	}
	if typed.Status != prepeetapi.ReadinessStatusReady {
		t.Errorf("status = %q, want %q", typed.Status, prepeetapi.ReadinessStatusReady)
	}
}

func TestLivenessMatchesTheGeneratedType(t *testing.T) {
	t.Parallel()

	handler := httpserver.New(httpserver.Config{Health: health.NewRegistry()})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	decoder := json.NewDecoder(strings.NewReader(rec.Body.String()))
	decoder.DisallowUnknownFields()

	var typed prepeetapi.Liveness
	if err := decoder.Decode(&typed); err != nil {
		t.Fatalf("liveness does not match the contract: %v\n%s", err, rec.Body.String())
	}
}

// The readiness check names a dependency but never why it failed. A driver
// error routinely carries a host, a port and sometimes a credential, and this
// endpoint is unauthenticated.
func TestGeneratedHealthCheckHasNoFieldForFailureDetail(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(prepeetapi.HealthCheck{
		Name:   "database",
		Status: prepeetapi.HealthCheckStatusUnready,
	})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}

	for _, leaky := range []string{"detail", "error", "reason", "message"} {
		if strings.Contains(string(encoded), leaky) {
			t.Errorf("the contract's HealthCheck carries a %q field, which would carry dependency detail into an unauthenticated response", leaky)
		}
	}
}
