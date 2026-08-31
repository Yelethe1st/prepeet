package progression

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Using prior gaps to shape the next session: PRG-05.
//
// The temptation here is a ranking function that sorts competencies by
// weakness and fills the session from the top. That is exactly the narrow
// loop the ticket forbids: a candidate weak at five things would be asked
// about the same five until they were not, never revisiting what they were
// once good at and never being surprised. So the reservation below is not
// a tuning parameter but the shape of the answer - at least one slot is
// always something the history did not ask for.
//
// Three other rules follow from the rest of this context rather than from
// this ticket. A competency nobody has been asked about is a reason to ask
// and never a finding, so it is targeted without being given a band. A
// reading old enough to be stale is a claim about a person who has
// practised since, so it earns a revisit rather than a pass. And history
// under another rubric measures something else, so it makes the
// recommendation cautious instead of making it confident about the wrong
// thing.

// ErrUntargetable refuses a request that cannot produce an honest
// recommendation: one with no role, or a role the catalogue can name
// nothing to ask about for.
var ErrUntargetable = errors.New("progression: this cannot be targeted")

// Why a competency was chosen for the next session.
//
// Three reasons and not one score, because they ask for different words on
// a screen and different behaviour from composition: a gap to work on, a
// blank to fill, and a claim old enough to want checking.
const (
	TargetBelowBand     = "below_band"
	TargetNeverObserved = "never_observed"
	TargetStaleEvidence = "stale_evidence"
)

// reservedSlots is how much of a session targeting may never take.
//
// One slot, always, however weak the history. It is the difference between
// a session shaped by what somebody struggled with and a session that is
// only ever about that, and it also protects the case nobody tests for: a
// competency that has quietly regressed since the last time it was strong
// will never be asked about again if only gaps are asked about.
const reservedSlots = 1

// RoleCompetencies is what targeting needs to know from the catalogue: the
// competencies a session for a role could ask about at all.
//
// Declared here and implemented in cmd, per ADR-0005. Progression must not
// import the catalogue context, and the narrow shape is deliberate: this
// package has no business knowing what else a role document contains.
type RoleCompetencies interface {
	// Competencies answers everything a session for this role may cover.
	Competencies(ctx context.Context, roleID string) ([]string, error)
}

// TargetingRequest is the session about to be composed.
//
// Bands and RubricReference are the comparability basis, passed in rather
// than assumed, so that a recommendation is made against the same scale
// the session will be judged on and never against this package's guess.
type TargetingRequest struct {
	RoleID          string
	RubricReference string
	Bands           []string

	// Slots is how many competencies the session has room for. Composition
	// owns the real number; targeting only says which ones.
	Slots int
}

// Target is one competency the next session should cover, and why.
type Target struct {
	CompetencyID string
	Reason       string

	// ObservedBand and ObservationID are set only for a competency that was
	// actually read, so a blank in the history can never be rendered as a
	// weak result.
	ObservedBand  string
	ObservationID string
	ObservedAt    time.Time
	Evidence      Evidence

	// Because is the sentence a candidate reads. Held beside the machine
	// readable reason rather than derived by each caller, so two screens
	// cannot explain the same recommendation two different ways.
	Because string
}

// Recommendation is what the next session should cover.
//
// Covers is the whole session and Targeted is the part the history asked
// for; the difference between them is the breadth the ticket requires, and
// keeping them as two lists is what makes "targeting is not the whole
// session" a thing a test can check rather than a promise.
type Recommendation struct {
	RoleID          string
	RubricReference string

	Covers   []string
	Targeted []Target

	// Cautious says the history was too thin, too old or too incomparable
	// to support a confident reading, and Caution says which. A cautious
	// recommendation still composes a session: caution must not become
	// silence for the candidate who most needs a first one.
	Cautious bool
	Caution  string
}

// Targeting turns a candidate's history into a recommendation for their
// next session.
type Targeting struct {
	catalogue RoleCompetencies
}

// NewTargeting wires targeting to the catalogue port.
func NewTargeting(catalogue RoleCompetencies) *Targeting {
	return &Targeting{catalogue: catalogue}
}

// Recommend answers what the next session should cover, and why.
//
// The order is deliberate: the catalogue decides what could be asked, the
// history decides what is worth asking, and the reservation decides how
// much of the session the history is allowed to claim. Reversing the first
// two would let a history of a competency the role no longer has shape a
// session for a role that cannot ask about it.
func (t *Targeting) Recommend(ctx context.Context, request TargetingRequest,
	observations []Observation, at time.Time) (Recommendation, error) {

	if request.RoleID == "" {
		return Recommendation{}, fmt.Errorf(
			"%w: a recommendation is about one role, and none was named", ErrUntargetable)
	}
	if request.RubricReference == "" || len(request.Bands) == 0 {
		return Recommendation{}, fmt.Errorf(
			"%w: without a rubric reference and a band scale nothing states which readings count",
			ErrUntargetable)
	}
	competencies, err := t.catalogue.Competencies(ctx, request.RoleID)
	if err != nil {
		return Recommendation{}, fmt.Errorf("progression: asking the catalogue: %w", err)
	}
	if len(competencies) == 0 {
		return Recommendation{}, fmt.Errorf(
			"%w: the catalogue names nothing a session for %q could ask about",
			ErrUntargetable, request.RoleID)
	}

	rank := make(map[string]int, len(request.Bands))
	for index, band := range request.Bands {
		rank[band] = index
	}
	latest, comparable := latestPerCompetency(request, rank, observations)

	candidates := make([]Target, 0, len(competencies))
	for _, competency := range competencies {
		if target, wanted := assess(competency, latest[competency], rank, request, at); wanted {
			candidates = append(candidates, target)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return targetPriority(candidates[i], rank) < targetPriority(candidates[j], rank)
	})

	recommendation := Recommendation{
		RoleID:          request.RoleID,
		RubricReference: request.RubricReference,
	}
	recommendation.Cautious, recommendation.Caution = caution(comparable, len(competencies), observations)

	slots := request.Slots
	if slots > len(competencies) {
		slots = len(competencies)
	}
	room := slots - reservedSlots
	if room < 0 {
		room = 0
	}
	if len(candidates) > room {
		candidates = candidates[:room]
	}
	recommendation.Targeted = candidates

	chosen := make(map[string]bool, slots)
	for _, target := range candidates {
		chosen[target.CompetencyID] = true
		recommendation.Covers = append(recommendation.Covers, target.CompetencyID)
	}
	// The reserved remainder, filled from the catalogue in its own order so
	// that what is not a gap is still asked about, and asked about
	// predictably rather than at random.
	for _, competency := range competencies {
		if len(recommendation.Covers) >= slots {
			break
		}
		if chosen[competency] {
			continue
		}
		recommendation.Covers = append(recommendation.Covers, competency)
	}
	return recommendation, nil
}

