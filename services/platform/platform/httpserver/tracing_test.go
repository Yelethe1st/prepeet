package httpserver_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	"github.com/Yelethe1st/prepeet/services/platform/platform/httpserver"
	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// A request produces exactly one span, and that span is the anchor everything
// else in an investigation hangs from: the log lines carry its identifier, and
// the spans for the database calls it made are its children.

// recording installs a tracer provider that keeps spans in memory, and returns
// the handler under test plus the recorder. The provider is set globally
// because that is how the middleware obtains its tracer in production, and a
// test that injected one instead would not exercise the same path.
func recording(t *testing.T, routes func(*http.ServeMux)) (http.Handler, *tracetest.SpanRecorder) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))

	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		_ = provider.Shutdown(context.Background())
		otel.SetTracerProvider(previous)
	})

	return httpserver.New(httpserver.Config{Routes: routes}), recorder
}

func attributesOf(span sdktrace.ReadOnlySpan) map[attribute.Key]string {
	found := map[attribute.Key]string{}
	for _, attr := range span.Attributes() {
		found[attr.Key] = attr.Value.Emit()
	}
	return found
}

func TestEveryRequestProducesOneSpan(t *testing.T) {
	handler, recorder := recording(t, nil)

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if ended := recorder.Ended(); len(ended) != 1 {
		t.Fatalf("recorded %d spans for one request, want 1", len(ended))
	}
}

// The span name is the route pattern, never the resolved path.
//
// Two reasons, and the second is the one that would be missed. A name built
// from the path gives every session its own name, which makes latency
// unaggregatable and is the classic way to overwhelm a tracing backend. And a
// path segment is caller controlled, so a name built from it is an
// unclassified string arriving in telemetry.
func TestTheSpanIsNamedForTheRouteNotThePath(t *testing.T) {
	handler, recorder := recording(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /api/v1/sessions/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	})

	handler.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/api/v1/sessions/01a0301d-aa10-7000-8f3e-1234567890ab", nil))

	name := recorder.Ended()[0].Name()
	if strings.Contains(name, "01a0301d") {
		t.Errorf("span name %q contains the identifier from the path, which makes latency unaggregatable", name)
	}
	if want := "GET /api/v1/sessions/{sessionID}"; name != want {
		t.Errorf("span name = %q, want %q", name, want)
	}
}

// An unmatched path must not become a span name either, or anyone can create
// unbounded span names by requesting nonsense.
func TestAnUnmatchedPathGetsAFixedSpanName(t *testing.T) {
	handler, recorder := recording(t, nil)

	handler.ServeHTTP(httptest.NewRecorder(),
		httptest.NewRequest(http.MethodGet, "/no/such/thing/"+strings.Repeat("x", 200), nil))

	if name := recorder.Ended()[0].Name(); strings.Contains(name, "xxx") {
		t.Errorf("span name %q is built from an unmatched path, so a caller controls span naming", name)
	}
}

// The span and the response must agree on the correlation identifier, or a
// user quoting the value from an error message leads to nothing.
func TestTheSpanCarriesTheSameRequestIDAsTheResponse(t *testing.T) {
	handler, recorder := recording(t, nil)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	fromHeader := response.Header().Get("X-Request-ID")
	if fromHeader == "" {
		t.Fatal("the response carries no correlation identifier")
	}
	if fromSpan := attributesOf(recorder.Ended()[0])[attribute.Key(telemetry.KeyRequestID)]; fromSpan != fromHeader {
		t.Errorf("span carries %q and the response carries %q, so neither leads to the other",
			fromSpan, fromHeader)
	}
}

// A trace that starts at the web tier must continue here rather than start
// again, or the two halves of one user action become two unrelated traces.
func TestAnInboundTraceIsContinued(t *testing.T) {
	handler, recorder := recording(t, nil)

	const traceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	request.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")

	handler.ServeHTTP(httptest.NewRecorder(), request)

	span := recorder.Ended()[0]
	if got := span.SpanContext().TraceID().String(); got != traceID {
		t.Errorf("trace ID = %s, want %s: the inbound trace was discarded", got, traceID)
	}
	if !span.Parent().IsValid() {
		t.Error("the span has no parent, so it is the root of a second trace rather than part of the first")
	}
}

// The handler must be able to reach the span, or nothing it does can be
// attributed to the request that caused it.
func TestTheHandlerRunsInsideTheSpan(t *testing.T) {
	var seen trace.SpanContext
	handler, recorder := recording(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /probe", func(w http.ResponseWriter, r *http.Request) {
			seen = trace.SpanContextFromContext(r.Context())
		})
	})

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/probe", nil))

	if !seen.IsValid() {
		t.Fatal("the handler saw no span, so its own work cannot be attached to the request")
	}
	if want := recorder.Ended()[0].SpanContext().SpanID(); seen.SpanID() != want {
		t.Errorf("the handler saw span %s but %s was recorded", seen.SpanID(), want)
	}
}

