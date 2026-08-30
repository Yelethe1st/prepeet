package progression

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"
)

// Readiness against a pinned role standard: PRG-02's pure half.
//
// The calculation is a deterministic function of one pinned standard and
// the candidate's observation history, which is what makes a stored
// readiness auditable: the same pin and the same history reproduce the
// same answer forever, whatever the registry currently publishes.
//
// Three rules shape everything below and each is structural rather than
// remembered. A readiness carries the standard's reference, version and
// digest, so a number can never be shown without what judged it. One
// standard produces one answer and no function here folds two together,
// because a single figure across incomparable roles is precisely the
// thing the ticket forbids. And a competency nobody measured is its own
// outcome with its own reason, never a zero and never a pass.

// ErrStandardIncoherent refuses a role standard that cannot be computed
// against: one that cannot name itself, or that judges nothing.
var ErrStandardIncoherent = errors.New("progression: the role standard is incoherent")

// ErrIncomparable refuses a set of standards that cannot stand beside
// each other, such as two claiming the same role. Ambiguity about which
// answer belongs to a role is how averaging starts.
var ErrIncomparable = errors.New("progression: these standards are not comparable")

// The three outcomes a requirement can have. Unassessed is deliberately
// its own value rather than a band of zero, because "we did not measure
// this" and "this was weak" are different facts about a candidate and
// collapsing them is the failure PRG-02 exists to prevent.
const (
	OutcomeMet        = "met"
	OutcomeBelow      = "below"
	OutcomeUnassessed = "unassessed"
)

// Why a requirement is unassessed. The reason is part of the answer: a
// competency that has never come up needs a different next session from
// one measured under a rubric this standard cannot be compared against.
const (
	ReasonNeverObserved      = "never_observed"
	ReasonNotAssessed        = "not_assessed"
	ReasonIncomparableRubric = "incomparable_rubric"
	ReasonIncomparableBand   = "incomparable_band"
)

// Pin names the exact published artifact a readiness was computed
// against. It travels with the answer rather than beside it, so a stored
// or rendered readiness can always say what judged it.
type Pin struct {
	Reference string
	Version   string
	Digest    string
}

// Requirement is one competency the standard expects, and the band that
// meets it.
type Requirement struct {
	CompetencyID string `json:"competency_id"`
	TargetBand   string `json:"target_band"`
}

// Standard is one parsed, coherent role standard.
//
// It carries its own band scale rather than borrowing one, because what
// counts as better is the pinned artifact's statement and not this
// package's assumption. RubricReference is the comparability basis: a
// reading produced by a different rubric measures a different thing and
// is reported as incomparable rather than counted.
type Standard struct {
	Pin

	Role            string        `json:"role"`
	Discipline      string        `json:"discipline"`
	RubricReference string        `json:"rubric_reference"`
	Bands           []string      `json:"bands"`
	Requirements    []Requirement `json:"requirements"`
}

// CompetencyReadiness is one requirement's answer, and the reading that
// produced it.
type CompetencyReadiness struct {
	CompetencyID string
	TargetBand   string

	// Outcome is met, below or unassessed. ObservedBand and ObservationID
	// are empty exactly when the outcome is unassessed, and Reason is set
	// exactly then, so the two states cannot be confused by a reader.
	Outcome      string
	ObservedBand string
	Reason       string

	ObservationID string
	ObservedAt    time.Time
}

// Readiness is one candidate's standing against one role standard.
//
// There is no overall figure and no percentage. Met, Below and Unassessed
// partition the requirements, which lets a screen show the unknown as
// its own quantity instead of hiding it inside a score.
type Readiness struct {
	Standard        Pin
	Role            string
	Discipline      string
	RubricReference string

	Competencies []CompetencyReadiness

	Met        int
	Below      int
	Unassessed int

	ComputedAt time.Time
}

// ParseStandard decodes a pinned role standard document.
//
// The pin is checked first and hardest. A standard that cannot name its
// reference, version and digest could still produce numbers, and those
// numbers would be unauditable, so the refusal happens here rather than
// being left to whoever renders the result.
func ParseStandard(pin Pin, body []byte) (Standard, error) {
	var standard Standard
	if err := json.Unmarshal(body, &standard); err != nil {
		return Standard{}, fmt.Errorf("%w: %v", ErrStandardIncoherent, err)
	}
	standard.Pin = pin
	if err := standard.validate(); err != nil {
		return Standard{}, err
	}
	return standard, nil
}

