package httpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/platform/health"
	"github.com/Yelethe1st/prepeet/services/platform/platform/httpserver"
)

func newHandler(t *testing.T, checks map[string]health.CheckFunc) http.Handler {
	t.Helper()
	registry := health.NewRegistry()
	for name, check := range checks {
		registry.Register(name, check)
	}
	return httpserver.New(httpserver.Config{Health: registry})
}

func do(t *testing.T, handler http.Handler, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// Liveness says the process is running. It must not consult dependencies: a
// database outage should stop traffic, not trigger a restart loop that makes
// the outage worse.
func TestLivenessIgnoresDependencies(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, map[string]health.CheckFunc{
		"database": func(context.Context) error { return errors.New("connection refused") },
	})

	rec := do(t, handler, http.MethodGet, "/healthz", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadinessIsOKWhenEveryDependencyAnswers(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, map[string]health.CheckFunc{
		"database": func(context.Context) error { return nil },
	})

	rec := do(t, handler, http.MethodGet, "/readyz", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	var report health.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("body is not a health report: %v", err)
	}
	if report.Status != health.StatusReady {
		t.Errorf("status = %q, want %q", report.Status, health.StatusReady)
	}
}

// An unready service returns 503 so a load balancer removes it from rotation.
func TestReadinessIsUnavailableWhenADependencyFails(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, map[string]health.CheckFunc{
		"temporal": func(context.Context) error { return errors.New("dial tcp: connection refused") },
	})

	rec := do(t, handler, http.MethodGet, "/readyz", nil)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

// The readiness endpoint is unauthenticated. A dependency error routinely
// carries a host, a port and sometimes a credential, so none of it may appear.
func TestReadinessBodyNeverCarriesDependencyDetail(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, map[string]health.CheckFunc{
		"database": func(context.Context) error {
			return errors.New("postgres://prepeet:hunter2@db.internal:5432 refused")
		},
	})

	body := do(t, handler, http.MethodGet, "/readyz", nil).Body.String()

	for _, secret := range []string{"hunter2", "db.internal", "5432", "postgres://"} {
		if strings.Contains(body, secret) {
			t.Errorf("body contains %q, want dependency detail withheld:\n%s", secret, body)
		}
	}
}

// Every response carries a correlation identifier so a support report can be
// tied to a trace. docs/contracts/public-api.md names X-Request-ID.
func TestEveryResponseCarriesARequestID(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, nil)

	for _, target := range []string{"/healthz", "/readyz", "/does-not-exist"} {
		rec := do(t, handler, http.MethodGet, target, nil)
		if got := rec.Header().Get("X-Request-ID"); got == "" {
			t.Errorf("%s: X-Request-ID is empty, want a generated identifier", target)
		}
	}
}

func TestInboundRequestIDIsEchoed(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, nil)
	const inbound = "req_0190a1b2c3d47abc8abc0123456789ab"

	rec := do(t, handler, http.MethodGet, "/healthz", map[string]string{"X-Request-ID": inbound})

	if got := rec.Header().Get("X-Request-ID"); got != inbound {
		t.Errorf("X-Request-ID = %q, want the inbound value %q", got, inbound)
	}
}

// An inbound header is attacker controlled. It is reflected into responses and
// into logs, so anything unreasonable is replaced rather than echoed.
func TestUnreasonableInboundRequestIDIsReplaced(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, nil)
	cases := map[string]string{
		"too long":     strings.Repeat("a", 200),
		"newline":      "abc\r\nX-Injected: yes",
		"control char": "abc\x00def",
		"empty":        "",
	}

	for name, inbound := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rec := do(t, handler, http.MethodGet, "/healthz", map[string]string{"X-Request-ID": inbound})

			got := rec.Header().Get("X-Request-ID")
			if got == inbound && inbound != "" {
				t.Errorf("X-Request-ID = %q, want the untrusted value replaced", got)
			}
			if got == "" {
				t.Error("X-Request-ID is empty, want a generated identifier")
			}
		})
	}
}

// Errors use one envelope across the whole API, so a client parses failures the
// same way everywhere. docs/contracts/public-api.md fixes the shape.
func TestUnknownRouteReturnsTheErrorEnvelope(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, nil)

	rec := do(t, handler, http.MethodGet, "/nope", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", got)
	}

	var body struct {
		Error struct {
			Code        string            `json:"code"`
			Message     string            `json:"message"`
			Retryable   bool              `json:"retryable"`
			FieldErrors []json.RawMessage `json:"field_errors"`
			RequestID   string            `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not the error envelope: %v\n%s", err, rec.Body.String())
	}
	if body.Error.Code != "NOT_FOUND" {
		t.Errorf("code = %q, want %q", body.Error.Code, "NOT_FOUND")
	}
	if body.Error.Message == "" {
		t.Error("message is empty, want a human readable message")
	}
	if body.Error.Retryable {
		t.Error("retryable = true, want false: retrying a missing route will not help")
	}
	if body.Error.RequestID != rec.Header().Get("X-Request-ID") {
		t.Errorf("request_id = %q, want it to match the X-Request-ID header %q",
			body.Error.RequestID, rec.Header().Get("X-Request-ID"))
	}
}

// A route that exists but does not accept this method is a different failure
// from a route that does not exist, and the client can act on the difference.
func TestWrongMethodIsRejectedDistinctly(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, nil)

	rec := do(t, handler, http.MethodPost, "/healthz", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if got := rec.Header().Get("Allow"); got != http.MethodGet {
		t.Errorf("Allow = %q, want %q so the client knows what would work", got, http.MethodGet)
	}

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not the error envelope: %v\n%s", err, rec.Body.String())
	}
	if body.Error.Code != "METHOD_NOT_ALLOWED" {
		t.Errorf("code = %q, want %q", body.Error.Code, "METHOD_NOT_ALLOWED")
	}
}

// The probe endpoints are for the deployment, not for a browser. Caching a
// readiness response would let a stale answer keep traffic flowing.
func TestProbeResponsesAreNotCacheable(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, nil)

	for _, target := range []string{"/healthz", "/readyz"} {
		rec := do(t, handler, http.MethodGet, target, nil)
		if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Errorf("%s: Cache-Control = %q, want it to contain no-store", target, got)
		}
	}
}
