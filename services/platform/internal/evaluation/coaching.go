package evaluation

// Coaching: PRC-02's deterministic floor, coaching-1.
//
// Coaching is derived, never stored: a pure function of the sealed input
// and the stored evidence, so it re-derives identically forever and a
// better coach replaces it behind the same shape. The rules are the
// spec's, made structural: every statement carries the exact quote it is
// about, and a rewrite is assembled ONLY from the candidate's own
// sentences plus visibly bracketed questions - the floor cannot invent a
// fact because it has no vocabulary to invent one with. ValidateCoaching
// is the gate that outlives the floor: when a model writes the rewrite,
// the same arithmetic holds it to the candidate's words.

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// CoachingVersion names this derivation on every review.
const CoachingVersion = "coaching-1"

// Review is one session's coaching, answer by answer.
type Review struct {
	SessionID       string           `json:"session_id"`
	CoachingVersion string           `json:"coaching_version"`
	Answers         []AnswerCoaching `json:"answers"`
}

// AnswerCoaching is one evidenced candidate turn's coaching.
type AnswerCoaching struct {
	Sequence  int             `json:"sequence"`
	Strengths []CoachingPoint `json:"strengths"`
	Gaps      []CoachingPoint `json:"gaps"`
	// Rewrite is empty when there is nothing useful to change: a strong
	// answer earns silence, never filler.
	Rewrite []RewritePart `json:"rewrite"`
}

// CoachingPoint is one statement about one exact quote.
type CoachingPoint struct {
	Statement string `json:"statement"`
	Quote     string `json:"quote"`
}

// RewritePart is one piece of a suggested answer: the candidate's own
// words, or a bracketed question standing where information is missing.
type RewritePart struct {
	Kind string `json:"kind"` // quote | placeholder
	Text string `json:"text"`
}

// The floor's placeholder questions. Questions on purpose: missing
// information is something to ask, never something to invent.
const (
	askOutcome   = "[What changed as a result? Add the number, the time saved, or what the outcome was.]"
	askGrounding = "[Which project or moment shows this? Name it, and what happened.]"
)

// Coach derives coaching-1 for every candidate turn that produced
// evidence. Turns without evidence get nothing: coaching that traces to
// nothing said would be the filler this feature exists to refuse.
func Coach(input SealedInput, spans []StoredSpan) Review {
	bySequence := map[int][]StoredSpan{}
	for _, span := range spans {
		bySequence[span.SegmentSequence] = append(bySequence[span.SegmentSequence], span)
	}
	turns := map[int]Turn{}
	for _, turn := range input.Turns {
		turns[turn.Sequence] = turn
	}

	sequences := make([]int, 0, len(bySequence))
	for sequence := range bySequence {
		sequences = append(sequences, sequence)
	}
	sort.Ints(sequences)

	answers := make([]AnswerCoaching, 0, len(sequences))
	for _, sequence := range sequences {
		turn, present := turns[sequence]
		if !present || turn.Speaker != "candidate" {
			continue
		}
		answer := AnswerCoaching{
			Sequence:  sequence,
			Strengths: []CoachingPoint{},
			Gaps:      []CoachingPoint{},
			Rewrite:   []RewritePart{},
		}
		needsOutcome, needsGrounding := false, false
		for _, span := range bySequence[sequence] {
			switch span.Kind {
			case "supporting":
				answer.Strengths = append(answer.Strengths, CoachingPoint{
					Statement: "You backed this with a concrete, measured outcome. Keep leading with it.",
					Quote:     span.Quote,
				})
			case "claim_unverified":
				answer.Gaps = append(answer.Gaps, CoachingPoint{
					Statement: "This is a claim about yourself with nothing a listener could check. Ground it in one specific moment.",
					Quote:     span.Quote,
				})
				answer.Rewrite = append(answer.Rewrite, RewritePart{Kind: "quote", Text: span.Quote})
				needsGrounding = true
			case "gap":
				answer.Gaps = append(answer.Gaps, CoachingPoint{
					Statement: "You named the limit of your experience honestly. Decide in advance what your nearest relevant example is.",
					Quote:     span.Quote,
				})
			case "contradictory":
				answer.Gaps = append(answer.Gaps, CoachingPoint{
					Statement: "This conflicts with something you said elsewhere. Settle which version is precise before the next interview.",
					Quote:     span.Quote,
				})
			}
		}
		// An answer that made claims without a single measured outcome is
		// missing its ending; the rewrite asks for it rather than writing
		// one.
		hasSupport := len(answer.Strengths) > 0
		if needsGrounding && !hasSupport {
			needsOutcome = true
		}
		if needsGrounding {
			answer.Rewrite = append(answer.Rewrite, RewritePart{Kind: "placeholder", Text: askGrounding})
		}
		if needsOutcome {
			answer.Rewrite = append(answer.Rewrite, RewritePart{Kind: "placeholder", Text: askOutcome})
		}
		answers = append(answers, answer)
	}
	return Review{SessionID: input.SessionID, CoachingVersion: CoachingVersion, Answers: answers}
}

// ErrCoachingUnpreserving refuses coaching that does not trace to the
// candidate's own words.
var ErrCoachingUnpreserving = errors.New(
	"evaluation: COACHING_UNPRESERVING: a coaching statement does not trace to what was said")

// ValidateCoaching holds a review to the sealed input: every statement's
// quote and every rewrite quote-part must be an exact substring of its own
// turn, and a placeholder must be a bracketed question with no digits - a
// number inside a placeholder is a fact wearing brackets. One violation
// refuses the whole review.
func ValidateCoaching(input SealedInput, review Review) error {
	turns := map[int]Turn{}
	for _, turn := range input.Turns {
		turns[turn.Sequence] = turn
	}
	for _, answer := range review.Answers {
		turn, present := turns[answer.Sequence]
		if !present || turn.Speaker != "candidate" {
			return fmt.Errorf("%w: answer %d coaches a turn that is not the candidate's", ErrCoachingUnpreserving, answer.Sequence)
		}
		for _, point := range append(append([]CoachingPoint{}, answer.Strengths...), answer.Gaps...) {
			if point.Quote == "" || !strings.Contains(turn.Text, point.Quote) {
				return fmt.Errorf("%w: answer %d quotes words not in its turn", ErrCoachingUnpreserving, answer.Sequence)
			}
		}
		for _, part := range answer.Rewrite {
			switch part.Kind {
			case "quote":
				if !strings.Contains(turn.Text, part.Text) {
					return fmt.Errorf("%w: answer %d's rewrite contains words the candidate never said", ErrCoachingUnpreserving, answer.Sequence)
				}
			case "placeholder":
				if !strings.HasPrefix(part.Text, "[") || !strings.HasSuffix(part.Text, "]") ||
					!strings.Contains(part.Text, "?") || strings.ContainsAny(part.Text, "0123456789") {
					return fmt.Errorf("%w: answer %d's placeholder is not a bracketed question", ErrCoachingUnpreserving, answer.Sequence)
				}
			default:
				return fmt.Errorf("%w: answer %d has a rewrite part of unknown kind %q", ErrCoachingUnpreserving, answer.Sequence, part.Kind)
			}
		}
	}
	return nil
}
