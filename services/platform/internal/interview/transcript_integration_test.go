//go:build integration

package interview_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// RTC-04 against real PostgreSQL: original and corrected text both retained
// with sequence and timing, word timing surviving storage exactly, and
// confidence carried per segment.

func finalSegment(sequence int, speaker, text string, startMs int, words []interview.TranscriptWord, confidence float64) interview.ControlEvent {
	payload, _ := json.Marshal(map[string]any{
		"speaker": speaker, "text": text,
		"start_ms": startMs, "end_ms": startMs + 3000,
		"confidence": confidence, "words": words,
	})
	return interview.ControlEvent{
		EventID: id.New().String(), Epoch: 1, Sequence: sequence,
		Type: "transcript.segment.final", Payload: payload,
		OccurredAt: time.Date(2026, 8, 26, 13, 0, sequence, 0, time.UTC),
	}
}

func correction(sequence, supersedes int, text string, confidence float64) interview.ControlEvent {
	payload, _ := json.Marshal(map[string]any{
		"speaker": "candidate", "text": text,
		"start_ms": 5000, "end_ms": 8000,
		"confidence": confidence, "supersedes_sequence": supersedes,
	})
	return interview.ControlEvent{
		EventID: id.New().String(), Epoch: 1, Sequence: sequence,
		Type: "transcript.segment.corrected", Payload: payload,
		OccurredAt: time.Date(2026, 8, 26, 13, 0, sequence, 0, time.UTC),
	}
}

func words(startMs int, texts ...string) []interview.TranscriptWord {
	out := make([]interview.TranscriptWord, 0, len(texts))
	cursor := startMs
	for _, text := range texts {
		out = append(out, interview.TranscriptWord{
			Word: text, StartMs: cursor, EndMs: cursor + 400, Confidence: 0.97,
		})
		cursor += 500
	}
	return out
}

func TestCorrectionSupersedesWithoutErasing(t *testing.T) {
	ctx := context.Background()
	events := interview.NewEvents(interview.NewStore(pool))
	session := startedPractice(t)

	original := finalSegment(2, "candidate", "I lead the migration", 5000,
		words(5000, "I", "lead", "the", "migration"), 0.81)
	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(1, "connection.established"), original,
			correction(3, 2, "I led the migration", 0.95)}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	transcript, err := events.AssembleTranscript(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	// Both versions stand, linked both ways, each with its own sequence.
	var kept, corrected *interview.TranscriptSegment
	for i := range transcript.Segments {
		switch transcript.Segments[i].Sequence {
		case 2:
			kept = &transcript.Segments[i]
		case 3:
			corrected = &transcript.Segments[i]
		}
	}
	if kept == nil || corrected == nil {
		t.Fatalf("segments = %+v", transcript.Segments)
	}
	if !kept.Superseded || kept.CorrectedBySequence != 3 || kept.Text != "I lead the migration" {
		t.Fatalf("the original changed or lost its link: %+v", kept)
	}
	if corrected.Supersedes != 2 || corrected.Text != "I led the migration" {
		t.Fatalf("the correction lost its provenance: %+v", corrected)
	}
	// The original's word timing remains the alignment source.
	if len(kept.Words) != 4 || kept.Words[1].Word != "lead" || kept.Words[1].StartMs != 5500 {
		t.Fatalf("word timing did not survive: %+v", kept.Words)
	}

	// The effective view quotes the correction and only the correction.
	effective := transcript.EffectiveText()
	texts := make([]string, 0, len(effective))
	for _, segment := range effective {
		if segment.Type == "transcript.segment.final" || segment.Type == "transcript.segment.corrected" {
			texts = append(texts, segment.Text)
		}
	}
	if !reflect.DeepEqual(texts, []string{"I led the migration"}) {
		t.Fatalf("effective = %v", texts)
	}
}

