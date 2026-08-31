package progression

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// Goals, milestones and practice cadence: PRG-03's pure half.
//
// A goal is the candidate's own target, and everything here exists to keep
// it honest in two directions at once. It must track something real, so
// progress is derived from the same observations readiness reads rather
// than from a counter somebody increments. And it must not become a stick,
// so the cadence below has no field that could hold a missed week, a
// broken streak or a failure, and no caller can invent one.
//
// The third property the ticket asks for, surviving a rubric version
// change, is why a goal pins its own band scale and its own rubric
// reference at creation. A version bump inside one reference is the same
// measurement restated and keeps counting; a different reference measures
// something else and is reported as incomparable, which is neither a reset
// nor a silent substitution. Milestones are append-only and each names the
// version it was earned under, so nothing a candidate has already done can
// be taken away by a later publication.

// ErrGoalIncoherent refuses a goal that cannot be tracked: one that
// cannot say where it came from, what it targets, or on what scale.
var ErrGoalIncoherent = errors.New("progression: the goal is incoherent")

// Where a goal came from. Recorded rather than inferred, because a goal
// raised from a readiness gap, one adopted from a drill and one a
// candidate chose outright deserve different words on a screen, and
// because a goal that cannot say why it exists is one nobody can review.
const (
	GoalFromGap        = "gap"
	GoalFromDrill      = "drill"
	GoalFromCompetency = "competency"
)

// A goal's lifecycle. Paused is deliberately not retired: a candidate
// stepping away from a target for a month should find it as they left it,
// with everything they earned intact.
const (
	GoalActive  = "active"
	GoalPaused  = "paused"
	GoalRetired = "retired"
)

// What a goal's evidence currently says.
//
// NotStarted is its own state rather than a zero, for the reason that runs
// through this whole context: a competency nobody has asked about must
// never render like one that was asked and answered badly.
const (
	GoalNotStarted   = "not_started"
	GoalInProgress   = "in_progress"
	GoalReached      = "reached"
	GoalIncomparable = "incomparable"
)

// Goal is one target a candidate set for themselves.
//
// Bands and RubricReference are pinned at creation rather than read from
// whatever standard is current. That is what makes progress reproducible:
// the scale a goal is measured on cannot move under it, and a rubric
// change is visible as a change rather than as a number that quietly
// means something else.
type Goal struct {
	ID              string
	Origin          string
	OriginReference string
	CompetencyID    string
	TargetBand      string
	RubricReference string
	Bands           []string
	Status          string
	CreatedAt       time.Time
}

// Milestone is a band this goal's competency reached, once, with the
// evidence and the rubric version that showed it.
//
// Append-only and never recomputed away. A milestone is a fact about what
// a candidate did on a day, and the point of pinning the version is that
// the fact stays readable after the rubric moves on.
type Milestone struct {
	GoalID          string
	Band            string
	RubricReference string
	RubricVersion   string
	ObservationID   string
	ReachedAt       time.Time
}

// GoalProgress is a goal and what its evidence currently says.
//
// Milestones is everything earned, oldest first. Fresh is the subset this
// call newly recognised, which is what a caller persists; separating them
// keeps "what is true" and "what is new" from being the same list, so a
// second call cannot write a milestone twice.
type GoalProgress struct {
	Goal        Goal
	State       string
	CurrentBand string

	// Reason is set exactly when there is no comparable current reading,
	// and uses the same vocabulary as readiness so a screen has one set of
	// words for the same silence.
	Reason string

	ObservationID string
	ObservedAt    time.Time

	// Evidence says how old the resolving reading is. Carried rather than
	// left to the caller, so a goal cannot be rendered as current standing
	// when the reading behind it is four months old.
	Evidence Evidence

	Milestones []Milestone
	Fresh      []Milestone
}

// Validate states what a trackable goal is.
//
// Checked on the value rather than in a constructor, because a Goal read
// back from the database is as much a Goal as one just built, and the
// guarantee has to hold for both.
func (g Goal) Validate() error {
	switch g.Origin {
	case GoalFromGap, GoalFromDrill, GoalFromCompetency:
	default:
		return fmt.Errorf("%w: %q is not a place a goal can come from", ErrGoalIncoherent, g.Origin)
	}
	switch g.Status {
	case GoalActive, GoalPaused, GoalRetired:
	default:
		return fmt.Errorf("%w: %q is not a goal lifecycle state", ErrGoalIncoherent, g.Status)
	}
	if g.CompetencyID == "" {
		return fmt.Errorf("%w: a goal with no competency measures nothing", ErrGoalIncoherent)
	}
	if g.RubricReference == "" {
		return fmt.Errorf("%w: without a rubric reference nothing states which readings count", ErrGoalIncoherent)
	}
	if len(g.Bands) == 0 {
		return fmt.Errorf("%w: a goal with no band scale cannot say what reaching it means", ErrGoalIncoherent)
	}
	onScale := false
	for _, band := range g.Bands {
		if band == g.TargetBand {
			onScale = true
		}
	}
	if !onScale {
		return fmt.Errorf("%w: target band %q is not on this goal's scale", ErrGoalIncoherent, g.TargetBand)
	}
	return nil
}

