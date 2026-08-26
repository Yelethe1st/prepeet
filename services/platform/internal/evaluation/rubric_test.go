package evaluation_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
)

// aggregate-1's rules, pinned: sufficiency before scoring, unassessed kept
// apart from poor, bands earned by data-driven thresholds, and the whole
// calculation deterministic - which is what the pinned-version guarantee
// rests on.

func rubricFixture(t *testing.T) evaluation.Rubric {
	t.Helper()
	rubric, err := evaluation.ParseRubric([]byte(`{
		"sufficiency": {"min_supporting": 2},
		"bands": [
			{"id": "developing", "min_ratio": 0.0},
			{"id": "solid", "min_ratio": 0.55},
			{"id": "strong", "min_ratio": 0.8}
		],
		"confidence": {
			"high": {"min_supporting": 4, "max_contradictory": 0},
			"medium": {"min_supporting": 2, "max_contradictory": 1}
		}}`))
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	return rubric
}

func evidence(competency, kind string, n int) []evaluation.StoredSpan {
	spans := make([]evaluation.StoredSpan, 0, n)
	for i := 0; i < n; i++ {
		spans = append(spans, evaluation.StoredSpan{
			ID: fmt.Sprintf("sp-%s-%s-%d", competency, kind, i),
			Span: evaluation.Span{
				CompetencyID: competency, Kind: kind, SegmentSequence: i + 2,
				Quote: "q", CharStart: 0, CharEnd: 1, StartMs: 1, EndMs: 2,
				ExtractionVersion: "evidence-1",
			},
		})
	}
	return spans
}

func TestSufficiencyComesBeforeAnyScore(t *testing.T) {
	competencies := []evaluation.Competency{{ID: "debugging", Name: "Debugging"}}
	// One supporting span: real evidence, below the threshold of two.
	result := evaluation.Aggregate(rubricFixture(t), competencies,
		evidence("debugging", "supporting", 1))

	competency := result.Competencies[0]
	if competency.Status != "unassessed" || competency.Band != "" {
		t.Fatalf("below sufficiency = %+v; a band was assigned to silence", competency)
	}
	found := false
	for _, code := range competency.ReasonCodes {
		if code == "INSUFFICIENT_EVIDENCE" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reasons = %v", competency.ReasonCodes)
	}
}

func TestUnassessedIsNotPoor(t *testing.T) {
	// The rule in one comparison: no evidence at all versus weak evidence
	// above threshold land in DIFFERENT fields, not different values of one.
	competencies := []evaluation.Competency{
		{ID: "silent", Name: "Silent"},
		{ID: "weak", Name: "Weak"},
	}
	spans := append(evidence("weak", "supporting", 2), evidence("weak", "contradictory", 2)...)

	result := evaluation.Aggregate(rubricFixture(t), competencies, spans)
	byID := map[string]evaluation.CompetencyResult{}
	for _, competency := range result.Competencies {
		byID[competency.CompetencyID] = competency
	}

	if byID["silent"].Status != "unassessed" || byID["silent"].Band != "" {
		t.Fatalf("silent = %+v", byID["silent"])
	}
	if byID["weak"].Status != "assessed" || byID["weak"].Band != "developing" {
		t.Fatalf("weak = %+v; weak evidence is a low band, never unassessed", byID["weak"])
	}
	if result.CoveredCompetencies != 1 || result.TotalCompetencies != 2 {
		t.Fatalf("coverage = %d/%d", result.CoveredCompetencies, result.TotalCompetencies)
	}
}

func TestBandsAreEarnedByTheRubricsOwnThresholds(t *testing.T) {
	competencies := []evaluation.Competency{{ID: "sd", Name: "Systems design"}}
	cases := []struct {
		supporting, contradictory int
		want                      string
	}{
		{2, 3, "developing"}, // ratio 0.4
		{3, 2, "solid"},      // ratio 0.6
		{4, 1, "strong"},     // ratio 0.8
		{5, 0, "strong"},     // ratio 1.0
	}
	for _, test := range cases {
		spans := append(evidence("sd", "supporting", test.supporting),
			evidence("sd", "contradictory", test.contradictory)...)
		result := evaluation.Aggregate(rubricFixture(t), competencies, spans)
		if got := result.Competencies[0].Band; got != test.want {
			t.Errorf("%d/%d supporting = band %q, want %q",
				test.supporting, test.supporting+test.contradictory, got, test.want)
		}
	}
}

