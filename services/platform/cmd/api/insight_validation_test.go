package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
)

// Regression from the ART-09 review: the adapter read the stored analysis for
// its digest and policy version and then passed the client's kind and key
// straight to the store, which checks the kind against a closed set and
// refuses an empty key and nothing more. A candidate could write unbounded
// rows against their own session under fabricated keys, and every one of them
// would land in the per-artifact rates QUA-06 reads. A verdict about an
// insight nobody was shown is not junk in the table, it is a wrong answer to
// the question the table exists to ask.

const analysisWithCoaching = `{
  "coaching": {
    "coaching_version": "articulation-coaching-v1",
    "priorities": [
      {"dimension": "fluency", "drill": "deliberate_pause"},
      {"dimension": "pace", "drill": "headline_first"}
    ]
  }
}`

func TestAVerdictMustNameAnInsightTheAnalysisGenerated(t *testing.T) {
	document := json.RawMessage(analysisWithCoaching)

	for name, probe := range map[string]struct {
		kind string
		key  string
		want bool
	}{
		"a priority it generated":     {evaluation.InsightPriority, "fluency", true},
		"the other priority":          {evaluation.InsightPriority, "pace", true},
		"a drill it chose":            {evaluation.InsightDrill, "deliberate_pause", true},
		"the other chosen drill":      {evaluation.InsightDrill, "headline_first", true},
		"a fabricated priority":       {evaluation.InsightPriority, "invented", false},
		"a fabricated drill":          {evaluation.InsightDrill, "invented", false},
		"a drill it did not choose":   {evaluation.InsightDrill, "sixty_second_compression", false},
		"a dimension used as a drill": {evaluation.InsightDrill, "fluency", false},
		"a drill used as a priority":  {evaluation.InsightPriority, "deliberate_pause", false},
		"an empty key":                {evaluation.InsightPriority, "", false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := insightExists(document, probe.kind, probe.key); got != probe.want {
				t.Fatalf("insightExists(%q, %q) = %v, want %v",
					probe.kind, probe.key, got, probe.want)
			}
		})
	}
}

// Strengths are in the contract's vocabulary and nothing generates them yet,
// so no key can name one. Accepting any would be accepting all of them.
func TestNoStrengthIsAcceptedWhileNoneAreGenerated(t *testing.T) {
	document := json.RawMessage(analysisWithCoaching)

	if insightExists(document, evaluation.InsightStrength, "fluency") {
		t.Fatal("a strength was accepted against an analysis that generates none")
	}
}

// An analysis that cannot be read is not one that generated this insight.
// Refusing is the safe direction: the alternative accepts everything the
// moment the document's shape changes.
func TestAnUnreadableAnalysisAcceptsNothing(t *testing.T) {
	for name, document := range map[string]json.RawMessage{
		"empty":       {},
		"not json":    json.RawMessage(`{not json`),
		"no coaching": json.RawMessage(`{"profile":{}}`),
		"no priority": json.RawMessage(`{"coaching":{"priorities":[]}}`),
		"withheld":    json.RawMessage(`{"coaching":{"available":false}}`),
	} {
		t.Run(name, func(t *testing.T) {
			if insightExists(document, evaluation.InsightPriority, "fluency") {
				t.Fatalf("%s accepted a verdict", name)
			}
		})
	}
}

// The rule above is only enforced if the adapter asks it.
//
// Every test in this file exercises insightExists directly, so deleting the
// call in RecordInsightFeedback would leave them all green while the guard did
// nothing: exactly the shape of failure the ownership rule in
// internal/architecture was written to catch. Checking the source is crude and
// it is the only thing here that notices.
func TestTheAdapterActuallyAsksBeforeRecording(t *testing.T) {
	source, err := os.ReadFile("interview_adapter.go")
	if err != nil {
		t.Fatalf("reading the adapter: %v", err)
	}

	body := string(source)
	start := strings.Index(body, "func (a interviewAdapter) RecordInsightFeedback(")
	if start == -1 {
		t.Fatal("RecordInsightFeedback is not where this test expects it")
	}
	end := strings.Index(body[start:], "\nfunc ")
	if end == -1 {
		end = len(body) - start
	}
	method := body[start : start+end]

	if !strings.Contains(method, "insightExists(") {
		t.Fatal("RecordInsightFeedback no longer checks the key against the analysis, " +
			"so a fabricated one is stored and reaches QUA-06's rates")
	}
	if !strings.Contains(method, "ErrFeedbackUnknownInsight") {
		t.Fatal("the check is made and its answer is not acted on")
	}
}
