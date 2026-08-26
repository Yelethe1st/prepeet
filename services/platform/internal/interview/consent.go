package interview

import (
	"encoding/json"
	"errors"
	"fmt"
)

// The practice recording consent document: what the candidate is shown when
// they choose what a session keeps, versioned as a registry artifact so the
// consent_version a session stores points at exact words forever.
//
// Implements part of CAT-05. Parsing lives in this context because the
// session is what the consent binds to; the loader runs it as the artifact's
// validating step, so a text that fails to explain both choices - or names
// no forfeit for transcript-only - never publishes at all.

// RecordingPreferences are the two things a practice session can keep.
const (
	RecordingAudioAndTranscript = "audio_and_transcript"
	RecordingTranscriptOnly     = "transcript_only"
)

// ConsentReference is the registry reference the practice recording consent
// text lives under.
const ConsentReference = "consent/practice-recording"

// ErrConsentIncoherent means the consent document contradicts its own job.
var ErrConsentIncoherent = errors.New("interview: the consent document is incoherent")

// ConsentChoice is one of the two options, explained.
type ConsentChoice struct {
	Label       string `json:"label"`
	Explanation string `json:"explanation"`
	// Forfeits is what choosing this visibly costs. Required for
	// transcript_only: a forfeit that is not named is a forfeit nobody chose.
	Forfeits []string `json:"forfeits,omitempty"`
}

// ConsentDocument is the parsed consent text.
type ConsentDocument struct {
	Title      string   `json:"title"`
	Statements []string `json:"statements"`
	Choices    struct {
		AudioAndTranscript ConsentChoice `json:"audio_and_transcript"`
		TranscriptOnly     ConsentChoice `json:"transcript_only"`
	} `json:"choices"`
}

// ParseConsent decodes and coheres one consent document.
func ParseConsent(body json.RawMessage) (ConsentDocument, error) {
	var document ConsentDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return ConsentDocument{}, fmt.Errorf("%w: %v", ErrConsentIncoherent, err)
	}
	if document.Title == "" {
		return ConsentDocument{}, fmt.Errorf("%w: the title is missing", ErrConsentIncoherent)
	}
	if len(document.Statements) == 0 {
		return ConsentDocument{}, fmt.Errorf("%w: a consent text with nothing to say is not one", ErrConsentIncoherent)
	}
	for name, choice := range map[string]ConsentChoice{
		RecordingAudioAndTranscript: document.Choices.AudioAndTranscript,
		RecordingTranscriptOnly:     document.Choices.TranscriptOnly,
	} {
		if choice.Label == "" || choice.Explanation == "" {
			return ConsentDocument{}, fmt.Errorf("%w: choice %s is not explained", ErrConsentIncoherent, name)
		}
	}
	if len(document.Choices.TranscriptOnly.Forfeits) == 0 {
		return ConsentDocument{}, fmt.Errorf("%w: transcript_only names no forfeit; an unnamed cost is one nobody agreed to", ErrConsentIncoherent)
	}
	return document, nil
}
