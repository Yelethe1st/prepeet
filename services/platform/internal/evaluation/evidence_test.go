package evaluation_test

import (
	"errors"
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
