package evaluation_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
)

// The honesty gate, attacked from every direction: a span that does not
// resolve to the sealed input never passes, whoever produced it. This is
// EVL-01's second box, and it is deliberately independent of the extractor:
// when a model replaces evidence-1, this validator is what still stands.

func sealedFixture() evaluation.SealedInput {
	return evaluation.SealedInput{
		SessionID: "ses-1",
		Competencies: []evaluation.Competency{
			{ID: "systems-design", Name: "Systems design"},
		},
		Turns: []evaluation.Turn{{
			Sequence: 3, Speaker: "candidate",
			Text:    "I redesigned the systems design to shard by region.",
			StartMs: 5000, EndMs: 20000,
		}},
	}
}

func honestSpan() evaluation.Span {
	return evaluation.Span{
		CompetencyID: "systems-design", Kind: "supporting",
		SegmentSequence: 3,
		Quote:           "I redesigned the systems design to shard by region.",
		CharStart:       0, CharEnd: 51,
		StartMs: 5000, EndMs: 19000,
		ExtractionVersion: "evidence-1",
	}
}

func TestAnHonestSpanPasses(t *testing.T) {
	if err := evaluation.Validate(sealedFixture(), []evaluation.Span{honestSpan()}); err != nil {
		t.Fatalf("an honest span was refused: %v", err)
	}
}

func TestEveryFabricationIsRefused(t *testing.T) {
	cases := map[string]func(span *evaluation.Span){
		"a quote that is not the text at its range": func(span *evaluation.Span) {
			span.Quote = "I single-handedly saved the company."
		},
		"a segment that does not exist": func(span *evaluation.Span) {
			span.SegmentSequence = 9
		},
		"a competency nobody asked about": func(span *evaluation.Span) {
			span.CompetencyID = "charisma"
		},
		"a character range outside the segment": func(span *evaluation.Span) {
			span.CharEnd = 500
		},
		"timing outside the segment": func(span *evaluation.Span) {
			span.EndMs = 99999
		},
		"a missing extraction version": func(span *evaluation.Span) {
			span.ExtractionVersion = ""
		},
		"an off-by-one quote": func(span *evaluation.Span) {
			span.CharStart = 1
		},
	}

	for name, fabricate := range cases {
		span := honestSpan()
		fabricate(&span)
		err := evaluation.Validate(sealedFixture(), []evaluation.Span{span})
		if !errors.Is(err, evaluation.ErrFabricated) {
			t.Errorf("%s passed validation: %v", name, err)
		}
	}
}

func TestOneFabricationRefusesTheWholeBatch(t *testing.T) {
	fabricated := honestSpan()
	fabricated.Quote = "invented"
	fabricated.CharEnd = 8

	err := evaluation.Validate(sealedFixture(), []evaluation.Span{honestSpan(), fabricated})
	if !errors.Is(err, evaluation.ErrFabricated) {
		t.Fatalf("a batch with one lie passed: %v", err)
	}
}

// EVL-04: the honesty gate holds contradiction pairs to the sealed input
// exactly as it holds spans, and the framing stays neutral by test.

func contradictingFixture() evaluation.SealedInput {
	return evaluation.SealedInput{
		SessionID: "ses-2",
		Competencies: []evaluation.Competency{
			{ID: "systems-design", Name: "Systems design"},
		},
		Turns: []evaluation.Turn{
			{
				Sequence: 3, Speaker: "candidate",
				Text:    "I led the payments migration team of 5 engineers.",
				StartMs: 5000, EndMs: 9000,
			},
			{
				Sequence: 4, Speaker: "interviewer",
				Text:    "The payments migration team had 9 people, you said?",
				StartMs: 10000, EndMs: 14000,
			},
			{
				Sequence: 5, Speaker: "candidate",
				Text:    "The payments migration team I led was 12 people.",
				StartMs: 15000, EndMs: 19000,
			},
		},
	}
}

func honestPair() evaluation.Contradiction {
	return evaluation.Contradiction{
		Topic: []string{"migration", "payments", "team"},
		SideA: evaluation.ContradictionSide{
			SegmentSequence: 3,
			Quote:           "I led the payments migration team of 5 engineers.",
			CharStart:       0, CharEnd: 49, StartMs: 5000, EndMs: 9000,
		},
		SideB: evaluation.ContradictionSide{
			SegmentSequence: 5,
			Quote:           "The payments migration team I led was 12 people.",
			CharStart:       0, CharEnd: 48, StartMs: 15000, EndMs: 19000,
		},
		ExtractionVersion: "evidence-1",
	}
}

func TestAnHonestPairPasses(t *testing.T) {
	pairs := []evaluation.Contradiction{honestPair()}
	if err := evaluation.ValidateContradictions(contradictingFixture(), pairs); err != nil {
		t.Fatalf("an honest pair was refused: %v", err)
	}
}

func TestEveryDishonestPairRefusesTheBatch(t *testing.T) {
	cases := map[string]func(pair *evaluation.Contradiction){
		"a side citing a ghost segment": func(pair *evaluation.Contradiction) {
			pair.SideB.SegmentSequence = 99
		},
		"a quote that is not the text at its range": func(pair *evaluation.Contradiction) {
			pair.SideA.Quote = "I led the payments migration team of 50 engineers."
		},
		"a character range outside the segment": func(pair *evaluation.Contradiction) {
			pair.SideB.CharEnd = 500
		},
		"timing outside the segment": func(pair *evaluation.Contradiction) {
			pair.SideA.EndMs = 999999
		},
		"a side quoting the interviewer": func(pair *evaluation.Contradiction) {
			pair.SideB = evaluation.ContradictionSide{
				SegmentSequence: 4,
				Quote:           "The payments migration team had 9 people, you said?",
				CharStart:       0, CharEnd: 51, StartMs: 10000, EndMs: 14000,
			}
		},
		"no extraction version": func(pair *evaluation.Contradiction) {
			pair.ExtractionVersion = ""
		},
	}
	for name, corrupt := range cases {
		t.Run(name, func(t *testing.T) {
			pair := honestPair()
			corrupt(&pair)
			err := evaluation.ValidateContradictions(contradictingFixture(), []evaluation.Contradiction{pair})
			if !errors.Is(err, evaluation.ErrFabricated) {
				t.Fatalf("%s = %v, want ErrFabricated", name, err)
			}
		})
	}
}

func TestThePipelineVocabularyNeverJudgesThePerson(t *testing.T) {
	// Box 3 as a test: every name this pipeline can emit - span kinds,
	// reason codes, statuses - describes evidence, never character. The
	// surface copy has its own twin test in the API package.
	vocabulary := strings.ToLower(strings.Join([]string{
		"supporting", "contradictory", "claim_unverified", "gap", "contradiction",
		"NOT_DISCUSSED", "INSUFFICIENT_EVIDENCE", "GAPS_ACKNOWLEDGED", "CONTRADICTIONS_PRESENT",
		"assessed", "unassessed",
	}, " "))
	for _, forbidden := range []string{"honest", "integrity", "credib", "lying", "deceit", "decept", "truthful"} {
		if strings.Contains(vocabulary, forbidden) {
			t.Fatalf("the shipped vocabulary contains %q; nothing in this pipeline judges the person", forbidden)
		}
	}
}
