package progression

import (
	"context"
	"fmt"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/progression/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Personal requirement storage: PRG-06's persisted half.
//
// Everything here is scoped to the person and never to a tenant, using the
// same asCandidate as goals, because migrations 0052 and 0053 give these
// tables no tenant column at all. That is the whole of "no personal
// requirement is reachable through employer authority": not a check
// somebody performs, but a scope in which the rows do not exist.

// CreateRequirement writes version 1 of a requirement and its criteria.
//
// The criteria are written in the same transaction as the requirement
// because a requirement with no criteria is selectable for a session it
// could never answer, and a half-written one is exactly that.
func (s *Store) CreateRequirement(ctx context.Context, owner Owner, requirement PersonalRequirement) error {
	if len(requirement.Criteria) == 0 {
		return fmt.Errorf("%w: a requirement with no criteria cannot be assessed",
			ErrRequirementIncoherent)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("progression: beginning the requirement write: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return err
	}
	q := db.New(tx)
	if err := q.InsertPersonalRequirement(ctx, db.InsertPersonalRequirementParams{
		ID: requirement.ID, CandidateID: owner.CandidateID,
		Intent: requirement.Intent, Status: requirement.Status,
		Version:   int32(requirement.Version),
		Reframing: requirement.Reframing, Prohibited: requirement.Prohibited,
	}); err != nil {
		return fmt.Errorf("progression: writing a requirement: %w", err)
	}
	if err := writeCriteria(ctx, q, owner, requirement); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReviseRequirement records the next version and its criteria.
//
// The earlier version's criteria are left exactly where they are. An
// outcome names a criterion version, so the only way a result from March
// can still be read in June is if June's edit added rows rather than
// changing them.
func (s *Store) ReviseRequirement(ctx context.Context, owner Owner, requirement PersonalRequirement) error {
	if requirement.Version < 2 {
		return fmt.Errorf("%w: a revision is version 2 or later", ErrRequirementIncoherent)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("progression: beginning the revision: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return err
	}
	q := db.New(tx)
	touched, err := q.ReviseRequirement(ctx, db.ReviseRequirementParams{
		ID: requirement.ID, Intent: requirement.Intent,
		Version:   int32(requirement.Version),
		Reframing: requirement.Reframing, Prohibited: requirement.Prohibited,
	})
	if err != nil {
		return fmt.Errorf("progression: revising a requirement: %w", err)
	}
	if touched == 0 {
		return fmt.Errorf("progression: no requirement %q belongs to this candidate", requirement.ID)
	}
	if err := writeCriteria(ctx, q, owner, requirement); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// writeCriteria stores one version's criteria in the order they resolved.
func writeCriteria(ctx context.Context, q *db.Queries, owner Owner, requirement PersonalRequirement) error {
	for position, criterion := range requirement.Criteria {
		if err := q.InsertRequirementCriterion(ctx, db.InsertRequirementCriterionParams{
			RequirementID: requirement.ID, CandidateID: owner.CandidateID,
			Version: int32(requirement.Version), CriterionID: criterion.ID,
			Position: int32(position), Statement: criterion.Statement,
			Observable: criterion.Observable,
		}); err != nil {
			return fmt.Errorf("progression: writing a criterion: %w", err)
		}
	}
	return nil
}

// SetRequirementStatus activates, pauses or retires a requirement.
func (s *Store) SetRequirementStatus(ctx context.Context, owner Owner, requirementID, status string) error {
	holder := PersonalRequirement{Status: RequirementDraft}
	if err := holder.MoveTo(status); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("progression: beginning the status change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return err
	}
	touched, err := db.New(tx).SetRequirementStatus(ctx, db.SetRequirementStatusParams{
		ID: requirementID, Status: status,
	})
	if err != nil {
		return fmt.Errorf("progression: setting a requirement's status: %w", err)
	}
	if touched == 0 {
		return fmt.Errorf("progression: no requirement %q belongs to this candidate", requirementID)
	}
	return tx.Commit(ctx)
}

// Requirements answers every requirement this candidate has written, each
// carrying the criteria of the version currently in use.
//
// The version in use rather than every version, because this is what a
// candidate selects for a session; the whole history of versions comes
// back through Export, where it is the point.
func (s *Store) Requirements(ctx context.Context, owner Owner) ([]PersonalRequirement, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: beginning the requirement read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return nil, err
	}
	q := db.New(tx)
	requirements, criteria, err := readRequirements(ctx, q)
	if err != nil {
		return nil, err
	}
	for index := range requirements {
		requirements[index].Criteria = criteria[versionKey{
			requirements[index].ID, requirements[index].Version,
		}]
	}
	return requirements, nil
}

// versionKey identifies one version of one requirement, which is the unit
// an outcome is reported against.
type versionKey struct {
	requirementID string
	version       int
}

// readRequirements reads the requirements and every version's criteria in
// one pass, so a caller can resolve an outcome from any version.
func readRequirements(ctx context.Context, q *db.Queries) (
	[]PersonalRequirement, map[versionKey][]Criterion, error) {

	rows, err := q.ListPersonalRequirements(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("progression: listing requirements: %w", err)
	}
	requirements := make([]PersonalRequirement, 0, len(rows))
	for _, row := range rows {
		requirements = append(requirements, PersonalRequirement{
			ID: row.ID, Intent: row.Intent, Status: row.Status,
			Version: int(row.Version), Reframing: row.Reframing,
			Prohibited: row.Prohibited, CreatedAt: row.CreatedAt,
		})
	}

	criterionRows, err := q.ListRequirementCriteria(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("progression: listing criteria: %w", err)
	}
	criteria := make(map[versionKey][]Criterion, len(criterionRows))
	for _, row := range criterionRows {
		key := versionKey{row.RequirementID, int(row.Version)}
		criteria[key] = append(criteria[key], Criterion{
			ID: row.CriterionID, Statement: row.Statement, Observable: row.Observable,
		})
	}
	return requirements, criteria, nil
}

// RecordOutcome stores one session's answer about one requirement.
//
// Idempotent per session and requirement, so a redelivered projection
// cannot count one session twice in a metric. The schema refuses a
// not-assessable row that blames anybody, which is the guard that matters:
// a bug here would show a candidate a failure for a question nobody asked.
func (s *Store) RecordOutcome(ctx context.Context, owner Owner, outcome RequirementOutcome) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("progression: beginning the outcome write: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return err
	}
	if err := db.New(tx).InsertRequirementOutcome(ctx, db.InsertRequirementOutcomeParams{
		ID: id.New().String(), RequirementID: outcome.RequirementID,
		CandidateID: owner.CandidateID, CriterionVersion: int32(outcome.CriterionVersion),
		SessionID: outcome.SessionID, RoleID: outcome.RoleID, ShapeID: outcome.ShapeID,
		Outcome: outcome.Outcome, Reason: outcome.Reason,
		Demonstrated: nonNil(outcome.Demonstrated), Missing: nonNil(outcome.Missing),
		Evidence: nonNil(outcome.Evidence), NextActions: nonNil(outcome.NextActions),
		ObservedAt: outcome.ObservedAt,
	}); err != nil {
		return fmt.Errorf("progression: writing a requirement outcome: %w", err)
	}
	return tx.Commit(ctx)
}

// nonNil turns a nil slice into an empty one, because the columns are NOT
// NULL and an absent list and an empty list mean the same thing here.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// Outcomes answers everything every session said about this candidate's
// requirements, oldest first.
func (s *Store) Outcomes(ctx context.Context, owner Owner) ([]RequirementOutcome, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: beginning the outcome read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return nil, err
	}
	return readOutcomes(ctx, db.New(tx))
}

func readOutcomes(ctx context.Context, q *db.Queries) ([]RequirementOutcome, error) {
	rows, err := q.ListRequirementOutcomes(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: listing requirement outcomes: %w", err)
	}
	outcomes := make([]RequirementOutcome, 0, len(rows))
	for _, row := range rows {
		outcomes = append(outcomes, RequirementOutcome{
			RequirementID: row.RequirementID, CriterionVersion: int(row.CriterionVersion),
			SessionID: row.SessionID, RoleID: row.RoleID, ShapeID: row.ShapeID,
			Outcome: row.Outcome, Reason: row.Reason,
			Demonstrated: row.Demonstrated, Missing: row.Missing,
			Evidence: row.Evidence, NextActions: row.NextActions,
			ObservedAt: row.ObservedAt,
		})
	}
	return outcomes, nil
}

// RecordSelfReport stores the candidate's own confidence rating.
//
// It goes into its own table and is never written beside an observation,
// which is what keeps "confidence is a self-rating and never an inference"
// true in the data rather than only in a policy document.
func (s *Store) RecordSelfReport(ctx context.Context, owner Owner, report SelfReport) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("progression: beginning the self-report write: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return err
	}
	if err := db.New(tx).InsertSelfReport(ctx, db.InsertSelfReportParams{
		CandidateID: owner.CandidateID, SessionID: report.SessionID,
		Phase: report.Phase, Rating: int16(report.Rating), ReportedAt: report.ReportedAt,
	}); err != nil {
		return fmt.Errorf("progression: writing a self-report: %w", err)
	}
	return tx.Commit(ctx)
}

// SetPersonalisation turns the use of prior evidence on or off.
//
// The candidate's switch, and the only one. Recommendations read it, so
// turning it off stops history shaping what gets asked next without also
// deleting the history, which are two different things a candidate might
// want and should not be forced to choose between.
func (s *Store) SetPersonalisation(ctx context.Context, owner Owner, enabled bool) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("progression: beginning the personalisation change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return err
	}
	if err := db.New(tx).SetPersonalisation(ctx, db.SetPersonalisationParams{
		CandidateID: owner.CandidateID, Enabled: enabled,
	}); err != nil {
		return fmt.Errorf("progression: setting personalisation: %w", err)
	}
	return tx.Commit(ctx)
}

