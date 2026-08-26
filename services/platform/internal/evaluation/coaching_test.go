package evaluation_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
)

// coaching-1: PRC-02's deterministic floor. Every statement carries the
// exact quote it is about, the rewrite is assembled ONLY from the
// candidate's own sentences plus visibly bracketed questions, and a turn
// with nothing useful to say gets nothing - never filler.

func coachingInput() evaluation.SealedInput {
	return evaluation.SealedInput{
		SessionID: "ses-1",
		Competencies: []evaluation.Competency{
			{ID: "systems-design", Name: "Systems design"},
		},
		Turns: []evaluation.Turn{
			{
				Sequence: 3, Speaker: "candidate",
				Text:    "I redesigned the checkout systems design. Latency dropped 40 percent.",
				StartMs: 5000, EndMs: 20000,
			},
			{
				Sequence: 5, Speaker: "candidate",
				Text:    "I am usually good at systems design tradeoffs.",
				StartMs: 30000, EndMs: 40000,
			},
		},
	}
}

func coachingSpans() []evaluation.StoredSpan {
	return []evaluation.StoredSpan{
		{ID: "sp-1", Span: evaluation.Span{
			CompetencyID: "systems-design", Kind: "supporting", SegmentSequence: 3,
			Quote:     "I redesigned the checkout systems design. Latency dropped 40 percent.",
			CharStart: 0, CharEnd: 70, StartMs: 5000, EndMs: 20000,
			ExtractionVersion: "evidence-1",
		}},
		{ID: "sp-2", Span: evaluation.Span{
			CompetencyID: "systems-design", Kind: "claim_unverified", SegmentSequence: 5,
			Quote:     "I am usually good at systems design tradeoffs.",
			CharStart: 0, CharEnd: 46, StartMs: 30000, EndMs: 40000,
			ExtractionVersion: "evidence-1",
		}},
	}
}

func TestEveryCoachingStatementCarriesItsQuote(t *testing.T) {
	review := evaluation.Coach(coachingInput(), coachingSpans())

	if len(review.Answers) != 2 {
		t.Fatalf("answers = %d, want one per evidenced candidate turn", len(review.Answers))
	}
	for _, answer := range review.Answers {
		for _, point := range append(answer.Strengths, answer.Gaps...) {
			if point.Quote == "" {
				t.Fatalf("a coaching statement with no quote: %+v", point)
			}
			turn := turnBySequence(t, coachingInput(), answer.Sequence)
			if !strings.Contains(turn.Text, point.Quote) {
				t.Fatalf("the quote is not in the turn: %q", point.Quote)
			}
		}
	}
}

func TestTheRewriteNeverAddsAFact(t *testing.T) {
	review := evaluation.Coach(coachingInput(), coachingSpans())

	claim := answerBySequence(t, review, 5)
	if len(claim.Rewrite) == 0 {
		t.Fatal("an unverified claim earns a rewrite: the claim plus the question that would ground it")
	}
	turn := turnBySequence(t, coachingInput(), 5)
	sawPlaceholder := false
	for _, part := range claim.Rewrite {
		switch part.Kind {
		case "quote":
			if !strings.Contains(turn.Text, part.Text) {
				t.Fatalf("the rewrite contains words the candidate never said: %q", part.Text)
			}
		case "placeholder":
			sawPlaceholder = true
			if !strings.HasPrefix(part.Text, "[") || !strings.HasSuffix(part.Text, "]") {
				t.Fatalf("a placeholder must be visibly bracketed: %q", part.Text)
			}
			if !strings.Contains(part.Text, "?") {
				t.Fatalf("missing information is a question, never an invented fact: %q", part.Text)
			}
		default:
			t.Fatalf("unknown rewrite part kind %q", part.Kind)
		}
	}
	if !sawPlaceholder {
		t.Fatal("the missing outcome must appear as a placeholder question")
	}
}

func TestAStrongAnswerGetsNoFillerRewrite(t *testing.T) {
	review := evaluation.Coach(coachingInput(), coachingSpans())

	strong := answerBySequence(t, review, 3)
	if len(strong.Rewrite) != 0 {
		t.Fatalf("a supported answer with no gaps got a rewrite: %+v", strong.Rewrite)
	}
	if len(strong.Strengths) == 0 {
		t.Fatal("the supported answer's strength went unstated")
	}
}

func TestCoachingIsDeterministic(t *testing.T) {
	first := evaluation.Coach(coachingInput(), coachingSpans())
	second := evaluation.Coach(coachingInput(), coachingSpans())
	if !reflect.DeepEqual(first, second) {
		t.Fatal("two runs over the same session disagree")
	}
}

func TestTheCoachingGateRefusesInventedWords(t *testing.T) {
	// The gate is what outlives coaching-1: when a model writes the
	// rewrite, this is what still holds it to the candidate's own words.
	input := coachingInput()
	honest := evaluation.Coach(input, coachingSpans())
	if err := evaluation.ValidateCoaching(input, honest); err != nil {
		t.Fatalf("honest coaching refused: %v", err)
	}

	cases := map[string]func(review *evaluation.Review){
		"a rewrite sentence the candidate never said": func(review *evaluation.Review) {
			review.Answers[0].Rewrite = []evaluation.RewritePart{
				{Kind: "quote", Text: "I single-handedly saved the company."},
			}
		},
		"a fact smuggled into a placeholder": func(review *evaluation.Review) {
			review.Answers[1].Rewrite = append(review.Answers[1].Rewrite,
				evaluation.RewritePart{Kind: "placeholder", Text: "[The project saved 2 million dollars]"})
		},
		"a statement quoting words not in its turn": func(review *evaluation.Review) {
			review.Answers[0].Strengths[0].Quote = "something else entirely"
		},
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			tampered := evaluation.Coach(input, coachingSpans())
			corrupt(&tampered)
			if err := evaluation.ValidateCoaching(input, tampered); !errors.Is(err, evaluation.ErrCoachingUnpreserving) {
				t.Fatalf("%s = %v, want ErrCoachingUnpreserving", name, err)
			}
		})
	}
}

func turnBySequence(t *testing.T, input evaluation.SealedInput, sequence int) evaluation.Turn {
	t.Helper()
	for _, turn := range input.Turns {
		if turn.Sequence == sequence {
			return turn
		}
	}
	t.Fatalf("no turn %d", sequence)
	return evaluation.Turn{}
}

func answerBySequence(t *testing.T, review evaluation.Review, sequence int) evaluation.AnswerCoaching {
	t.Helper()
	for _, answer := range review.Answers {
		if answer.Sequence == sequence {
			return answer
		}
	}
	t.Fatalf("no answer for turn %d", sequence)
	return evaluation.AnswerCoaching{}
}
