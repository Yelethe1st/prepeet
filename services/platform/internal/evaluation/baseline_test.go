package evaluation_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
)

// ART-07's derivation: no range before the documented minimum, ranges
// made only of values that occurred, and copy that calls them guidance.

func measured(wpm float64, status string) evaluation.Articulation {
	document, _ := json.Marshal(map[string]any{"metrics": map[string]float64{
		"words_per_minute": wpm, "fillers_per_100_words": wpm / 20, "long_pause_count": 2,
	}})
	return evaluation.Articulation{Status: status, Document: document}
}

func TestNoBaselineBeforeTheDocumentedMinimum(t *testing.T) {
	history := []evaluation.Articulation{}
	for i := 0; i < evaluation.MinBaselineSessions-1; i++ {
		history = append(history, measured(150, "assessable"))
	}
	baseline := evaluation.DeriveBaseline(history)
	if baseline.Ready || len(baseline.Ranges) != 0 {
		t.Fatalf("a baseline was drawn from %d sessions: %+v", len(history), baseline)
	}
	if baseline.SessionsMeasured != evaluation.MinBaselineSessions-1 || baseline.MinimumSessions != evaluation.MinBaselineSessions {
		t.Fatalf("the absence does not say how far it is: %+v", baseline)
	}
}

func TestNotAssessableSessionsNeverCountTowardsTheMinimum(t *testing.T) {
	history := []evaluation.Articulation{}
	for i := 0; i < evaluation.MinBaselineSessions; i++ {
		history = append(history, measured(150, "not_assessable"))
	}
	if baseline := evaluation.DeriveBaseline(history); baseline.Ready {
		t.Fatal("unmeasured sessions drew a baseline")
	}
}

func TestTheRangeIsTheMiddleHalfOfValuesThatOccurred(t *testing.T) {
	history := []evaluation.Articulation{}
	for _, wpm := range []float64{110, 130, 150, 170, 190, 210, 230, 250} {
		history = append(history, measured(wpm, "assessable"))
	}
	baseline := evaluation.DeriveBaseline(history)
	if !baseline.Ready {
		t.Fatal("eight measured sessions drew no baseline")
	}
	pace := baseline.Ranges["words_per_minute"]
	if pace.Low != 130 || pace.High != 210 {
		t.Fatalf("pace range = %+v, want the middle half 130 to 210", pace)
	}
	for metric, span := range baseline.Ranges {
		if span.Low > span.High {
			t.Fatalf("%s range is inverted: %+v", metric, span)
		}
	}
}

func TestTheCopyCallsRangesGuidanceNotACorrectRate(t *testing.T) {
	baseline := evaluation.DeriveBaseline(nil)
	if !strings.Contains(baseline.Note, "not a target") || !strings.Contains(baseline.Note, "no correct speaking rate") {
		t.Fatalf("note = %q", baseline.Note)
	}
	if baseline.BaselineVersion != evaluation.BaselineVersion {
		t.Fatalf("version = %q", baseline.BaselineVersion)
	}
	_ = fmt.Sprintf
}