// TrackGoal answers what a goal's evidence says, and which milestones that
// evidence newly earns.
//
// The reading it uses is the latest comparable assessed observation of the
// goal's competency, chosen exactly as readiness chooses one, so a goal and
// a readiness screen can never disagree about the same competency. earned
// is the milestones already recorded; they are returned untouched whatever
// the current evidence says, which is the ticket's third property made
// structural rather than remembered.
func TrackGoal(goal Goal, observations []Observation, earned []Milestone, at time.Time) GoalProgress {
	progress := GoalProgress{Goal: goal, Milestones: append([]Milestone(nil), earned...)}
	sort.SliceStable(progress.Milestones, func(i, j int) bool {
		return progress.Milestones[i].ReachedAt.Before(progress.Milestones[j].ReachedAt)
	})

	rank := make(map[string]int, len(goal.Bands))
	for index, band := range goal.Bands {
		rank[band] = index
	}
	comparable := comparableReadings(goal, rank, observations)

	current, found := latestOf(comparable)
	switch {
	case found:
		progress.CurrentBand = current.Band
		progress.ObservationID = current.ID
		progress.ObservedAt = current.ObservedAt
		if rank[current.Band] >= rank[goal.TargetBand] {
			progress.State = GoalReached
		} else {
			progress.State = GoalInProgress
		}
	default:
		progress.State = GoalNotStarted
		progress.Reason = silenceReason(goal, rank, observations)
		if progress.Reason == ReasonIncomparableRubric || progress.Reason == ReasonIncomparableBand {
			progress.State = GoalIncomparable
		}
	}

	// A retired or paused goal is still readable, and still holds what it
	// earned, but stops accruing: a target somebody stepped away from
	// should not keep congratulating them in the background.
	if goal.Status == GoalActive {
		progress.Fresh = newMilestones(goal, rank, comparable, progress.Milestones)
		progress.Milestones = append(progress.Milestones, progress.Fresh...)
		sort.SliceStable(progress.Milestones, func(i, j int) bool {
			return progress.Milestones[i].ReachedAt.Before(progress.Milestones[j].ReachedAt)
		})
	}
	progress.Evidence = Freshness(progress.ObservedAt, at)
	return progress
}

// comparableReadings filters the history to the assessed readings of this
// goal's competency that its pinned rubric reference and scale can place.
//
// Superseded rows are dropped for the same reason readiness drops them: a
// correction that has been corrected is not what the record says.
func comparableReadings(goal Goal, rank map[string]int, observations []Observation) []Observation {
	superseded := make(map[string]bool)
	for _, observation := range observations {
		if observation.Supersedes != "" {
			superseded[observation.Supersedes] = true
		}
	}
	comparable := make([]Observation, 0, len(observations))
	for _, observation := range observations {
		if observation.CompetencyID != goal.CompetencyID || superseded[observation.ID] {
			continue
		}
		if observation.RubricReference != goal.RubricReference || observation.Status != "assessed" {
			continue
		}
		if _, placed := rank[observation.Band]; !placed {
			continue
		}
		comparable = append(comparable, observation)
	}
	return comparable
}

// latestOf picks the most recent reading, breaking a tie by identifier so
// the answer is stable rather than dependent on row order.
func latestOf(observations []Observation) (Observation, bool) {
	var latest Observation
	found := false
	for _, observation := range observations {
		if !found || observation.ObservedAt.After(latest.ObservedAt) ||
			(observation.ObservedAt.Equal(latest.ObservedAt) && observation.ID > latest.ID) {
			latest, found = observation, true
		}
	}
	return latest, found
}

// silenceReason says why a goal has no comparable reading, in readiness's
// vocabulary. The reasons are ordered from the most specific fact about
// this candidate to the least.
func silenceReason(goal Goal, rank map[string]int, observations []Observation) string {
	var sawUnassessed, sawForeignRubric, sawUnplaceableBand bool
	for _, observation := range observations {
		if observation.CompetencyID != goal.CompetencyID {
			continue
		}
		switch {
		case observation.RubricReference != goal.RubricReference:
			sawForeignRubric = true
		case observation.Status != "assessed":
			sawUnassessed = true
		default:
			if _, placed := rank[observation.Band]; !placed {
				sawUnplaceableBand = true
			}
		}
	}
	switch {
	case sawUnassessed:
		return ReasonNotAssessed
	case sawUnplaceableBand:
		return ReasonIncomparableBand
	case sawForeignRubric:
		return ReasonIncomparableRubric
	default:
		return ReasonNeverObserved
	}
}

