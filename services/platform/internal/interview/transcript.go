package interview

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
)

// Transcript capture, correction and provenance: RTC-04.
//
// The control event log already retains every final and corrected segment
// with its epoch, sequence and timing; nothing is ever edited. What this
// file adds is the shape those payloads must have to be worth retaining -
// word-level timing on the room's clock (ADR-0013's single timebase),
// confidence per segment, an explicit supersession reference on every
// correction - and the read model that assembles them: corrections replace
// a segment's effective text while the original stays underneath with its
// own sequence and timing, so evidence always traces to what was actually
// said, and to what it was corrected to, and to which came first.

// TranscriptWord is one word on the room clock.
type TranscriptWord struct {
	Word       string  `json:"w"`
	StartMs    int     `json:"start_ms"`
	EndMs      int     `json:"end_ms"`
	Confidence float64 `json:"confidence"`
}

// transcriptPayload is the wire shape of a transcript segment event.
type transcriptPayload struct {
	Speaker    string           `json:"speaker"`
	Text       string           `json:"text"`
	StartMs    int              `json:"start_ms"`
	EndMs      int              `json:"end_ms"`
	Confidence float64          `json:"confidence"`
	Words      []TranscriptWord `json:"words"`
	// SupersedesSequence names the final segment a correction replaces.
	// Required on corrections, forbidden on finals.
	SupersedesSequence int `json:"supersedes_sequence,omitempty"`
}

// validateTranscriptPayload refuses a segment that could not serve as
// evidence. Ingest calls this so a malformed segment never enters the
// timeline at all: a transcript row without timing or confidence is not a
// lesser record, it is a record that cannot answer the questions the
// product exists to answer.
func validateTranscriptPayload(eventType string, raw json.RawMessage) error {
	var payload transcriptPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("undecodable payload: %w", err)
	}
	if payload.Speaker != "candidate" && payload.Speaker != "interviewer" {
		return fmt.Errorf("speaker %q is neither candidate nor interviewer", payload.Speaker)
	}
	if payload.Text == "" {
		return fmt.Errorf("a segment with no text records nothing")
	}
	if payload.StartMs < 0 || payload.EndMs <= payload.StartMs {
		return fmt.Errorf("timing [%d,%d) is not a span on the room clock", payload.StartMs, payload.EndMs)
	}
	if payload.Confidence < 0 || payload.Confidence > 1 {
		return fmt.Errorf("confidence %v is outside [0,1]", payload.Confidence)
	}

	switch eventType {
	case "transcript.segment.final":
		if payload.SupersedesSequence != 0 {
			return fmt.Errorf("a final segment supersedes nothing; corrections do")
		}
		// Word timing is what ART-01 aligns on; a final segment without it
		// would make delivery measurement silently impossible later.
		if len(payload.Words) == 0 {
			return fmt.Errorf("a final segment needs word-level timing")
		}
		for i, word := range payload.Words {
			if word.Word == "" || word.StartMs < payload.StartMs || word.EndMs > payload.EndMs || word.EndMs <= word.StartMs {
				return fmt.Errorf("word %d does not sit inside its segment on the room clock", i)
			}
			if word.Confidence < 0 || word.Confidence > 1 {
				return fmt.Errorf("word %d confidence %v is outside [0,1]", i, word.Confidence)
			}
		}
	case "transcript.segment.corrected":
		// A correction may be textual (a human or model fixing words), so
		// word timing is optional; the original's timing remains the
		// alignment source. What is not optional is saying what it corrects.
		if payload.SupersedesSequence < 1 {
			return fmt.Errorf("a correction must name the sequence it supersedes")
		}
	}
	return nil
}

// TranscriptSegment is one entry of the assembled transcript, provenance
// included.
type TranscriptSegment struct {
	Epoch      int
	Sequence   int
	Type       string
	Speaker    string
	Text       string
	StartMs    int
	EndMs      int
	Confidence float64
	Words      []TranscriptWord
	// Superseded marks text a correction replaced. The row stays: evidence
	// traces to what was actually said first.
	Superseded bool
	// CorrectedBySequence links a superseded segment to the correction
	// that replaced it; Supersedes links a correction back.
	CorrectedBySequence int
	Supersedes          int
}