// validate states what a usable standard is. Called by ParseStandard and
// again by Compute, because a Standard can be built by hand and the
// guarantee has to hold for the value rather than for one constructor.
func (s Standard) validate() error {
	if s.Reference == "" || s.Version == "" || s.Digest == "" {
		return fmt.Errorf("%w: a readiness must name the standard, version and digest it was computed against", ErrStandardIncoherent)
	}
	if s.Role == "" || s.Discipline == "" {
		return fmt.Errorf("%w: readiness is grouped by role and discipline, so both must be named", ErrStandardIncoherent)
	}
	if s.RubricReference == "" {
		return fmt.Errorf("%w: without a rubric reference nothing states which readings are comparable", ErrStandardIncoherent)
	}
	if len(s.Bands) == 0 {
		return fmt.Errorf("%w: a standard with no band scale cannot say what meets it", ErrStandardIncoherent)
	}
	seenBand := make(map[string]bool, len(s.Bands))
	for _, band := range s.Bands {
		if band == "" {
			return fmt.Errorf("%w: a nameless band", ErrStandardIncoherent)
		}
		if seenBand[band] {
			return fmt.Errorf("%w: band %q appears twice, so the scale has no order", ErrStandardIncoherent, band)
		}
		seenBand[band] = true
	}
	if len(s.Requirements) == 0 {
		return fmt.Errorf("%w: a standard with no requirements measures nothing", ErrStandardIncoherent)
	}
	seenCompetency := make(map[string]bool, len(s.Requirements))
	for _, requirement := range s.Requirements {
		if requirement.CompetencyID == "" {
			return fmt.Errorf("%w: a nameless competency", ErrStandardIncoherent)
		}
		if seenCompetency[requirement.CompetencyID] {
			return fmt.Errorf("%w: competency %q is required twice", ErrStandardIncoherent, requirement.CompetencyID)
		}
		seenCompetency[requirement.CompetencyID] = true
		if !seenBand[requirement.TargetBand] {
			return fmt.Errorf("%w: competency %q targets band %q, which is not on the scale",
				ErrStandardIncoherent, requirement.CompetencyID, requirement.TargetBand)
		}
	}
	return nil
}

// Compute answers one candidate's readiness against one pinned standard.
//
// One standard, deliberately. The signature is the guard for the rule
// that incomparable roles are never averaged: there is no way to ask this
// package for a figure that spans two standards, because the only entry
// points take exactly one or return exactly one answer per standard.
func Compute(standard Standard, observations []Observation, at time.Time) (Readiness, error) {
	if err := standard.validate(); err != nil {
		return Readiness{}, err
	}

	rank := make(map[string]int, len(standard.Bands))
	for index, band := range standard.Bands {
		rank[band] = index
	}

	// A correction supersedes rather than replaces (PRG-01), so both rows
	// are in history. Reading the superseded one as current would make
	// readiness disagree with the corrected record.
	superseded := make(map[string]bool)
	for _, observation := range observations {
		if observation.Supersedes != "" {
			superseded[observation.Supersedes] = true
		}
	}

	readiness := Readiness{
		Standard:        standard.Pin,
		Role:            standard.Role,
		Discipline:      standard.Discipline,
		RubricReference: standard.RubricReference,
		Competencies:    make([]CompetencyReadiness, 0, len(standard.Requirements)),
		ComputedAt:      at,
	}

	// Sorted rather than in document order, so the stored answer and its
	// rendering are stable whoever authored the standard.
	requirements := append([]Requirement(nil), standard.Requirements...)
	sort.Slice(requirements, func(i, j int) bool {
		return requirements[i].CompetencyID < requirements[j].CompetencyID
	})

	for _, requirement := range requirements {
		readiness.Competencies = append(readiness.Competencies,
			resolve(requirement, standard, rank, superseded, observations))
	}
	readiness.recount()
	return readiness, nil
}

