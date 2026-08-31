package progression

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/progression/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

// Goal storage: PRG-03's persisted half.
//
// Everything here runs under the candidate's own scope and never a
// tenant's, which is not a convention but the only scope in which a goal
// row exists at all: migration 0051 gives progression.goals no tenant
// column and one policy keyed to the person. asCandidate below is
// therefore the only scoping function in this file, deliberately, so that
// there is no call anybody could add that would open a goal to an
// employer without also changing the schema.

// asCandidate sets the row-level security context to the person.
//
// Only the user is set, and that is enough rather than merely convenient:
// SetUser uses SET LOCAL, so no value survives the transaction that set it,
// and a transaction that never sets a tenant is a transaction in which
// app.tenant_id is empty however the connection was used a moment ago.
// That is what makes the goal policy's "and no tenant context" clause
// something the application cannot accidentally satisfy for an employer.
func asCandidate(ctx context.Context, tx pgx.Tx, owner Owner) error {
	return database.SetUser(ctx, tx, owner.CandidateID)
}

// CreateGoal records one target the candidate set.
//
// Validated before it is written even though the schema checks the same
// things, because a refusal from Go names the goal's own vocabulary and a
// refusal from a CHECK constraint names a column. Both run; the candidate
// reads the first.
func (s *Store) CreateGoal(ctx context.Context, owner Owner, goal Goal) error {
	if err := goal.Validate(); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("progression: beginning the goal write: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return err
	}
	if err := db.New(tx).InsertGoal(ctx, db.InsertGoalParams{
		ID: goal.ID, CandidateID: owner.CandidateID,
		Origin: goal.Origin, OriginReference: goal.OriginReference,
		CompetencyID: goal.CompetencyID, TargetBand: goal.TargetBand,
		RubricReference: goal.RubricReference, Bands: goal.Bands,
		Status: goal.Status,
	}); err != nil {
		return fmt.Errorf("progression: writing a goal: %w", err)
	}
	return tx.Commit(ctx)
}

// SetGoalStatus pauses, resumes or retires a goal.
//
// The only edit a goal admits. Changing what a goal measures would re-date
// every milestone already earned under the old target, so the database
// refuses it and there is no call here that would try.
func (s *Store) SetGoalStatus(ctx context.Context, owner Owner, goalID, status string) error {
	switch status {
	case GoalActive, GoalPaused, GoalRetired:
	default:
		return fmt.Errorf("%w: %q is not a goal lifecycle state", ErrGoalIncoherent, status)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("progression: beginning the goal update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return err
	}
	touched, err := db.New(tx).SetGoalStatus(ctx, db.SetGoalStatusParams{ID: goalID, Status: status})
	if err != nil {
		return fmt.Errorf("progression: setting a goal's status: %w", err)
	}
	if touched == 0 {
		return fmt.Errorf("progression: no goal %q belongs to this candidate", goalID)
	}
	return tx.Commit(ctx)
}

// Goals answers every goal this candidate has set, oldest first, retired
// ones included.
//
// Retired goals are returned rather than filtered because they are part of
// the record of what somebody worked on, and a screen that hid them would
// make a year of practice look like whatever is currently open.
func (s *Store) Goals(ctx context.Context, owner Owner) ([]Goal, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: beginning the goal read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return nil, err
	}
	return readGoals(ctx, db.New(tx))
}

// readGoals is the shared read, used by Goals and again by TrackGoals so
// the two can never disagree about what a goal is.
func readGoals(ctx context.Context, q *db.Queries) ([]Goal, error) {
	rows, err := q.ListGoals(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: listing goals: %w", err)
	}
	goals := make([]Goal, 0, len(rows))
	for _, row := range rows {
		goals = append(goals, Goal{
			ID: row.ID, Origin: row.Origin, OriginReference: row.OriginReference,
			CompetencyID: row.CompetencyID, TargetBand: row.TargetBand,
			RubricReference: row.RubricReference, Bands: row.Bands,
			Status: row.Status, CreatedAt: row.CreatedAt,
		})
	}
	return goals, nil
}

