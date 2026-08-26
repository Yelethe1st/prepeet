package interview_test

import (
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
)

// SES-05's first box as arithmetic: duration counts active interview time
// per connection epoch, so the room-clock gap a reconnection leaves
// between epochs never counts against the candidate.

func segment(epoch, startMs, endMs int) interview.TranscriptSegment {
	return interview.TranscriptSegment{
		Epoch: epoch, Type: "transcript.segment.final", Speaker: "candidate",
		Text: "spoken", StartMs: startMs, EndMs: endMs,
	}
}

func TestActiveSecondsSumsWithinEpochs(t *testing.T) {
	segments := []interview.TranscriptSegment{
		segment(1, 5_000, 65_000),
		segment(1, 70_000, 125_000),
	}
	if got := interview.ActiveSeconds(segments); got != 120 {
		t.Fatalf("one epoch spanning 5s to 125s = %ds active, want 120", got)
	}
}

func TestReconnectionGapsDoNotConsumeCandidateTime(t *testing.T) {
	// Epoch 1 covers two minutes, then the connection dies. The room
	// clock keeps ticking for ten minutes of reconnecting before epoch 2
	// covers one more minute. Active time is three minutes, not thirteen.
	segments := []interview.TranscriptSegment{
		segment(1, 0, 120_000),
		segment(2, 720_000, 780_000),
	}
	if got := interview.ActiveSeconds(segments); got != 180 {
		t.Fatalf("active seconds = %d, want 180: the reconnection gap was billed to the candidate", got)
	}
}

func TestActiveSecondsOfNothingIsZero(t *testing.T) {
	if got := interview.ActiveSeconds(nil); got != 0 {
		t.Fatalf("no segments = %d", got)
	}
}

func TestEpochOrderDoesNotMatter(t *testing.T) {
	// Replay order is the timeline's, but the arithmetic must not depend
	// on it: per-epoch bounds are min and max, wherever they appear.
	shuffled := []interview.TranscriptSegment{
		segment(2, 720_000, 780_000),
		segment(1, 60_000, 120_000),
		segment(1, 0, 50_000),
	}
	if got := interview.ActiveSeconds(shuffled); got != 180 {
		t.Fatalf("active seconds = %d, want 180", got)
	}
}