// latestPerCompetency reduces the history to the latest comparable reading
// of each competency, and says how many there were.
//
// Comparable means this rubric reference, assessed, and a band the scale
// can place, which is the same test readiness and goals apply. The count is
// what caution is judged on: it is the amount of usable evidence, not the
// number of rows.
func latestPerCompetency(request TargetingRequest, rank map[string]int,
	observations []Observation) (map[string]Observation, int) {

	superseded := make(map[string]bool)
	for _, observation := range observations {
		if observation.Supersedes != "" {
			superseded[observation.Supersedes] = true
		}
	}
	latest := make(map[string]Observation, len(observations))
	comparable := 0
	for _, observation := range observations {
		if superseded[observation.ID] ||
			observation.RubricReference != request.RubricReference ||
			observation.Status != "assessed" {
			continue
		}
		if _, placed := rank[observation.Band]; !placed {
			continue
		}
		comparable++
		held, seen := latest[observation.CompetencyID]
		if !seen || observation.ObservedAt.After(held.ObservedAt) ||
			(observation.ObservedAt.Equal(held.ObservedAt) && observation.ID > held.ID) {
			latest[observation.CompetencyID] = observation
		}
	}
	return latest, comparable
}

// assess decides whether one competency is worth targeting, and says why
// in the same sentence a candidate will read.
//
// The band that counts as a gap is the top of the scale short of the best
// band, which is deliberately generous: the purpose is to choose what to
// practise, not to grade, and a scale's second band from the top is still
// somewhere a session can usefully go.
func assess(competency string, reading Observation, rank map[string]int,
	request TargetingRequest, at time.Time) (Target, bool) {

	target := Target{CompetencyID: competency}
	if reading.ID == "" {
		target.Reason = TargetNeverObserved
		target.Evidence = Freshness(time.Time{}, at)
		target.Because = "No session has asked about " + competency + " yet, " +
			"so there is nothing to say about it either way."
		return target, true
	}

	target.ObservedBand = reading.Band
	target.ObservationID = reading.ID
	target.ObservedAt = reading.ObservedAt
	target.Evidence = Freshness(reading.ObservedAt, at)

	best := len(request.Bands) - 1
	switch {
	case rank[reading.Band] < best:
		target.Reason = TargetBelowBand
		target.Because = fmt.Sprintf(
			"The last reading of %s was %s, below the top of this rubric's scale.",
			competency, reading.Band)
		return target, true
	case target.Evidence.Standing == EvidenceStale:
		target.Reason = TargetStaleEvidence
		target.Because = fmt.Sprintf(
			"The last reading of %s was %s, but it is %d days old and worth checking again.",
			competency, reading.Band, target.Evidence.AgeDays)
		return target, true
	}
	return Target{}, false
}

// targetPriority orders the candidates for the slots there are.
//
// Never observed first, because a blank is the largest thing not known
// about somebody; then weak readings, weakest first; then stale ones. A
// competency nobody has been asked about outranks a weak one deliberately:
// the purpose is to reduce what is unknown, not to drill what is already
// measured.
func targetPriority(target Target, rank map[string]int) int {
	switch target.Reason {
	case TargetNeverObserved:
		return 0
	case TargetBelowBand:
		return 10 + rank[target.ObservedBand]
	default:
		return 100
	}
}

// caution decides whether the recommendation should say out loud that it
// is working from very little.
//
// The three shapes worth naming separately, because they ask for different
// words: nothing at all, too little to be a pattern, and a history that
// exists but measures something this session cannot be compared against.
// Sparse is one comparable reading per competency or fewer, which is the
// point below which "improving" and "had a good day" are the same data.
func caution(comparable, competencies int, observations []Observation) (bool, string) {
	if comparable == 0 {
		if len(observations) > 0 {
			return true, "Every previous reading was produced under a different rubric, " +
				"so none of them can be compared with this session. This recommendation is " +
				"based on what has not been asked rather than on a trend."
		}
		return true, "There is no practice history yet, so this session covers the role " +
			"broadly rather than targeting anything."
	}
	if comparable <= competencies {
		return true, "There are only a few comparable readings so far. This is a suggestion " +
			"about what to try next, not a trend."
	}
	return false, ""
}