func TestAggregationIsDeterministic(t *testing.T) {
	competencies := []evaluation.Competency{
		{ID: "a", Name: "A"}, {ID: "b", Name: "B"},
	}
	spans := append(evidence("b", "supporting", 3), evidence("a", "gap", 1)...)

	first := evaluation.Aggregate(rubricFixture(t), competencies, spans)
	second := evaluation.Aggregate(rubricFixture(t), competencies, spans)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("two aggregations of one input disagreed")
	}
	one, _ := json.Marshal(first)
	two, _ := json.Marshal(second)
	if string(one) != string(two) {
		t.Fatal("serialization is not stable; the result digest would drift")
	}
}

func TestAnIncoherentRubricNeverJudges(t *testing.T) {
	cases := map[string]string{
		"no bands":         `{"sufficiency":{"min_supporting":2},"bands":[]}`,
		"zero sufficiency": `{"sufficiency":{"min_supporting":0},"bands":[{"id":"x","min_ratio":0}]}`,
		"descending bands": `{"sufficiency":{"min_supporting":1},"bands":[{"id":"a","min_ratio":0},{"id":"b","min_ratio":0.8},{"id":"c","min_ratio":0.5}]}`,
		"floating floor":   `{"sufficiency":{"min_supporting":1},"bands":[{"id":"a","min_ratio":0.3}]}`,
		"a nameless band":  `{"sufficiency":{"min_supporting":1},"bands":[{"id":"","min_ratio":0}]}`,
	}
	for name, raw := range cases {
		if _, err := evaluation.ParseRubric([]byte(raw)); err == nil {
			t.Errorf("%s parsed without complaint", name)
		}
	}
}