func TestTheLatestCorrectionWinsAndEveryVersionRemains(t *testing.T) {
	ctx := context.Background()
	events := interview.NewEvents(interview.NewStore(pool))
	session := startedPractice(t)

	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{
			event(1, "connection.established"),
			finalSegment(2, "candidate", "forty services", 5000, words(5000, "forty", "services"), 0.7),
			correction(3, 2, "fourteen services", 0.85),
			correction(4, 2, "forty-two services", 0.97),
		}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	transcript, err := events.AssembleTranscript(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	effective := transcript.EffectiveText()
	var effectiveTexts []string
	for _, segment := range effective {
		if segment.Supersedes != 0 || segment.Type == "transcript.segment.final" {
			effectiveTexts = append(effectiveTexts, segment.Text)
		}
	}
	if !reflect.DeepEqual(effectiveTexts, []string{"forty-two services"}) {
		t.Fatalf("effective = %v", effectiveTexts)
	}
	// All three versions retained: the trail answers what was said, what it
	// was first corrected to, and what stands now. Non-transcript events
	// never enter the read model.
	if len(transcript.Segments) != 3 {
		t.Fatalf("%d segments retained, want all three versions", len(transcript.Segments))
	}
	superseded := 0
	for _, segment := range transcript.Segments {
		if segment.Superseded {
			superseded++
			if segment.CorrectedBySequence != 4 {
				t.Fatalf("segment %d is superseded by %d, want the latest correction 4",
					segment.Sequence, segment.CorrectedBySequence)
			}
		}
	}
	if superseded != 2 {
		t.Fatalf("%d superseded versions, want the original and the first correction", superseded)
	}
}

func TestConfidenceIsPerSegmentAndInvalidSegmentsNeverEnter(t *testing.T) {
	ctx := context.Background()
	events := interview.NewEvents(interview.NewStore(pool))
	session := startedPractice(t)

	low := finalSegment(2, "candidate", "mumbled answer", 5000, words(5000, "mumbled", "answer"), 0.31)
	high := finalSegment(3, "interviewer", "Could you repeat that?", 9000,
		words(9000, "Could", "you", "repeat", "that?"), 0.99)

	missingWords, _ := json.Marshal(map[string]any{
		"speaker": "candidate", "text": "no timing here",
		"start_ms": 12000, "end_ms": 13000, "confidence": 0.9,
	})
	invalid := interview.ControlEvent{
		EventID: id.New().String(), Epoch: 1, Sequence: 4,
		Type: "transcript.segment.final", Payload: missingWords,
		OccurredAt: time.Now(),
	}

	ack, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{event(1, "connection.established"), low, high, invalid})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	refused := ack.Outcomes[3]
	if refused.Status != "refused" || refused.Reason != "TRANSCRIPT_INVALID" {
		t.Fatalf("a final segment without word timing = %+v", refused)
	}

	transcript, err := events.AssembleTranscript(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	confidences := map[int]float64{}
	for _, segment := range transcript.Segments {
		if segment.Type == "transcript.segment.final" {
			confidences[segment.Sequence] = segment.Confidence
		}
	}
	if confidences[2] != 0.31 || confidences[3] != 0.99 {
		t.Fatalf("per-segment confidence = %v", confidences)
	}
}

func TestAnOrphanCorrectionIsListedNotHidden(t *testing.T) {
	ctx := context.Background()
	events := interview.NewEvents(interview.NewStore(pool))
	session := startedPractice(t)

	if _, err := events.Ingest(ctx, session.ID, "practice", candidateID, "", 1,
		[]interview.ControlEvent{
			event(1, "connection.established"),
			// A correction whose target (sequence 7) never arrived: kept
			// and flagged, because dropping it would hide a resend the
			// client still owes.
			correction(3, 7, "orphaned fix", 0.9),
		}); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	transcript, err := events.AssembleTranscript(ctx, session.ID, "practice", candidateID, "")
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if len(transcript.OrphanCorrections) != 1 || transcript.OrphanCorrections[0] != 3 {
		t.Fatalf("orphans = %v", transcript.OrphanCorrections)
	}
	_ = fmt.Sprintf
}