// newMilestones finds the bands this goal's evidence reaches that have not
// been recorded yet.
//
// A band is a milestone once. Each is dated to the first reading that
// reached it rather than to the moment somebody looked, so a chart of
// milestones plots what the candidate did and not how often the job ran.
func newMilestones(goal Goal, rank map[string]int, comparable []Observation, earned []Milestone) []Milestone {
	already := make(map[string]bool, len(earned))
	for _, milestone := range earned {
		already[milestone.Band] = true
	}

	first := make(map[string]Observation)
	for _, observation := range comparable {
		held, seen := first[observation.Band]
		if !seen || observation.ObservedAt.Before(held.ObservedAt) {
			first[observation.Band] = observation
		}
	}

	fresh := make([]Milestone, 0, len(first))
	for band, observation := range first {
		if already[band] {
			continue
		}
		fresh = append(fresh, Milestone{
			GoalID: goal.ID, Band: band,
			RubricReference: observation.RubricReference,
			RubricVersion:   observation.RubricVersion,
			ObservationID:   observation.ID,
			ReachedAt:       observation.ObservedAt,
		})
	}
	sort.Slice(fresh, func(i, j int) bool {
		if fresh[i].ReachedAt.Equal(fresh[j].ReachedAt) {
			return rank[fresh[i].Band] < rank[fresh[j].Band]
		}
		return fresh[i].ReachedAt.Before(fresh[j].ReachedAt)
	})
	return fresh
}

// Cadence is how regularly somebody has been practising.
//
// Every field here counts something the candidate did. There is no missed
// count, no broken flag and no target, because a shape that cannot express
// a reproach cannot be rendered as one however the screen above it is
// written, and "must not become punitive gamification" is a property worth
// holding in the type rather than in a review comment.
type Cadence struct {
	// Run is the number of consecutive weeks practised, counting back from
	// the most recent one. The week in progress never ends a run.
	Run int

	// LongestRun is the best stretch ever managed, which is kept for good:
	// coming back after a break should find the best week still there.
	LongestRun int

	// WeeksPractised is every week with at least one session, ever.
	WeeksPractised int

	ActiveThisWeek  bool
	LastPractisedAt time.Time

	// Resting says the run has lapsed, and is the only word this type has
	// for that. It is false for somebody who has never practised, because
	// resting implies something to rest from and a first-run candidate is
	// owed neither a broken streak nor a consolation.
	Resting bool
}

// Practice reads a cadence out of the candidate's own history.
//
// Weekly rather than daily, and by week rather than by session, on
// purpose. A daily streak punishes an ordinary life, and counting sessions
// would reward cramming and make a lighter week look like a decline; a
// week with one honest session is the habit this is trying to encourage.
//
// The week in progress is never counted against anybody: a Monday morning
// with nothing done yet leaves last week's run standing, because a run is
// only over once a whole week has passed with nothing in it.
func Practice(observations []Observation, now time.Time) Cadence {
	weeks := make(map[time.Time]bool, len(observations))
	var cadence Cadence
	for _, observation := range observations {
		if observation.ObservedAt.IsZero() {
			continue
		}
		weeks[weekOf(observation.ObservedAt)] = true
		if observation.ObservedAt.After(cadence.LastPractisedAt) {
			cadence.LastPractisedAt = observation.ObservedAt
		}
	}
	if len(weeks) == 0 {
		return Cadence{}
	}
	cadence.WeeksPractised = len(weeks)

	ordered := make([]time.Time, 0, len(weeks))
	for week := range weeks {
		ordered = append(ordered, week)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Before(ordered[j]) })

	run := 0
	var previous time.Time
	for _, week := range ordered {
		if !previous.IsZero() && week.Equal(previous.AddDate(0, 0, 7)) {
			run++
		} else {
			run = 1
		}
		if run > cadence.LongestRun {
			cadence.LongestRun = run
		}
		previous = week
	}

	thisWeek := weekOf(now)
	cadence.ActiveThisWeek = weeks[thisWeek]
	lastPractisedWeek := ordered[len(ordered)-1]
	switch {
	case lastPractisedWeek.Equal(thisWeek) || lastPractisedWeek.Equal(thisWeek.AddDate(0, 0, -7)):
		cadence.Run = run
	default:
		cadence.Resting = true
	}
	return cadence
}

// weekOf is the Monday that starts the week a moment falls in, in UTC.
//
// One convention, applied everywhere, so that two candidates in two
// timezones are not told different things about the same day. UTC is the
// same choice the rest of the platform's timestamps already make.
func weekOf(at time.Time) time.Time {
	utc := at.UTC()
	weekday := (int(utc.Weekday()) + 6) % 7
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC).
		AddDate(0, 0, -weekday)
}