func TestTheShippedRubricParses(t *testing.T) {
	// Across the module boundary, hence -count=1 in test-go.
	raw, err := os.ReadFile("../../../intelligence/artifacts/rubric/practice-default@1.1.0.json")
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	var envelope struct {
		Type string          `json:"type"`
		Body json.RawMessage `json:"body"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Type != "rubric" {
		t.Fatalf("envelope: %v (%s)", err, envelope.Type)
	}
	if _, err := evaluation.ParseRubric(envelope.Body); err != nil {
		t.Fatalf("the shipped rubric does not parse: %v", err)
	}
}

// EVL-03: coverage and the insufficient-evidence outcome as first-class
// facts, not derivable side effects.

func TestSilenceAndThinEvidenceAreDistinctReasons(t *testing.T) {
	rubric := rubricFixture(t)
	competencies := []evaluation.Competency{
		{ID: "never-raised", Name: "Never raised"},
		{ID: "touched-once", Name: "Touched once"},
	}
	spans := evidence("touched-once", "supporting", 1)

	aggregation := evaluation.Aggregate(rubric, competencies, spans)

	byID := map[string]evaluation.CompetencyResult{}
	for _, result := range aggregation.Competencies {
		byID[result.CompetencyID] = result
	}

	silent := byID["never-raised"]
	if silent.Status != "unassessed" || !hasReason(silent, "NOT_DISCUSSED") {
		t.Fatalf("a competency the conversation never reached must say NOT_DISCUSSED: %+v", silent)
	}
	if hasReason(silent, "INSUFFICIENT_EVIDENCE") {
		t.Fatalf("NOT_DISCUSSED and INSUFFICIENT_EVIDENCE name different failures; both on one result blurs them: %+v", silent)
	}

	thin := byID["touched-once"]
	if thin.Status != "unassessed" || !hasReason(thin, "INSUFFICIENT_EVIDENCE") {
		t.Fatalf("a discussed competency below the threshold must say INSUFFICIENT_EVIDENCE: %+v", thin)
	}
	if hasReason(thin, "NOT_DISCUSSED") {
		t.Fatalf("the conversation did reach this competency: %+v", thin)
	}
}

func TestCoverageNamesWhatWasReachedAndWhatWasNot(t *testing.T) {
	rubric := rubricFixture(t)
	competencies := []evaluation.Competency{
		{ID: "a-reached", Name: "A"},
		{ID: "b-silent", Name: "B"},
		{ID: "c-reached", Name: "C"},
	}
	spans := append(evidence("a-reached", "supporting", 1),
		evidence("c-reached", "supporting", 2)...)

	aggregation := evaluation.Aggregate(rubric, competencies, spans)

	if got, want := aggregation.Coverage.Reached, []string{"a-reached", "c-reached"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("reached = %v, want %v", got, want)
	}
	if got, want := aggregation.Coverage.NotReached, []string{"b-silent"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("not reached = %v, want %v", got, want)
	}
	if aggregation.CoveredCompetencies != 2 || aggregation.TotalCompetencies != 3 {
		t.Fatalf("counts disagree with the named lists: %d/%d", aggregation.CoveredCompetencies, aggregation.TotalCompetencies)
	}
}

func TestUnassessedCompetenciesNeverLowerAnAssessedResult(t *testing.T) {
	// The third box, as a property: the same evidence about the same
	// competency earns the same band whether or not other competencies
	// went unassessed. There is no overall number anywhere in the
	// aggregation for silence to drag toward zero.
	rubric := rubricFixture(t)
	spans := evidence("systems-design", "supporting", 4)

	alone := evaluation.Aggregate(rubric, []evaluation.Competency{{ID: "systems-design", Name: "Systems design"}}, spans)
	crowded := evaluation.Aggregate(rubric, []evaluation.Competency{
		{ID: "systems-design", Name: "Systems design"},
		{ID: "silent-one", Name: "Silent one"},
		{ID: "silent-two", Name: "Silent two"},
	}, spans)

	var aloneResult, crowdedResult evaluation.CompetencyResult
	for _, r := range alone.Competencies {
		if r.CompetencyID == "systems-design" {
			aloneResult = r
		}
	}
	for _, r := range crowded.Competencies {
		if r.CompetencyID == "systems-design" {
			crowdedResult = r
		}
	}
	if !reflect.DeepEqual(aloneResult, crowdedResult) {
		t.Fatalf("silence elsewhere changed an assessed result:\nalone   %+v\ncrowded %+v", aloneResult, crowdedResult)
	}
}

func hasReason(result evaluation.CompetencyResult, code string) bool {
	for _, reason := range result.ReasonCodes {
		if reason == code {
			return true
		}
	}
	return false
}

func TestResponseLatencyIsInvisibleToScoring(t *testing.T) {
	// SES-05's third box, on the aggregation path: two candidates give the
	// same answers, one after long pauses. Every span differs only in its
	// clock values, and the aggregation must be identical - timing is
	// provenance for replay, never an input to a score.
	rubric := rubricFixture(t)
	competencies := []evaluation.Competency{{ID: "systems-design", Name: "Systems design"}}

	prompt := evidence("systems-design", "supporting", 3)
	delayed := make([]evaluation.StoredSpan, len(prompt))
	copy(delayed, prompt)
	for i := range delayed {
		delayed[i].StartMs += 90_000
		delayed[i].EndMs += 240_000
	}

	fast := evaluation.Aggregate(rubric, competencies, prompt)
	slow := evaluation.Aggregate(rubric, competencies, delayed)
	if !reflect.DeepEqual(fast, slow) {
		t.Fatalf("latency changed the aggregation:\nfast %+v\nslow %+v", fast, slow)
	}
}

// EVL-05: confidence from the rubric's own versioned rules (ADR-0015),
// evidence references on every result, and the publication gate that
// recomputes everything from the store before anything publishes.

func TestConfidenceIsTheRubricsRuleNeverAnAssertion(t *testing.T) {
	rubric := rubricFixture(t)
	competencies := []evaluation.Competency{{ID: "systems-design", Name: "Systems design"}}

	cases := []struct {
		name       string
		spans      []evaluation.StoredSpan
		status     string
		confidence string
	}{
		{"four clean supporting spans earn high", evidence("systems-design", "supporting", 4), "assessed", "high"},
		{"two supporting spans earn medium", evidence("systems-design", "supporting", 2), "assessed", "medium"},
		{"contradictions past the ceiling demote to low",
			append(evidence("systems-design", "supporting", 4), evidence("systems-design", "contradictory", 2)...),
			"assessed", "low"},
		{"unassessed is not_assessable, never low", evidence("systems-design", "supporting", 1), "unassessed", "not_assessable"},
		{"silence is not_assessable too", nil, "unassessed", "not_assessable"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := evaluation.Aggregate(rubric, competencies, c.spans).Competencies[0]
			if result.Status != c.status || result.Confidence != c.confidence {
				t.Fatalf("status/confidence = %s/%s, want %s/%s", result.Status, result.Confidence, c.status, c.confidence)
			}
		})
	}
}

func TestEveryResultNamesTheEvidenceItAggregated(t *testing.T) {
	rubric := rubricFixture(t)
	spans := append(evidence("systems-design", "supporting", 2), evidence("debugging", "supporting", 2)...)
	aggregation := evaluation.Aggregate(rubric, []evaluation.Competency{
		{ID: "debugging", Name: "Debugging"},
		{ID: "systems-design", Name: "Systems design"},
	}, spans)

	for _, result := range aggregation.Competencies {
		if len(result.EvidenceIDs) != 2 {
			t.Fatalf("%s references %d spans, want 2", result.CompetencyID, len(result.EvidenceIDs))
		}
		for _, id := range result.EvidenceIDs {
			if !strings.Contains(id, result.CompetencyID) {
				t.Fatalf("%s references a foreign span %s", result.CompetencyID, id)
			}
		}
	}
}

func TestARubricWithoutCoherentConfidenceRulesRefuses(t *testing.T) {
	cases := map[string]string{
		"no confidence block": `{"sufficiency":{"min_supporting":2},"bands":[{"id":"only","min_ratio":0}]}`,
		"high below medium": `{"sufficiency":{"min_supporting":2},"bands":[{"id":"only","min_ratio":0}],
			"confidence":{"high":{"min_supporting":1,"max_contradictory":0},"medium":{"min_supporting":2,"max_contradictory":1}}}`,
		"medium below sufficiency": `{"sufficiency":{"min_supporting":2},"bands":[{"id":"only","min_ratio":0}],
			"confidence":{"high":{"min_supporting":4,"max_contradictory":0},"medium":{"min_supporting":1,"max_contradictory":1}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := evaluation.ParseRubric([]byte(body)); !errors.Is(err, evaluation.ErrRubricIncoherent) {
				t.Fatalf("%s = %v, want ErrRubricIncoherent", name, err)
			}
		})
	}
}

