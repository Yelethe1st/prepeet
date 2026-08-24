// Package httpserver builds the HTTP handler the api binary serves.
//
// It owns the concerns every route shares: correlation identifiers, request
// tracing, panic recovery, the single error envelope from
// docs/contracts/public-api.md, and the deployment probes.
// Product routes are mounted onto it by their own bounded context; this package
// never holds a product rule.
//
// Implements part of PLT-01 and the request path half of PLT-08.
package httpserver

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/Yelethe1st/prepeet/services/platform/platform/health"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// maxRequestIDLength bounds the inbound correlation header. The value is
// reflected into responses and written to logs, so an unbounded one is a log
// flooding vector.
const maxRequestIDLength = 128

// requestIDHeader is the correlation header named in docs/contracts/public-api.md.
const requestIDHeader = "X-Request-ID"

// Config is the dependency set for the handler.
type Config struct {
	// Health is consulted by the readiness probe. A nil registry is treated as
	// one with no dependencies, which reports ready.
	Health *health.Registry

	// Routes mounts the product endpoints. A bounded context registers its own
	// handlers here rather than this package knowing they exist, which is what
	// keeps this package free of product rules.
	Routes func(mux *http.ServeMux)
}

type contextKey int

const requestIDKey contextKey = iota

// RequestIDFrom returns the correlation identifier for ctx, or an empty string
// if the request did not pass through this handler.
//
// Every log line and every error response carries this value, which is what
// makes a user's support report resolvable to a single trace.
func RequestIDFrom(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

// New returns the root HTTP handler.
func New(cfg Config) http.Handler {
	registry := cfg.Health
	if registry == nil {
		registry = health.NewRegistry()
	}

	mux := http.NewServeMux()

	// Liveness deliberately ignores dependencies. If a failing database made
	// this endpoint fail, the orchestrator would restart healthy processes
	// during a database outage and turn a degradation into an outage.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "alive"})
	})

	// Readiness reports whether this process can serve traffic. 503 tells the
	// load balancer to route elsewhere without restarting anything.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		report := registry.Check(r.Context())
		status := http.StatusOK
		if report.Status != health.StatusReady {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, r, status, report)
	})

	if cfg.Routes != nil {
		cfg.Routes(mux)
	}

	// Go's ServeMux answers an unmatched path or method with a plain text body.
	// Every failure in this API uses one envelope, so both are replaced here.
	root := http.NewServeMux()
	root.Handle("/", notFound(mux))

	// Order matters. The correlation identifier is established first so the span
	// can carry it, and tracing wraps the routes so a panic anywhere inside them
	// still ends its span.
	return withRequestID(withTracing(mux, root))
}

// notFound converts ServeMux's built in 404 and 405 responses into the API
// error envelope, without changing which status code it chose.
func notFound(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := mux.Handler(r)
		if pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}

		// An empty pattern means no route matched the path at all. A path that
		// exists under another method resolves to a pattern, so ServeMux is
		// left to answer 405 itself; it is intercepted below.
		if allowed := allowedMethods(mux, r); len(allowed) > 0 {
			for _, method := range allowed {
				w.Header().Add("Allow", method)
			}
			WriteError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED",
				"This endpoint does not accept that HTTP method.", false)
			return
		}

		WriteError(w, r, http.StatusNotFound, "NOT_FOUND",
			"No endpoint matches this path.", false)
	})
}

// allowedMethods reports which methods the mux would accept for this path. It
// is used to tell "wrong method" apart from "no such route", which are
// different problems for the caller.
func allowedMethods(mux *http.ServeMux, r *http.Request) []string {
	var allowed []string
	for _, method := range []string{
		http.MethodGet, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete,
	} {
		if method == r.Method {
			continue
		}
		probe := r.Clone(r.Context())
		probe.Method = method
		if _, pattern := mux.Handler(probe); pattern != "" {
			allowed = append(allowed, method)
		}
	}
	return allowed
}

// withRequestID attaches a correlation identifier to every request, reusing a
// caller supplied one when it is safe to reuse.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := sanitiseRequestID(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = id.Prefixed("req")
		}

		w.Header().Set(requestIDHeader, requestID)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, requestID)))
	})
}

// sanitiseRequestID accepts a caller supplied correlation identifier only when
// it is safe to echo. The header is attacker controlled and is written to both
// responses and logs, so control characters, which allow header and log
// injection, and unbounded lengths are rejected outright rather than escaped.
func sanitiseRequestID(candidate string) string {
	if candidate == "" || len(candidate) > maxRequestIDLength {
		return ""
	}
	for _, r := range candidate {
		if r < 0x20 || r == 0x7F {
			return ""
		}
	}
	return candidate
}

// WriteError writes the single API error envelope.
//
// The shape is fixed by docs/contracts/public-api.md: a stable machine readable
// code, a human readable message that is never machine logic, whether retrying
// could help, field level errors for validation failures, and the correlation
// identifier so a user can quote it to support.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, retryable bool) {
	type fieldError struct {
		Field   string `json:"field"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	type envelope struct {
		Error struct {
			Code        string       `json:"code"`
			Message     string       `json:"message"`
			Retryable   bool         `json:"retryable"`
			FieldErrors []fieldError `json:"field_errors"`
			RequestID   string       `json:"request_id"`
		} `json:"error"`
	}

	var body envelope
	body.Error.Code = code
	body.Error.Message = message
	body.Error.Retryable = retryable
	body.Error.FieldErrors = []fieldError{}
	body.Error.RequestID = RequestIDFrom(r.Context())

	writeJSON(w, r, status, body)
}

// writeJSON writes a JSON response with the headers every response in this API
// carries. Probe and error responses are never cacheable: a cached readiness
// answer would keep traffic flowing to a process that has already failed.
func writeJSON(w http.ResponseWriter, _ *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)

	// The response is already committed by WriteHeader, so an encoding failure
	// cannot be reported to the client. It is dropped here and surfaces as a
	// truncated body plus a server side log once PLT-08 lands.
	_ = json.NewEncoder(w).Encode(body)
}