// TrackGoals answers every goal's progress, recording any milestone the
// evidence has newly earned.
//
// One transaction, deliberately: the milestones written are exactly the
// ones the returned progress claims, so a caller cannot show a candidate
// an achievement that failed to persist. Safe to run as often as anybody
// likes, because a band is a milestone once and the insert says so.
func (s *Store) TrackGoals(ctx context.Context, owner Owner, at time.Time) ([]GoalProgress, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: beginning the goal tracking: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return nil, err
	}
	q := db.New(tx)

	goals, err := readGoals(ctx, q)
	if err != nil {
		return nil, err
	}
	observations, err := historyOf(ctx, q)
	if err != nil {
		return nil, err
	}
	milestoneRows, err := q.ListMilestones(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: listing milestones: %w", err)
	}
	earned := make(map[string][]Milestone, len(goals))
	for _, row := range milestoneRows {
		earned[row.GoalID] = append(earned[row.GoalID], Milestone{
			GoalID: row.GoalID, Band: row.Band,
			RubricReference: row.RubricReference, RubricVersion: row.RubricVersion,
			ObservationID: row.ObservationID, ReachedAt: row.ReachedAt,
		})
	}

	tracked := make([]GoalProgress, 0, len(goals))
	for _, goal := range goals {
		progress := TrackGoal(goal, observations, earned[goal.ID], at)
		for _, milestone := range progress.Fresh {
			if err := q.InsertMilestone(ctx, db.InsertMilestoneParams{
				GoalID: goal.ID, CandidateID: owner.CandidateID,
				Band:            milestone.Band,
				RubricReference: milestone.RubricReference,
				RubricVersion:   milestone.RubricVersion,
				ObservationID:   milestone.ObservationID,
				ReachedAt:       milestone.ReachedAt,
			}); err != nil {
				return nil, fmt.Errorf("progression: recording a milestone: %w", err)
			}
		}
		tracked = append(tracked, progress)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("progression: committing the goal tracking: %w", err)
	}
	return tracked, nil
}

// PracticeCadence answers how regularly this candidate has been
// practising, from their own observation history.
//
// Derived rather than stored, which is the whole reason there is no
// cadence table. A stored streak is a number somebody has to decide when
// to decrement, and every rule for decrementing it is a rule about
// punishing a quiet week.
func (s *Store) PracticeCadence(ctx context.Context, owner Owner, at time.Time) (Cadence, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Cadence{}, fmt.Errorf("progression: beginning the cadence read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := asCandidate(ctx, tx, owner); err != nil {
		return Cadence{}, err
	}
	observations, err := historyOf(ctx, db.New(tx))
	if err != nil {
		return Cadence{}, err
	}
	return Practice(observations, at), nil
}

// historyOf reads the observation history inside a transaction that is
// already scoped, so goal tracking and cadence read exactly what History
// reads without opening a second transaction that could see a different
// moment.
func historyOf(ctx context.Context, q *db.Queries) ([]Observation, error) {
	rows, err := q.ListObservations(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: listing history: %w", err)
	}
	observations := make([]Observation, 0, len(rows))
	for _, row := range rows {
		observations = append(observations, Observation{
			ID: row.ID, SessionID: row.SessionID, EvaluationID: row.EvaluationID,
			CompetencyID: row.CompetencyID, Status: row.Status, Band: row.Band,
			Confidence: row.Confidence, EvidenceCount: int(row.EvidenceCount),
			Supporting: int(row.Supporting), Contradictory: int(row.Contradictory),
			Unverified: int(row.Unverified), Gaps: int(row.Gaps),
			RubricReference: row.RubricReference, RubricVersion: row.RubricVersion,
			RubricDigest:       row.RubricDigest,
			AggregationVersion: row.AggregationVersion,
			ExtractionVersion:  row.ExtractionVersion,
			ModelVersion:       row.ModelVersion, PolicyVersion: row.PolicyVersion,
			Supersedes: row.Supersedes, ObservedAt: row.ObservedAt,
		})
	}
	return observations, nil
}
