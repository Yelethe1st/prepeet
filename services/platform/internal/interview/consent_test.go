package interview_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
)

// The consent document's coherence, and the shipped text itself.
//
// The rules matter because the stored consent_version on a session points at
// this document forever: a text that failed to explain the transcript-only
// forfeit would make that pointer attest to an explanation nobody read.

func TestAParsedConsentDocumentCarriesBothChoicesExplained(t *testing.T) {
	document, err := interview.ParseConsent(json.RawMessage(`{
		"title": "T",
		"statements": ["practice only", "reaffirmed at device check"],
		"choices": {
			"audio_and_transcript": {"label": "A", "explanation": "why"},
			"transcript_only": {"label": "B", "explanation": "why", "forfeits": ["replay"]}
		}
	}`))
	if err != nil {
		t.Fatalf("parsing: %v", err)
	}
	if document.Choices.TranscriptOnly.Forfeits[0] != "replay" {
		t.Fatalf("document = %+v", document)
	}
}

func TestAnIncoherentConsentDocumentIsRefused(t *testing.T) {
	cases := map[string]string{
		"no statements": `{"title":"T","statements":[],"choices":{"audio_and_transcript":{"label":"A","explanation":"e"},"transcript_only":{"label":"B","explanation":"e","forfeits":["r"]}}}`,
		"no forfeits":   `{"title":"T","statements":["s"],"choices":{"audio_and_transcript":{"label":"A","explanation":"e"},"transcript_only":{"label":"B","explanation":"e","forfeits":[]}}}`,
		"a bare choice": `{"title":"T","statements":["s"],"choices":{"audio_and_transcript":{"label":"","explanation":""},"transcript_only":{"label":"B","explanation":"e","forfeits":["r"]}}}`,
		"missing title": `{"title":"","statements":["s"],"choices":{"audio_and_transcript":{"label":"A","explanation":"e"},"transcript_only":{"label":"B","explanation":"e","forfeits":["r"]}}}`,
	}
	for name, raw := range cases {
		if _, err := interview.ParseConsent(json.RawMessage(raw)); err == nil {
			t.Errorf("%s parsed without complaint", name)
		}
	}
}

func TestTheShippedConsentTextParsesAndNamesTheForfeits(t *testing.T) {
	// Across the module boundary, hence -count=1 in test-go.
	raw, err := os.ReadFile("../../../intelligence/artifacts/consent/practice-recording@1.0.0.json")
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	var envelope struct {
		Type string          `json:"type"`
		Body json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Type != "consent_text" {
		t.Fatalf("envelope: %v (%s)", err, envelope.Type)
	}
	document, err := interview.ParseConsent(envelope.Body)
	if err != nil {
		t.Fatalf("the shipped consent text does not parse: %v", err)
	}
	// The second criterion's words: what transcript-only costs is named, not
	// implied, and both replay and delivery measurement are in it.
	if len(document.Choices.TranscriptOnly.Forfeits) < 2 {
		t.Fatalf("forfeits = %v; replay and delivery measurement must both be named", document.Choices.TranscriptOnly.Forfeits)
	}
}
