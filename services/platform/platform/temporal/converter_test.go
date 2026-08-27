package temporal_test

import (
	"log/slog"
	"strings"
	"testing"

	"go.temporal.io/sdk/converter"

	"github.com/Yelethe1st/prepeet/services/platform/platform/temporal"
)

// ADR-0007 says a workflow carries identifiers and small control values, never
// transcript text, evaluation prose, CV content, model output or candidate
// contact details.
//
// Workflow history is durable storage on its own retention schedule, outside
// the deletion machinery that governs the tables. An activity taking a
// transcript as an argument silently creates a second copy of it in a store
// nobody classified, and a deletion request cannot reach it.
//
// So the rule is enforced on the encode path rather than at review. These
// assertions are what makes that claim true.

// A workflow argument as it should look: identifiers and control values.
type startInterview struct {
	SessionID string `json:"session_id"`
	TenantID  string `json:"tenant_id"`
	RubricVer int    `json:"rubric_version"`
	Mode      string `json:"mode"`
}

func TestAnIdentifierOnlyPayloadIsAccepted(t *testing.T) {
	t.Parallel()

	payload, err := temporal.NewDataConverter().ToPayload(startInterview{
		SessionID: "ses_7Kq2XA",
		TenantID:  "01a0301d-aa10-7000-8f3e-1234567890ab",
		RubricVer: 4,
		Mode:      "screening",
	})
	if err != nil {
		t.Fatalf("a payload of identifiers was refused: %v", err)
	}
	if payload == nil {
		t.Fatal("ToPayload returned nothing")
	}
}

// The failure this exists to prevent, and the likeliest one: somebody passes
// the transcript to the evaluation activity because that is what evaluation
// needs, and it is durably stored for a month.
func TestABulkTextPayloadIsRefused(t *testing.T) {
	t.Parallel()

	transcript := strings.Repeat("The candidate described their approach at some length. ", 200)

	_, err := temporal.NewDataConverter().ToPayload(map[string]string{"transcript": transcript})
	if err == nil {
		t.Fatal("a transcript-sized payload was accepted into workflow history")
	}
	if !strings.Contains(err.Error(), "identifier") {
		t.Errorf("the refusal does not say what to do instead: %v", err)
	}
}

// The size rule alone would let a short address through, and contact details
// are restricted regardless of length.
func TestAPayloadCarryingContactDetailsIsRefused(t *testing.T) {
	t.Parallel()

	for _, value := range []any{
		map[string]string{"email": "daniel.okonkwo@example.com"},
		map[string]string{"note": "could not reach amara.osei@example.com"},
		map[string]string{"token": "ses_AbCdEf0123456789AbCdEf0123456789"},
		map[string]string{"dsn": "postgres://prepeet:hunter2@db.internal:5432/prepeet"},
	} {
		if _, err := temporal.NewDataConverter().ToPayload(value); err == nil {
			t.Errorf("a payload carrying %v was accepted into workflow history", value)
		}
	}
}

// The refusal must not quote what it found. Otherwise the error message carries
// the content into the log the refusal was protecting.
func TestARefusalDoesNotQuoteWhatItFound(t *testing.T) {
	t.Parallel()

	_, err := temporal.NewDataConverter().ToPayload(map[string]string{"email": "daniel.okonkwo@example.com"})
	if err == nil {
		t.Fatal("the payload was accepted")
	}
	if strings.Contains(err.Error(), "daniel.okonkwo") {
		t.Errorf("the refusal quotes the address it refused: %v", err)
	}
}

// Round tripping must work, or the converter is a filter rather than a
// converter and every workflow argument arrives as nothing.
func TestAPayloadRoundTrips(t *testing.T) {
	t.Parallel()

	dataConverter := temporal.NewDataConverter()
	sent := startInterview{SessionID: "ses_7Kq2XA", TenantID: "tn_1", RubricVer: 4, Mode: "practice"}

	payload, err := dataConverter.ToPayload(sent)
	if err != nil {
		t.Fatalf("ToPayload: %v", err)
	}

	var received startInterview
	if err := dataConverter.FromPayload(payload, &received); err != nil {
		t.Fatalf("FromPayload: %v", err)
	}
	if received != sent {
		t.Errorf("round trip changed the value:\n  sent:     %+v\n  received: %+v", sent, received)
	}
}

