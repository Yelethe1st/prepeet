package progression

import "time"

// Evidence freshness: PRG-04's requirement, computed once here because
// three different readers need the same answer.
//
// An observation from six months ago and one from last week are not the
// same claim, and a screen that renders both as "solid" has quietly
// promoted a memory to a measurement. Freshness is therefore part of every
// reading this context hands out rather than something a caller may
// remember to work out, and it is a named standing rather than a bare age
// so that the boundary between "current" and "worth revisiting" is one
// decision recorded in one place.

// How current a reading is.
//
// None is separate from Stale on purpose, and for the reason that recurs
// through this context: never measured and measured long ago are different
// facts, and the first must not be rendered as an extreme of the second.
const (
	EvidenceFresh = "fresh"
	EvidenceAging = "aging"
	EvidenceStale = "stale"
	EvidenceNone  = "none"
)

// The boundaries, in days.
//
// Thirty days is roughly a practice cycle: inside it a reading still
// describes how somebody currently answers. Ninety is the point past which
// a candidate practising at all has almost certainly changed, so the
// reading is reported as stale rather than counted as current. Both are
// deliberately coarse. A finer scale would imply a precision the evidence
// does not have.
const (
	freshDays = 30
	staleDays = 90
)

// Evidence is how old a reading is and what that makes it.
type Evidence struct {
	Standing   string
	AgeDays    int
	ObservedAt time.Time
}

// Freshness answers how current one reading is at a given moment.
//
// A zero observedAt means there is no reading, which answers None with no
// age, because reporting an age for evidence that does not exist is how a
// screen ends up showing "last measured: 1 January year one". Evidence
// dated ahead of now is clamped to zero days rather than reported as
// negative: clock skew between the worker that observed and the reader
// that asks is not a fact about the candidate.
func Freshness(observedAt, at time.Time) Evidence {
	if observedAt.IsZero() {
		return Evidence{Standing: EvidenceNone}
	}
	days := int(at.UTC().Sub(observedAt.UTC()).Hours() / 24)
	if days < 0 {
		days = 0
	}
	evidence := Evidence{AgeDays: days, ObservedAt: observedAt}
	switch {
	case days < freshDays:
		evidence.Standing = EvidenceFresh
	case days < staleDays:
		evidence.Standing = EvidenceAging
	default:
		evidence.Standing = EvidenceStale
	}
	return evidence
}