// PrivateHistory is everything this context holds about one candidate's
// practice, in one value.
//
// One shape for inspecting and for exporting, deliberately. An export that
// went through a different path from the screen would eventually disagree
// with it, and the disagreement would be invisible to the person it was
// about.
type PrivateHistory struct {
	Requirements []PersonalRequirement

	// Criteria is every version's, keyed by requirement and version, so an
	// outcome can be read against exactly what judged it rather than
	// against whatever the requirement says today.
	Criteria map[string]map[int][]Criterion

	Outcomes     []RequirementOutcome
	SelfReports  []SelfReport
	Observations []Observation

	PersonalisationEnabled bool
}

// Export answers everything this context holds about the candidate.
//
// One transaction, so the export is a consistent moment rather than a
// series of reads that could disagree with each other, which matters
// because this is the artefact somebody uses to check what is held about
// them.
func (s *Store) Export(ctx context.Context, owner Owner) (PrivateHistory, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return PrivateHistory{}, fmt.Errorf("progression: beginning the export: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return PrivateHistory{}, err
	}
	q := db.New(tx)

	requirements, criteria, err := readRequirements(ctx, q)
	if err != nil {
		return PrivateHistory{}, err
	}
	history := PrivateHistory{
		Requirements: requirements,
		Criteria:     make(map[string]map[int][]Criterion, len(requirements)),
	}
	for key, list := range criteria {
		byVersion, seen := history.Criteria[key.requirementID]
		if !seen {
			byVersion = make(map[int][]Criterion)
			history.Criteria[key.requirementID] = byVersion
		}
		byVersion[key.version] = list
	}
	if history.Outcomes, err = readOutcomes(ctx, q); err != nil {
		return PrivateHistory{}, err
	}
	reportRows, err := q.ListSelfReports(ctx)
	if err != nil {
		return PrivateHistory{}, fmt.Errorf("progression: listing self-reports: %w", err)
	}
	for _, row := range reportRows {
		history.SelfReports = append(history.SelfReports, SelfReport{
			SessionID: row.SessionID, Phase: row.Phase,
			Rating: int(row.Rating), ReportedAt: row.ReportedAt,
		})
	}
	if history.Observations, err = historyOf(ctx, q); err != nil {
		return PrivateHistory{}, err
	}
	if history.PersonalisationEnabled, err = q.PersonalisationEnabled(ctx); err != nil {
		return PrivateHistory{}, fmt.Errorf("progression: reading personalisation: %w", err)
	}
	return history, nil
}

// EraseRequirement deletes one requirement, its criteria and every outcome
// recorded against it.
//
// Erasure rather than editing, and the cascade is the point: deleting a
// requirement while leaving the results behind would leave a candidate
// with outcomes about a thing they had asked to be forgotten. Practice
// requirement history is private to the candidate, no employer sees it and
// no decision rests on it, which is why erasure has nothing to weigh
// against here and why screening observations stay append-only.
func (s *Store) EraseRequirement(ctx context.Context, owner Owner, requirementID string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("progression: beginning the erasure: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return err
	}
	removed, err := db.New(tx).DeleteRequirement(ctx, requirementID)
	if err != nil {
		return fmt.Errorf("progression: erasing a requirement: %w", err)
	}
	if removed == 0 {
		return fmt.Errorf("progression: no requirement %q belongs to this candidate", requirementID)
	}
	return tx.Commit(ctx)
}