func TestSeveralArgumentsRoundTrip(t *testing.T) {
	t.Parallel()

	dataConverter := temporal.NewDataConverter()

	payloads, err := dataConverter.ToPayloads("ses_7Kq2XA", 4, true)
	if err != nil {
		t.Fatalf("ToPayloads: %v", err)
	}

	var (
		session string
		version int
		enabled bool
	)
	if err := dataConverter.FromPayloads(payloads, &session, &version, &enabled); err != nil {
		t.Fatalf("FromPayloads: %v", err)
	}
	if session != "ses_7Kq2XA" || version != 4 || !enabled {
		t.Errorf("round trip gave %q, %d, %v", session, version, enabled)
	}
}

// One bad argument must refuse the whole call rather than being dropped, or a
// workflow starts with an argument silently missing.
func TestOneBadArgumentRefusesThemAll(t *testing.T) {
	t.Parallel()

	if _, err := temporal.NewDataConverter().ToPayloads(
		"ses_7Kq2XA",
		strings.Repeat("transcript text ", 500),
	); err == nil {
		t.Fatal("a call with one oversized argument was accepted")
	}
}

// Decoding is not filtered. History written before a rule tightened must still
// be readable, or the rule change bricks every in-flight workflow, and a
// refusal on the way out protects nothing: the content is already stored.
func TestDecodingIsNotFiltered(t *testing.T) {
	t.Parallel()

	// Encoded with a plain converter, standing in for history written earlier.
	payload, err := converter.GetDefaultDataConverter().ToPayload(
		map[string]string{"note": "written before the rule existed, for daniel.okonkwo@example.com"})
	if err != nil {
		t.Fatalf("building the prior payload: %v", err)
	}

	var received map[string]string
	if err := temporal.NewDataConverter().FromPayload(payload, &received); err != nil {
		t.Errorf("history written before the rule cannot be decoded, so in-flight workflows break: %v", err)
	}
}

// nil is a legitimate argument and a legitimate result.
func TestNilIsAccepted(t *testing.T) {
	t.Parallel()

	if _, err := temporal.NewDataConverter().ToPayload(nil); err != nil {
		t.Errorf("nil was refused: %v", err)
	}
}

// The cap has to be a real number somebody can look up, not an implementation
// detail, because the answer to "why was my activity argument refused" is this
// constant.
func TestTheCapIsSmallEnoughToMeanIdentifiersOnly(t *testing.T) {
	t.Parallel()

	if temporal.MaxPayloadBytes > 8192 {
		t.Errorf("MaxPayloadBytes is %d, which is large enough to hold prose, "+
			"so the rule stops being about identifiers", temporal.MaxPayloadBytes)
	}
}

func TestEveryLogLevelScrubsWhatTheSDKHandsIt(t *testing.T) {
	var written strings.Builder
	logger := slog.New(slog.NewJSONHandler(&written, &slog.HandlerOptions{Level: slog.LevelDebug}))

	temporal.LogEveryLevelForTest(logger,
		"dialling for ama@example.com", "peer", "ama@example.com", "attempt", 2)

	line := written.String()
	if strings.Contains(line, "ama@example.com") {
		t.Fatalf("an address reached the log: %s", line)
	}
	// All four levels wrote, and the non-string value is passed through
	// untouched rather than stringified.
	for _, level := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		if !strings.Contains(line, level) {
			t.Fatalf("%s did not write: %s", level, line)
		}
	}
	if !strings.Contains(line, `"attempt":2`) {
		t.Fatalf("the attempt number was altered: %s", line)
	}
}

func TestTheConverterRendersPayloadsForOperators(t *testing.T) {
	// ToString and ToStrings are what the CLI and the UI call to show a
	// payload; they delegate, and what matters is that they answer
	// something rather than panicking on the wrapper.
	converter := temporal.NewDataConverter()

	payload, err := converter.ToPayload("hello")
	if err != nil {
		t.Fatalf("to payload: %v", err)
	}
	if rendered := converter.ToString(payload); !strings.Contains(rendered, "hello") {
		t.Fatalf("rendered = %q", rendered)
	}

	payloads, err := converter.ToPayloads("first", "second")
	if err != nil {
		t.Fatalf("to payloads: %v", err)
	}
	rendered := converter.ToStrings(payloads)
	if len(rendered) != 2 || !strings.Contains(rendered[1], "second") {
		t.Fatalf("rendered = %v", rendered)
	}
}