// Status recording, and the distinction that decides whether an alert fires.
//
// A 4xx is the caller getting it wrong, and marking those as errors means an
// error rate that tracks how many people mistyped a password. A 5xx is this
// system getting it wrong.
func TestOnlyServerFailuresMarkTheSpanAsAnError(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		wantError bool
	}{
		{"a success", http.StatusOK, false},
		{"a rejected request", http.StatusBadRequest, false},
		{"a rate limited request", http.StatusTooManyRequests, false},
		{"a server failure", http.StatusInternalServerError, true},
		{"an unavailable dependency", http.StatusServiceUnavailable, true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			handler, recorder := recording(t, func(mux *http.ServeMux) {
				mux.HandleFunc("GET /probe", func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(testCase.status)
				})
			})

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/probe", nil))

			span := recorder.Ended()[0]
			isError := span.Status().Code == codes.Error
			if isError != testCase.wantError {
				t.Errorf("status %d recorded error=%v, want %v", testCase.status, isError, testCase.wantError)
			}

			attrs := attributesOf(span)
			if got := attrs["http.response.status_code"]; got == "" {
				t.Error("the span does not record the status code, so a failure cannot be found by searching for one")
			}
		})
	}
}

// A handler that never writes a status has still returned 200, and the span
// must say so rather than recording nothing.
func TestAnImplicitStatusIsRecorded(t *testing.T) {
	handler, recorder := recording(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /probe", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("{}"))
		})
	})

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/probe", nil))

	if got := attributesOf(recorder.Ended()[0])["http.response.status_code"]; got != "200" {
		t.Errorf("status code = %q, want 200 for a handler that wrote a body without a status", got)
	}
}

// A panic is the failure most worth recording and the one most likely to be
// lost: the connection drops, the client sees nothing, and without this the
// span never ends and never leaves the process.
func TestAPanicIsRecordedAndAnsweredWithTheErrorEnvelope(t *testing.T) {
	handler, recorder := recording(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
			panic("the connection pool is nil")
		})
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if response.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500: a panic left the client with no answer", response.Code)
	}
	if !strings.Contains(response.Body.String(), "INTERNAL_ERROR") {
		t.Errorf("a panic did not produce the error envelope: %s", response.Body.String())
	}

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("recorded %d spans after a panic, want 1: the span was never ended", len(ended))
	}
	if ended[0].Status().Code != codes.Error {
		t.Error("the span for a panicking request is not marked as an error")
	}
	if len(ended[0].Events()) == 0 {
		t.Error("the panic was not recorded on the span, so the trace shows a 500 with no cause")
	}
}

// The client must never be told what panicked. The message is for the trace.
func TestAPanicDoesNotLeakItsMessageToTheClient(t *testing.T) {
	handler, _ := recording(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
			panic("dial postgres://prepeet:hunter2@db.internal:5432/prepeet: refused")
		})
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/boom", nil))

	for _, secret := range []string{"hunter2", "db.internal", "postgres://"} {
		if strings.Contains(response.Body.String(), secret) {
			t.Errorf("the response body leaks %q from a panic: %s", secret, response.Body.String())
		}
	}
}

// And what is recorded about the panic is scrubbed too, because a panic
// message is free text nobody wrote with a classification in mind.
func TestARecordedPanicIsScrubbed(t *testing.T) {
	handler, recorder := recording(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /boom", func(w http.ResponseWriter, r *http.Request) {
			panic("dial postgres://prepeet:hunter2@db.internal:5432/prepeet: refused")
		})
	})

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	for _, event := range recorder.Ended()[0].Events() {
		for _, attr := range event.Attributes {
			if strings.Contains(attr.Value.Emit(), "hunter2") {
				t.Errorf("the recorded panic carries a credential: %s", attr.Value.Emit())
			}
		}
	}
}

// Without a configured provider the tracer is a noop, and the server must
// still serve. This is the local development path and the first thing that
// would break if tracing were assumed rather than optional.
func TestTheServerWorksWithNoTracerConfigured(t *testing.T) {
	// Nothing is installed here on purpose. The global default is a noop
	// tracer, which is the state a developer running the stack locally is in.
	response := httptest.NewRecorder()
	httpserver.New(httpserver.Config{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if response.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 with no tracer configured", response.Code)
	}
}