// recount derives the three totals from the requirements themselves.
//
// One implementation, used when a readiness is computed and again when one
// is read back, because a stored total is a number that can quietly stop
// agreeing with the rows it claims to summarise - and the disagreement
// that would matter here is precisely the invisible one, an unmeasured
// competency counted as a pass.
func (r *Readiness) recount() {
	r.Met, r.Below, r.Unassessed = 0, 0, 0
	for _, competency := range r.Competencies {
		switch competency.Outcome {
		case OutcomeMet:
			r.Met++
		case OutcomeBelow:
			r.Below++
		default:
			r.Unassessed++
		}
	}
}

// resolve answers one requirement from the history.
//
// It returns the latest comparable assessed reading, or an unassessed
// outcome naming why there is none. The reasons are ordered from the most
// specific fact about this candidate to the least: history that exists but
// was not assessed says more than history that cannot be compared.
func resolve(requirement Requirement, standard Standard, rank map[string]int,
	superseded map[string]bool, observations []Observation) CompetencyReadiness {

	answer := CompetencyReadiness{
		CompetencyID: requirement.CompetencyID,
		TargetBand:   requirement.TargetBand,
	}

	var (
		latest             Observation
		found              bool
		sawUnassessed      bool
		sawForeignRubric   bool
		sawUnplaceableBand bool
	)
	for _, observation := range observations {
		if observation.CompetencyID != requirement.CompetencyID || superseded[observation.ID] {
			continue
		}
		if observation.RubricReference != standard.RubricReference {
			sawForeignRubric = true
			continue
		}
		if observation.Status != "assessed" {
			sawUnassessed = true
			continue
		}
		if _, placed := rank[observation.Band]; !placed {
			sawUnplaceableBand = true
			continue
		}
		if !found || observation.ObservedAt.After(latest.ObservedAt) ||
			(observation.ObservedAt.Equal(latest.ObservedAt) && observation.ID > latest.ID) {
			latest, found = observation, true
		}
	}

	if !found {
		answer.Outcome = OutcomeUnassessed
		switch {
		case sawUnassessed:
			answer.Reason = ReasonNotAssessed
		case sawUnplaceableBand:
			answer.Reason = ReasonIncomparableBand
		case sawForeignRubric:
			answer.Reason = ReasonIncomparableRubric
		default:
			answer.Reason = ReasonNeverObserved
		}
		return answer
	}

	answer.ObservedBand = latest.Band
	answer.ObservationID = latest.ID
	answer.ObservedAt = latest.ObservedAt
	if rank[latest.Band] >= rank[requirement.TargetBand] {
		answer.Outcome = OutcomeMet
	} else {
		answer.Outcome = OutcomeBelow
	}
	return answer
}

// ComputeAll answers one readiness per standard, grouped by discipline
// then role.
//
// It returns a list and never a total. Two roles are two answers here and
// at every layer above, which is the ticket's second rule made structural:
// there is nothing to average because no combined value is ever produced.
func ComputeAll(standards []Standard, observations []Observation, at time.Time) ([]Readiness, error) {
	byRole := make(map[string]bool, len(standards))
	byReference := make(map[string]bool, len(standards))
	readinesses := make([]Readiness, 0, len(standards))
	for _, standard := range standards {
		if byRole[standard.Role] {
			return nil, fmt.Errorf("%w: two standards claim role %q, so neither answer is the role's",
				ErrIncomparable, standard.Role)
		}
		if byReference[standard.Reference] {
			return nil, fmt.Errorf("%w: standard %q appears twice", ErrIncomparable, standard.Reference)
		}
		byRole[standard.Role], byReference[standard.Reference] = true, true

		readiness, err := Compute(standard, observations, at)
		if err != nil {
			return nil, err
		}
		readinesses = append(readinesses, readiness)
	}

	sort.Slice(readinesses, func(i, j int) bool {
		if readinesses[i].Discipline != readinesses[j].Discipline {
			return readinesses[i].Discipline < readinesses[j].Discipline
		}
		return readinesses[i].Role < readinesses[j].Role
	})
	return readinesses, nil
}