// Transcript is the assembled read model.
type Transcript struct {
	Segments []TranscriptSegment
	// OrphanCorrections are corrections whose target has not arrived.
	// Listed rather than hidden: a dangling supersession is a gap the
	// client should resolve by resending, not a row to quietly drop.
	OrphanCorrections []int
}

// EffectiveText answers the corrected view in timeline order: what
// downstream consumers quote. Superseded originals and the corrections
// that replaced earlier corrections are excluded here and retained in
// Segments.
func (t Transcript) EffectiveText() []TranscriptSegment {
	effective := make([]TranscriptSegment, 0, len(t.Segments))
	for _, segment := range t.Segments {
		if !segment.Superseded {
			effective = append(effective, segment)
		}
	}
	return effective
}

// AssembleTranscript builds the read model for one session by replaying
// the durable timeline. Deterministic by construction: replay order is the
// timeline's, and the latest correction of a segment wins with every
// earlier version retained and linked.
func (e *Events) AssembleTranscript(ctx context.Context, sessionID, mode, candidateID, tenantID string) (Transcript, error) {
	events, err := e.Replay(ctx, sessionID, mode, candidateID, tenantID, 0, 0)
	if err != nil {
		return Transcript{}, err
	}

	var transcript Transcript
	// bySequence keyed by (epoch, sequence) of ORIGINAL final segments;
	// corrections chain onto their target.
	type key struct{ epoch, sequence int }
	index := map[key]int{}

	for _, event := range events {
		if event.Type != "transcript.segment.final" && event.Type != "transcript.segment.corrected" {
			continue
		}
		var payload transcriptPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			// Ingest validates, so this is corruption; refuse loudly rather
			// than serving a partial transcript as whole.
			return Transcript{}, fmt.Errorf("interview: stored segment %s does not decode: %w", event.EventID, err)
		}

		segment := TranscriptSegment{
			Epoch: event.Epoch, Sequence: event.Sequence, Type: event.Type,
			Speaker: payload.Speaker, Text: payload.Text,
			StartMs: payload.StartMs, EndMs: payload.EndMs,
			Confidence: payload.Confidence, Words: payload.Words,
			Supersedes: payload.SupersedesSequence,
		}

		if event.Type == "transcript.segment.corrected" {
			target, present := index[key{event.Epoch, payload.SupersedesSequence}]
			if !present {
				transcript.OrphanCorrections = append(transcript.OrphanCorrections, event.Sequence)
				transcript.Segments = append(transcript.Segments, segment)
				continue
			}
			// The latest correction wins; every earlier version - the
			// original and any prior correction - is retained, superseded,
			// and linked forward.
			transcript.Segments[target].Superseded = true
			transcript.Segments[target].CorrectedBySequence = event.Sequence
			current := transcript.Segments[target].CorrectedBySequence
			for i := range transcript.Segments {
				if transcript.Segments[i].Type == "transcript.segment.corrected" &&
					transcript.Segments[i].Supersedes == payload.SupersedesSequence &&
					transcript.Segments[i].Epoch == event.Epoch &&
					transcript.Segments[i].Sequence != current {
					transcript.Segments[i].Superseded = true
					transcript.Segments[i].CorrectedBySequence = event.Sequence
				}
			}
		} else {
			index[key{event.Epoch, event.Sequence}] = len(transcript.Segments)
		}
		transcript.Segments = append(transcript.Segments, segment)
	}

	sort.SliceStable(transcript.Segments, func(i, j int) bool {
		if transcript.Segments[i].Epoch != transcript.Segments[j].Epoch {
			return transcript.Segments[i].Epoch < transcript.Segments[j].Epoch
		}
		return transcript.Segments[i].Sequence < transcript.Segments[j].Sequence
	})
	return transcript, nil
}