func TestThePublicationGateRefusesEveryTamperedResult(t *testing.T) {
	// The gate is independent of the aggregator by recomputing from the
	// stored spans, so a model-backed aggregator later is held to the
	// same arithmetic. Each tamper is one lie a publication must catch.
	rubric := rubricFixture(t)
	competencies := []evaluation.Competency{{ID: "systems-design", Name: "Systems design"}}
	spans := evidence("systems-design", "supporting", 4)
	honest := evaluation.Aggregate(rubric, competencies, spans)

	if err := evaluation.ValidatePublication(rubric, competencies, spans, honest); err != nil {
		t.Fatalf("the honest aggregation was refused: %v", err)
	}

	cases := map[string]func(a *evaluation.Aggregation){
		"a dangling evidence reference": func(a *evaluation.Aggregation) {
			a.Competencies[0].EvidenceIDs[0] = "sp-fabricated"
		},
		"an inflated supporting count": func(a *evaluation.Aggregation) {
			a.Competencies[0].Supporting++
		},
		"a band the ratio did not earn": func(a *evaluation.Aggregation) {
			a.Competencies[0].Band = "solid"
		},
		"unassessed wearing a band": func(a *evaluation.Aggregation) {
			a.Competencies[0].Status = "unassessed"
		},
		"a confidence the rules did not produce": func(a *evaluation.Aggregation) {
			a.Competencies[0].Confidence = "low"
		},
		"a competency nobody asked about": func(a *evaluation.Aggregation) {
			a.Competencies[0].CompetencyID = "invented"
		},
	}
	for name, tamper := range cases {
		t.Run(name, func(t *testing.T) {
			tampered := evaluation.Aggregate(rubric, competencies, spans)
			tamper(&tampered)
			if err := evaluation.ValidatePublication(rubric, competencies, spans, tampered); !errors.Is(err, evaluation.ErrUnpublishable) {
				t.Fatalf("%s = %v, want ErrUnpublishable", name, err)
			}
		})
	}
}
