package progression_test

import (
	"testing"
	"time"

	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
)

// PRG-03's pure half. Three properties are asserted here and each is a
// thing the ticket asks for rather than a thing the code happens to do:
// a goal tracks real progress from the same observations readiness reads,
// a rubric version change leaves that progress standing, and the cadence
// has no way to express a reproach.

// scale is the band order these tests share, weakest first.
var scale = []string{"emerging", "developing", "solid", "strong"}

// goalAt builds an active goal targeting one competency at one band.
func goalAt(competency, target string) progression.Goal {
	return progression.Goal{
		ID:              "goal-" + competency,
		Origin:          progression.GoalFromGap,
		OriginReference: "readiness/backend-engineer",
		CompetencyID:    competency,
		TargetBand:      target,
		RubricReference: "rubric/practice-default",
		Bands:           scale,
		Status:          progression.GoalActive,
		CreatedAt:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// bandAt builds one assessed observation of a competency at a band.
func bandAt(id, competency, band, rubricVersion string, at time.Time) progression.Observation {
	return progression.Observation{
		ID: id, SessionID: "session-" + id, EvaluationID: "evaluation-" + id,
		CompetencyID: competency, Status: "assessed", Band: band,
		Confidence: "medium", EvidenceCount: 3, Supporting: 2,
		RubricReference: "rubric/practice-default", RubricVersion: rubricVersion,
		RubricDigest: "sha256:" + rubricVersion, ObservedAt: at,
	}
}

func day(n int) time.Time {
	return time.Date(2026, 1, n, 12, 0, 0, 0, time.UTC)
}

func TestAGoalTracksTheBandItsCompetencyActuallyReached(t *testing.T) {
	t.Parallel()
	progress := progression.TrackGoal(goalAt("systems-design", "strong"), []progression.Observation{
		bandAt("a", "systems-design", "developing", "1.0.0", day(2)),
		bandAt("b", "systems-design", "solid", "1.0.0", day(9)),
	}, nil, day(10))

	if progress.State != progression.GoalInProgress {
		t.Fatalf("state = %q, want in_progress", progress.State)
	}
	if progress.CurrentBand != "solid" {
		t.Errorf("current band = %q, want solid", progress.CurrentBand)
	}
	if progress.Reason != "" {
		t.Errorf("a tracked goal with a reading should carry no reason, got %q", progress.Reason)
	}
}

func TestAGoalWhoseCompetencyReachedItsTargetIsReached(t *testing.T) {
	t.Parallel()
	progress := progression.TrackGoal(goalAt("systems-design", "solid"), []progression.Observation{
		bandAt("a", "systems-design", "solid", "1.0.0", day(9)),
	}, nil, day(10))
	if progress.State != progression.GoalReached {
		t.Fatalf("state = %q, want reached", progress.State)
	}
}

func TestAGoalWithNoReadingIsNotStartedRatherThanZero(t *testing.T) {
	t.Parallel()
	// The whole point of the unassessed distinction, carried into goals: a
	// candidate nobody asked must not read as a candidate who failed.
	progress := progression.TrackGoal(goalAt("systems-design", "solid"), nil, nil, day(10))
	if progress.State != progression.GoalNotStarted {
		t.Fatalf("state = %q, want not_started", progress.State)
	}
	if progress.CurrentBand != "" {
		t.Errorf("an untracked goal must carry no band, got %q", progress.CurrentBand)
	}
	if progress.Reason != progression.ReasonNeverObserved {
		t.Errorf("reason = %q, want never_observed", progress.Reason)
	}
}

func TestAGoalCanBeCreatedFromAGapADrillOrACompetency(t *testing.T) {
	t.Parallel()
	for _, origin := range []string{
		progression.GoalFromGap, progression.GoalFromDrill, progression.GoalFromCompetency,
	} {
		goal := goalAt("systems-design", "solid")
		goal.Origin = origin
		if err := goal.Validate(); err != nil {
			t.Errorf("origin %q: %v", origin, err)
		}
	}
}

func TestAGoalMustNameWhereItCameFrom(t *testing.T) {
	t.Parallel()
	goal := goalAt("systems-design", "solid")
	goal.Origin = "because-i-said-so"
	if err := goal.Validate(); err == nil {
		t.Fatal("an unnamed origin was accepted, so no goal can explain itself")
	}
}

func TestAGoalTargetingABandOffItsOwnScaleIsRefused(t *testing.T) {
	t.Parallel()
	goal := goalAt("systems-design", "outstanding")
	if err := goal.Validate(); err == nil {
		t.Fatal("a target off the scale was accepted, so progress toward it is unmeasurable")
	}
}

func TestARubricVersionChangeLeavesTheGoalTracking(t *testing.T) {
	t.Parallel()
	// Box 3. A version bump inside one rubric reference is the same
	// measurement restated, so a goal keeps counting rather than resetting.
	progress := progression.TrackGoal(goalAt("systems-design", "strong"), []progression.Observation{
		bandAt("a", "systems-design", "developing", "1.0.0", day(2)),
		bandAt("b", "systems-design", "solid", "2.0.0", day(9)),
	}, nil, day(10))

	if progress.State != progression.GoalInProgress {
		t.Fatalf("state = %q, want in_progress after a version bump", progress.State)
	}
	if progress.CurrentBand != "solid" {
		t.Errorf("current band = %q: the reading under 2.0.0 should count", progress.CurrentBand)
	}
}

func TestAMilestoneEarnedUnderAnEarlierRubricVersionStands(t *testing.T) {
	t.Parallel()
	// Box 3 again, from the other side. Milestones are the durable part of
	// a goal: they record what a candidate did, pinned to what judged it,
	// and a later rubric cannot take one away.
	earned := []progression.Milestone{{
		GoalID: "goal-systems-design", Band: "developing",
		RubricReference: "rubric/practice-default", RubricVersion: "1.0.0",
		ObservationID: "a", ReachedAt: day(2),
	}}
	progress := progression.TrackGoal(goalAt("systems-design", "strong"), []progression.Observation{
		bandAt("b", "systems-design", "solid", "2.0.0", day(9)),
	}, earned, day(10))

	if len(progress.Milestones) != 2 {
		t.Fatalf("milestones = %d, want the earned one plus the new one", len(progress.Milestones))
	}
	if progress.Milestones[0].RubricVersion != "1.0.0" {
		t.Errorf("the earlier milestone lost its version: %+v", progress.Milestones[0])
	}
	if progress.Fresh[0].Band != "solid" || progress.Fresh[0].RubricVersion != "2.0.0" {
		t.Errorf("the newly reached milestone is wrong: %+v", progress.Fresh[0])
	}
}

func TestAReadingUnderAnotherRubricDoesNotResetAGoal(t *testing.T) {
	t.Parallel()
	// A different rubric reference measures a different thing (PRG-02), so
	// it is reported as incomparable rather than counted or subtracted.
	foreign := bandAt("c", "systems-design", "emerging", "1.0.0", day(20))
	foreign.RubricReference = "rubric/some-other-standard"

	earned := []progression.Milestone{{
		GoalID: "goal-systems-design", Band: "solid",
		RubricReference: "rubric/practice-default", RubricVersion: "1.0.0",
		ObservationID: "a", ReachedAt: day(2),
	}}
	progress := progression.TrackGoal(goalAt("systems-design", "strong"),
		[]progression.Observation{foreign}, earned, day(21))

	if progress.CurrentBand != "" {
		t.Errorf("an incomparable reading became the current band: %q", progress.CurrentBand)
	}
	if progress.Reason != progression.ReasonIncomparableRubric {
		t.Errorf("reason = %q, want incomparable_rubric", progress.Reason)
	}
	if len(progress.Milestones) != 1 {
		t.Fatalf("the earned milestone was dropped: %+v", progress.Milestones)
	}
}

func TestABandIsOnlyEverAMilestoneOnce(t *testing.T) {
	t.Parallel()
	// Reaching solid twice is one achievement, not two, or a chart of
	// milestones would reward repetition rather than progress.
	progress := progression.TrackGoal(goalAt("systems-design", "strong"), []progression.Observation{
		bandAt("a", "systems-design", "solid", "1.0.0", day(2)),
		bandAt("b", "systems-design", "solid", "1.0.0", day(9)),
	}, nil, day(10))
	if len(progress.Fresh) != 1 {
		t.Fatalf("new milestones = %d, want 1: %+v", len(progress.Fresh), progress.Fresh)
	}
	if progress.Fresh[0].ReachedAt != day(2) {
		t.Errorf("the milestone should date from the first time it was reached, got %v",
			progress.Fresh[0].ReachedAt)
	}
}

func TestAPausedGoalStopsEarningMilestonesWithoutLosingTheOnesItHas(t *testing.T) {
	t.Parallel()
	goal := goalAt("systems-design", "strong")
	goal.Status = progression.GoalPaused
	earned := []progression.Milestone{{
		GoalID: goal.ID, Band: "developing", RubricReference: goal.RubricReference,
		RubricVersion: "1.0.0", ObservationID: "a", ReachedAt: day(2),
	}}
	progress := progression.TrackGoal(goal, []progression.Observation{
		bandAt("b", "systems-design", "solid", "1.0.0", day(9)),
	}, earned, day(10))

	if len(progress.Fresh) != 0 {
		t.Errorf("a paused goal earned a milestone: %+v", progress.Fresh)
	}
	if len(progress.Milestones) != 1 {
		t.Errorf("a paused goal lost what it had earned: %+v", progress.Milestones)
	}
}

// --------------------------------------------------------------- cadence

func TestPractisingThisWeekAndTheLastTwoIsARunOfThree(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 21, 9, 0, 0, 0, time.UTC) // a Wednesday
	cadence := progression.Practice([]progression.Observation{
		bandAt("a", "systems-design", "solid", "1.0.0", day(7)),
		bandAt("b", "systems-design", "solid", "1.0.0", day(14)),
		bandAt("c", "systems-design", "solid", "1.0.0", day(21)),
	}, now)

	if cadence.Run != 3 {
		t.Fatalf("run = %d, want 3", cadence.Run)
	}
	if !cadence.ActiveThisWeek {
		t.Error("the candidate practised today and the cadence says otherwise")
	}
	if cadence.Resting {
		t.Error("a candidate practising every week is not resting")
	}
}

func TestTheCurrentWeekIsNeverCountedAgainstTheCandidate(t *testing.T) {
	t.Parallel()
	// Monday morning, having practised every week until now. A cadence that
	// counted the unfinished week would tell somebody they had lost a run
	// they still have every chance to keep.
	now := time.Date(2026, 1, 19, 8, 0, 0, 0, time.UTC)
	cadence := progression.Practice([]progression.Observation{
		bandAt("a", "systems-design", "solid", "1.0.0", day(7)),
		bandAt("b", "systems-design", "solid", "1.0.0", day(14)),
	}, now)

	if cadence.Run != 2 {
		t.Fatalf("run = %d, want 2: the week in progress must not end a run", cadence.Run)
	}
	if cadence.ActiveThisWeek {
		t.Error("nothing has been practised this week yet")
	}
	if cadence.Resting {
		t.Error("a week that has barely started is not a rest")
	}
}

func TestALapsedRunIsRestAndTheLongestRunIsKept(t *testing.T) {
	t.Parallel()
	// Box 2. Coming back after a month finds the longest run intact and the
	// state named "resting". Nothing anywhere counts what was missed.
	now := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	cadence := progression.Practice([]progression.Observation{
		bandAt("a", "systems-design", "solid", "1.0.0", day(7)),
		bandAt("b", "systems-design", "solid", "1.0.0", day(14)),
		bandAt("c", "systems-design", "solid", "1.0.0", day(21)),
	}, now)

	if cadence.Run != 0 {
		t.Errorf("run = %d, want 0 after a long gap", cadence.Run)
	}
	if !cadence.Resting {
		t.Error("a lapsed run should read as resting")
	}
	if cadence.LongestRun != 3 {
		t.Errorf("longest run = %d, want the 3 that were actually done", cadence.LongestRun)
	}
	if cadence.WeeksPractised != 3 {
		t.Errorf("weeks practised = %d, want 3", cadence.WeeksPractised)
	}
}

func TestACandidateWhoHasNeverPractisedIsNotResting(t *testing.T) {
	t.Parallel()
	// "Resting" implies something to rest from. A first-run candidate gets
	// neither a broken streak nor a consolation.
	cadence := progression.Practice(nil, day(10))
	if cadence.Resting || cadence.Run != 0 || cadence.LongestRun != 0 {
		t.Fatalf("a first-run cadence should be entirely empty: %+v", cadence)
	}
	if !cadence.LastPractisedAt.IsZero() {
		t.Errorf("last practised = %v, want the zero time", cadence.LastPractisedAt)
	}
}

func TestSeveralSessionsInOneWeekAreOneWeek(t *testing.T) {
	t.Parallel()
	// Cadence measures habit, not volume. Counting sessions would reward
	// cramming and make a lighter week look like a decline.
	now := time.Date(2026, 1, 9, 9, 0, 0, 0, time.UTC)
	cadence := progression.Practice([]progression.Observation{
		bandAt("a", "systems-design", "solid", "1.0.0", day(5)),
		bandAt("b", "communication", "solid", "1.0.0", day(5)),
		bandAt("c", "systems-design", "solid", "1.0.0", day(8)),
	}, now)
	if cadence.WeeksPractised != 1 {
		t.Fatalf("weeks practised = %d, want 1", cadence.WeeksPractised)
	}
}
