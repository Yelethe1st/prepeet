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
			if _, got := resolveInsight(document, probe.kind, probe.key); got != probe.want {
				t.Fatalf("resolveInsight(%q, %q) = %v, want %v",
					probe.kind, probe.key, got, probe.want)
			}
		})
	}
}

// Strengths are in the contract's vocabulary and nothing generates them yet,
// so no key can name one. Accepting any would be accepting all of them.
func TestNoStrengthIsAcceptedWhileNoneAreGenerated(t *testing.T) {
	document := json.RawMessage(analysisWithCoaching)

	if _, found := resolveInsight(document, evaluation.InsightStrength, "fluency"); found {
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
			if _, found := resolveInsight(document, evaluation.InsightPriority, "fluency"); found {
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

	if !strings.Contains(method, "resolveInsight(") {
		t.Fatal("RecordInsightFeedback no longer checks the key against the analysis, " +
			"so a fabricated one is stored and reaches QUA-06's rates")
	}
	if !strings.Contains(method, "ErrFeedbackUnknownInsight") {
		t.Fatal("the check is made and its answer is not acted on")
	}
}

// Regression from the second ART review: the dimension came from the client
// and nothing checked it, so a caller could pass a real key with a fabricated
// dimension and corrupt per-dimension monitoring while passing the key guard.
// It is read from the analysis now.
func TestTheDimensionComesFromTheAnalysisNotTheCaller(t *testing.T) {
	document := json.RawMessage(analysisWithCoaching)

	t.Run("a priority is attributed to itself", func(t *testing.T) {
		insight, found := resolveInsight(document, evaluation.InsightPriority, "fluency")
		if !found {
			t.Fatal("a generated priority was not found")
		}
		if insight.dimension != "fluency" {
			t.Fatalf("dimension = %q, want the analysis's own", insight.dimension)
		}
	})

	t.Run("a drill is attributed to the priority it was chosen for", func(t *testing.T) {
		insight, found := resolveInsight(document, evaluation.InsightDrill, "headline_first")
		if !found {
			t.Fatal("a chosen drill was not found")
		}
		// Not the drill's key, and not whatever the caller sent: the drill was
		// chosen because of pace, so that is what a verdict about it says.
		if insight.dimension != "pace" {
			t.Fatalf("dimension = %q, want pace", insight.dimension)
		}
	})
}

// Regression: artifact_digest held the seal's evaluation-input digest, which
// is unique per session, so the rate per artifact put every candidate in a
// group of one and answered nothing.
func TestTheArtifactIsTheGoverendRevisionNotTheTranscript(t *testing.T) {
	insight, found := resolveInsight(json.RawMessage(analysisWithCoaching),
		evaluation.InsightPriority, "fluency")
	if !found {
		t.Fatal("not found")
	}

	if insight.artifact != "articulation-coaching-v1" {
		t.Fatalf("artifact = %q, want the coaching version", insight.artifact)
	}
	if strings.HasPrefix(insight.artifact, "sha256:") {
		t.Fatal("the transcript digest is being used as the aggregation key")
	}
}

// An analysis that does not say what generated it cannot have a verdict
// attributed to anything. "unknown" in the aggregation column is worse than no
// row: it pools unrelated sessions under one name.
func TestAnAnalysisWithNoCoachingVersionAcceptsNothing(t *testing.T) {
	document := json.RawMessage(`{"coaching":{"priorities":[{"dimension":"fluency","drill":"d"}]}}`)

	if _, found := resolveInsight(document, evaluation.InsightPriority, "fluency"); found {
		t.Fatal("a verdict was accepted with nothing to attribute it to")
	}
}
