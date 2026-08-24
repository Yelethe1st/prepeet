package telemetry_test

import (
	"bytes"
	"context"
	"log/slog"
	"regexp"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// SEC-08 requires a scanner that fails the build when restricted content
// reaches telemetry. This is it.
//
// The distinction from the tests in attributes_test.go matters. Those check the
// scrubber in isolation, which proves the scrubber works. This one records real
// spans and real log output and inspects what actually came out, which proves
// the scrubber is on the path. A correct scrubber nobody calls is the failure
// mode a unit test cannot see.

// The shapes that must never appear in telemetry, whatever produced them.
var restricted = map[string]*regexp.Regexp{
	"an email address":    regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
	"a bearer token":      regexp.MustCompile(`\b(ses|ref|vrf|rst|mgc|inv)_[A-Za-z0-9_\-]{16,}`),
	"a password hash":     regexp.MustCompile(`\$argon2(id|i|d)\$`),
	"a connection string": regexp.MustCompile(`[a-z]+://[^:/\s]+:[^@/\s]+@`),
}

// assertClean fails naming both the shape and where it was found, because the
// useful part of this failure is which call site to fix.
func assertClean(t *testing.T, where, text string) {
	t.Helper()

	for shape, pattern := range restricted {
		if match := pattern.FindString(text); match != "" {
			t.Errorf("%s carries %s: %q\n  in: %s", where, shape, match, text)
		}
	}
}

// A span carrying what a handler would plausibly attach during an incident.
func TestRecordedSpansCarryNoRestrictedContent(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	ctx, span := provider.Tracer("test").Start(context.Background(), "authenticate")

	// Everything below is what somebody debugging a failed login would reach
	// for, and every one of them is restricted.
	span.SetAttributes(
		telemetry.MustAttr(telemetry.KeyRequestID, "req_01a03"),
		telemetry.MustAttr(telemetry.KeyOutcome, "credentials did not match for daniel.okonkwo@example.com"),
		telemetry.MustAttr(telemetry.KeyErrorCode, "rejected token ses_AbCdEf0123456789AbCdEf0123456789"),
	)
	span.End()
	_ = ctx

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(ended))
	}
	for _, attr := range ended[0].Attributes() {
		assertClean(t, "span attribute "+string(attr.Key), attr.Value.Emit())
	}
}

// The logger is the likelier leak. Attributes are deliberate; a log message is
// written in the moment and often interpolates whatever variable is to hand.
func TestLogOutputCarriesNoRestrictedContent(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&out, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Value.Kind() == slog.KindString {
				attr.Value = slog.StringValue(telemetry.Scrub(attr.Value.String()))
			}
			return attr
		},
	}))

	logger.Info(telemetry.Scrub("no account for amara.osei@example.com"),
		slog.String("token", "ses_ZzYyXx9876543210ZzYyXx9876543210"),
		slog.String("dsn", "postgres://prepeet:hunter2@db.internal:5432/prepeet"),
		slog.String("credential", "$argon2id$v=19$m=65536,t=2,p=1$c2FsdA$aGFzaA"))

	assertClean(t, "log output", out.String())
}

// The logger built by this package must scrub without the caller remembering
// to, or the rule is a convention again. This calls NewLogger itself rather
// than rebuilding its handler chain, because the chain being correct somewhere
// else is not the property that matters.
func TestTheProvidedLoggerScrubsWithoutBeingAsked(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := telemetry.NewLogger(telemetry.Config{
		ServiceName: "api",
		Environment: "test",
	}, &out)

	// Nothing here calls Scrub. That is the point: a caller in a hurry will not.
	logger.Info("no account for amara.osei@example.com",
		slog.String("token", "ses_ZzYyXx9876543210ZzYyXx9876543210"),
		slog.String("dsn", "postgres://prepeet:hunter2@db.internal:5432/prepeet"),
		slog.String("credential", "$argon2id$v=19$m=65536,t=2,p=1$c2FsdA$aGFzaA"))

	assertClean(t, "NewLogger output with no explicit Scrub call", out.String())

	// And it must still say which service produced the line, or an operator
	// cannot use it.
	if !strings.Contains(out.String(), `"service":"api"`) {
		t.Errorf("the log line does not identify its service: %s", out.String())
	}
}

// A log line written inside a span must carry the trace, or the log and the
// trace describing the same moment stay unjoinable, which is the state that
// makes people paste content into log messages to compensate.
func TestLogLinesCarryTheActiveTrace(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	var out bytes.Buffer
	logger := telemetry.NewLogger(telemetry.Config{ServiceName: "api", Environment: "test"}, &out)

	ctx, span := provider.Tracer("test").Start(context.Background(), "authenticate")
	logger.InfoContext(ctx, "credentials rejected")
	span.End()

	wanted := span.SpanContext().TraceID().String()
	if !strings.Contains(out.String(), wanted) {
		t.Errorf("the log line does not carry trace %s, so it cannot be joined to its span: %s",
			wanted, out.String())
	}
}

// Removing content is only half of it. A span that says nothing is a span
// nobody can use, and the temptation to add the leaking attribute comes from a
// span that did not carry enough.
func TestSpansStillCarryEnoughToBeUseful(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	defer func() { _ = provider.Shutdown(context.Background()) }()

	_, span := provider.Tracer("test").Start(context.Background(), "evaluate")
	span.SetAttributes(
		telemetry.MustAttr(telemetry.KeyRequestID, "req_01a03"),
		telemetry.MustAttr(telemetry.KeySessionID, "ses_7Kq2XA"),
		telemetry.MustAttr(telemetry.KeyTenantID, "tn_northwind"),
		telemetry.MustAttr(telemetry.KeyOutcome, "insufficient_evidence"),
	)
	span.End()

	found := map[attribute.Key]string{}
	for _, attr := range recorder.Ended()[0].Attributes() {
		found[attr.Key] = attr.Value.Emit()
	}

	for _, wanted := range []telemetry.Key{
		telemetry.KeyRequestID, telemetry.KeySessionID, telemetry.KeyTenantID, telemetry.KeyOutcome,
	} {
		value, present := found[attribute.Key(wanted)]
		if !present {
			t.Errorf("%s is missing, so the span cannot be traced back to what it describes", wanted)
			continue
		}
		if value == "" || strings.Contains(value, "redacted") {
			t.Errorf("%s = %q, and an identifier should survive scrubbing intact", wanted, value)
		}
	}
}

// An identifier is not restricted content, and scrubbing one would make every
// span useless while protecting nothing: it resolves to a record only for
// somebody already authorised to read it.
func TestIdentifiersSurviveScrubbing(t *testing.T) {
	t.Parallel()

	for _, identifier := range []string{
		"req_01a0301daa1070009",
		"ses_7Kq2XA",
		"tn_northwind",
		"cmp_icu_autumn",
		"01a0301d-aa10-7000-8f3e-1234567890ab",
	} {
		if scrubbed := telemetry.Scrub(identifier); scrubbed != identifier {
			t.Errorf("Scrub(%q) = %q, and an identifier carries nothing restricted", identifier, scrubbed)
		}
	}
}
