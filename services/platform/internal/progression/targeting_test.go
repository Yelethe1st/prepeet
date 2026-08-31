package progression_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
)

// PRG-05. The four properties, each asserted as a thing that could
// plausibly go wrong: a recommendation that covers nothing, one that
// covers only the gaps, one that cannot say why, and one that invents a
// trend out of two readings from last spring.

// roleCompetencies is the catalogue answer these tests share: what a
// session for this role could ask about at all.
var roleCompetencies = []string{
	"systems-design", "debugging", "communication", "testing", "trade-offs",
}

// catalogue is the consumer-defined port, satisfied by a stub here and by
// the catalogue context in cmd.
type catalogue struct {
	competencies []string
	err          error
}

func (c catalogue) Competencies(_ context.Context, _ string) ([]string, error) {
	return c.competencies, c.err
}

func recent(competency, band string, daysAgo int) progression.Observation {
	return bandAt("obs-"+competency+band, competency, band, "1.0.0",
		reference.AddDate(0, 0, -daysAgo))
}

// reference is the moment these tests ask from.
var reference = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func recommend(t *testing.T, observations []progression.Observation) progression.Recommendation {
	t.Helper()
	recommendation, err := progression.NewTargeting(catalogue{competencies: roleCompetencies}).
		Recommend(context.Background(), progression.TargetingRequest{
			RoleID:          "backend-engineer",
			RubricReference: "rubric/practice-default",
			Bands:           scale,
			Slots:           4,
		}, observations, reference)
	if err != nil {
		t.Fatalf("recommend: %v", err)
	}
	return recommendation
}

func TestATargetedGapIsCoveredByTheComposition(t *testing.T) {
	t.Parallel()
	// Box 1. Debugging is weak and testing has never been asked, so both
	// have to appear in what the next session covers.
	recommendation := recommend(t, []progression.Observation{
		recent("systems-design", "strong", 5),
		recent("debugging", "emerging", 7),
		recent("communication", "solid", 9),
		recent("trade-offs", "solid", 9),
	})

	covered := map[string]bool{}
	for _, competency := range recommendation.Covers {
		covered[competency] = true
	}
	for _, gap := range []string{"debugging", "testing"} {
		if !covered[gap] {
			t.Errorf("%q is a gap and the composition does not cover it: %v", gap, recommendation.Covers)
		}
	}
}

func TestTargetingIsNeverTheWholeSession(t *testing.T) {
	t.Parallel()
	// Box 2. Every competency is weak, which is exactly when a naive
	// recommendation turns practice into a narrow loop. At least one slot
	// stays open for something that is not a gap.
	recommendation := recommend(t, []progression.Observation{
		recent("systems-design", "emerging", 5),
		recent("debugging", "emerging", 5),
		recent("communication", "emerging", 5),
		recent("testing", "emerging", 5),
		recent("trade-offs", "emerging", 5),
	})

	if len(recommendation.Targeted) >= len(recommendation.Covers) {
		t.Fatalf("every slot is targeted: targeted %v of covered %v",
			recommendation.Targeted, recommendation.Covers)
	}
	if len(recommendation.Covers) != 4 {
		t.Errorf("covers %d competencies, want the 4 slots asked for", len(recommendation.Covers))
	}
}

func TestEveryTargetedCompetencySaysWhyItWasChosen(t *testing.T) {
	t.Parallel()
	// Box 3. A recommendation nobody can interrogate is one nobody can
	// disagree with, which is the wrong kind of authority for a system
	// telling somebody what to work on.
	// Every competency has been read, so nothing is targeted for being a
	// blank and the below-band branch is the one under test. The first
	// version of this test left three competencies unobserved, they took
	// every slot, and deleting the below-band explanation did not fail it.
	recommendation := recommend(t, []progression.Observation{
		recent("systems-design", "strong", 5),
		recent("debugging", "emerging", 7),
		recent("communication", "strong", 5),
		recent("testing", "strong", 5),
		recent("trade-offs", "strong", 5),
	})

	if len(recommendation.Targeted) == 0 {
		t.Fatal("nothing was targeted, so no explanation was checked")
	}
	for _, target := range recommendation.Targeted {
		if target.Because == "" {
			t.Errorf("%q was targeted with no reason", target.CompetencyID)
		}
		switch target.Reason {
		case progression.TargetBelowBand:
			if target.ObservationID == "" {
				t.Errorf("%q cites a weak reading but names no observation", target.CompetencyID)
			}
			if !strings.Contains(target.Because, target.ObservedBand) {
				t.Errorf("%q's explanation does not mention the band it read: %q",
					target.CompetencyID, target.Because)
			}
		case progression.TargetNeverObserved, progression.TargetStaleEvidence:
		default:
			t.Errorf("%q was targeted for an unnamed reason %q", target.CompetencyID, target.Reason)
		}
	}
}

func TestNeverObservedIsTargetedWithoutBeingCalledWeak(t *testing.T) {
	t.Parallel()
	// The unassessed rule, carried into recommendations. Testing has never
	// been asked about, which is a reason to ask, and not a finding.
	recommendation := recommend(t, []progression.Observation{
		recent("systems-design", "strong", 5),
	})
	for _, target := range recommendation.Targeted {
		if target.CompetencyID != "testing" {
			continue
		}
		if target.Reason != progression.TargetNeverObserved {
			t.Fatalf("reason = %q, want never_observed", target.Reason)
		}
		if target.ObservedBand != "" {
			t.Errorf("an unasked competency was given the band %q", target.ObservedBand)
		}
		return
	}
	t.Fatal("a competency nobody has been asked about was not targeted")
}

