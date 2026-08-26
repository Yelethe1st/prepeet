package interview

// Active-time accounting and the timing policy: SES-05.
//
// Duration counts active interview time. The durable timeline already
// separates connection epochs, so the accounting is arithmetic over what
// is stored: per epoch, the span from its first conversational segment to
// its last; in total, the sum. The room-clock gap between epochs is a
// reconnection, and reconnecting is the platform's problem, never the
// candidate's - it can never appear in the sum by construction.

// ActiveSeconds sums each epoch's conversational span. Segment order does
// not matter: per-epoch bounds are min start and max end.
func ActiveSeconds(segments []TranscriptSegment) int {
	type bounds struct{ first, last int }
	epochs := map[int]bounds{}
	for _, segment := range segments {
		b, seen := epochs[segment.Epoch]
		if !seen || segment.StartMs < b.first {
			b.first = segment.StartMs
		}
		if segment.EndMs > b.last {
			b.last = segment.EndMs
		}
		epochs[segment.Epoch] = b
	}
	total := 0
	for _, b := range epochs {
		if b.last > b.first {
			total += b.last - b.first
		}
	}
	return total / 1000
}

// TimingPolicy is one versioned set of timing rules. Values live in the
// database and reach the client through the start response, so no grace
// window or ceiling is ever a constant compiled into a UI.
type TimingPolicy struct {
	Version               int
	ReconnectGraceSeconds int
	MaxOverrunSeconds     int
}