func TestStaleEvidenceIsAReasonToAskAgain(t *testing.T) {
	t.Parallel()
	// PRG-04's freshness put to work: a strong reading from six months ago
	// is a claim about somebody who has practised since, so it earns a
	// revisit rather than a pass.
	recommendation := recommend(t, []progression.Observation{
		recent("systems-design", "strong", 200),
		recent("debugging", "strong", 5),
		recent("communication", "strong", 5),
		recent("testing", "strong", 5),
		recent("trade-offs", "strong", 5),
	})
	for _, target := range recommendation.Targeted {
		if target.CompetencyID == "systems-design" && target.Reason == progression.TargetStaleEvidence {
			return
		}
	}
	t.Fatalf("a six-month-old reading was treated as current: %+v", recommendation.Targeted)
}

func TestSparseHistoryProducesACautiousRecommendation(t *testing.T) {
	t.Parallel()
	// Box 4. One reading is not a trend. The recommendation still says what
	// to ask about, and says out loud that it is guessing.
	recommendation := recommend(t, []progression.Observation{
		recent("systems-design", "emerging", 5),
	})
	if !recommendation.Cautious {
		t.Fatal("a single reading produced a confident recommendation")
	}
	if recommendation.Caution == "" {
		t.Error("the recommendation is cautious and does not say why")
	}
	if len(recommendation.Covers) == 0 {
		t.Error("caution became silence: a first-run candidate still gets a session")
	}
}

func TestNoHistoryAtAllIsCautiousAndStillComposes(t *testing.T) {
	t.Parallel()
	recommendation := recommend(t, nil)
	if !recommendation.Cautious {
		t.Fatal("a candidate with no history got a confident recommendation")
	}
	if len(recommendation.Covers) != 4 {
		t.Fatalf("covers %d, want the 4 slots asked for", len(recommendation.Covers))
	}
	for _, target := range recommendation.Targeted {
		if target.Reason != progression.TargetNeverObserved {
			t.Errorf("%q was targeted for %q with no history at all",
				target.CompetencyID, target.Reason)
		}
	}
}

func TestAHistoryUnderAnotherRubricIsCautiousRatherThanCounted(t *testing.T) {
	t.Parallel()
	// Incomparable readings are not evidence about this rubric's
	// competencies. Counting them would produce a confident recommendation
	// built on a measurement of something else.
	// Two readings each, so that counting them would put the history well
	// past the point at which a recommendation stops being cautious. With
	// one reading each the caution threshold hid the bug: deleting the
	// rubric filter left the recommendation cautious anyway, for the wrong
	// reason, and this test passed.
	observations := make([]progression.Observation, 0, 2*len(roleCompetencies))
	for _, competency := range roleCompetencies {
		for round, daysAgo := range []int{5, 40} {
			foreign := recent(competency, "strong", daysAgo)
			foreign.ID = fmt.Sprintf("foreign-%s-%d", competency, round)
			foreign.RubricReference = "rubric/some-other-standard"
			observations = append(observations, foreign)
		}
	}
	recommendation := recommend(t, observations)
	if !recommendation.Cautious {
		t.Fatal("readings from another rubric were counted as history")
	}
	for _, target := range recommendation.Targeted {
		if target.ObservedBand != "" {
			t.Errorf("%q borrowed a band from an incomparable reading", target.CompetencyID)
		}
	}
}

func TestARecommendationCannotBeMadeWithoutARoleToMakeItFor(t *testing.T) {
	t.Parallel()
	_, err := progression.NewTargeting(catalogue{competencies: roleCompetencies}).
		Recommend(context.Background(), progression.TargetingRequest{
			RubricReference: "rubric/practice-default", Bands: scale, Slots: 4,
		}, nil, reference)
	if !errors.Is(err, progression.ErrUntargetable) {
		t.Fatalf("err = %v, want ErrUntargetable", err)
	}
}

func TestACatalogueWithNothingToAskAboutIsRefusedRatherThanInvented(t *testing.T) {
	t.Parallel()
	// The port answering nothing is a real state: a role whose standard has
	// not been published yet. Composing a session from an empty list would
	// mean inventing competencies nobody catalogued.
	_, err := progression.NewTargeting(catalogue{}).
		Recommend(context.Background(), progression.TargetingRequest{
			RoleID: "backend-engineer", RubricReference: "rubric/practice-default",
			Bands: scale, Slots: 4,
		}, nil, reference)
	if !errors.Is(err, progression.ErrUntargetable) {
		t.Fatalf("err = %v, want ErrUntargetable", err)
	}
}

func TestMoreSlotsThanCompetenciesCoversWhatExistsAndNoMore(t *testing.T) {
	t.Parallel()
	recommendation, err := progression.NewTargeting(catalogue{competencies: roleCompetencies}).
		Recommend(context.Background(), progression.TargetingRequest{
			RoleID: "backend-engineer", RubricReference: "rubric/practice-default",
			Bands: scale, Slots: 50,
		}, nil, reference)
	if err != nil {
		t.Fatalf("recommend: %v", err)
	}
	if len(recommendation.Covers) != len(roleCompetencies) {
		t.Fatalf("covers %d of %d catalogued competencies",
			len(recommendation.Covers), len(roleCompetencies))
	}
}
