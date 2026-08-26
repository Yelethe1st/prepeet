// Package progression owns the candidate's competency history: append-only
// observations with full rubric provenance, projected from what evaluation
// published.
//
// Implements PRG-01. The properties everything here serves: nothing is
// ever updated in place - a correction is a new row naming what it
// supersedes; every row carries exactly what judged it, so any historical
// point reconstructs against its own rubric; and a re-evaluation under a
// new rubric adds its own reading while the earlier view stands. The
// consumer's retry converges on the unique (evaluation, competency) row.
package progression

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/progression/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Observation is one competency reading from one evaluation.
type Observation struct {
	ID           string
	SessionID    string
	EvaluationID string
	CompetencyID string

	Status        string
	Band          string
	Confidence    string
	EvidenceCount int
	Supporting    int
	Contradictory int
	Unverified    int
	Gaps          int

	RubricReference    string
	RubricVersion      string
	RubricDigest       string
	AggregationVersion string
	ExtractionVersion  string
	ModelVersion       string
	PolicyVersion      string

	// Supersedes names the earlier observation a correction replaces;
	// empty for a first reading. The earlier row stands either way.
	Supersedes string

	ObservedAt time.Time
}

// Owner names whose history rows belong to.
type Owner struct {
	Mode        string
	CandidateID string
	TenantID    string
}

// Store persists observations.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wires the store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// scope sets the row-level security context: practice acts as the
// candidate, screening under the tenant.
func scope(ctx context.Context, tx pgx.Tx, owner Owner) error {
	if owner.Mode == "practice" {
		return database.SetUser(ctx, tx, owner.CandidateID)
	}
	return database.SetTenant(ctx, tx, owner.TenantID)
}

// Append writes one evaluation's observations, idempotently: rows that
// already exist for the (evaluation, competency) pair are left standing,
// so a redelivered event converges instead of duplicating history.
func (s *Store) Append(ctx context.Context, owner Owner, observations []Observation) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("progression: beginning append: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, owner); err != nil {
		return err
	}
	q := db.New(tx)
	for _, observation := range observations {
		if _, err := q.InsertObservation(ctx, db.InsertObservationParams{
			ID:          id.New().String(),
			CandidateID: owner.CandidateID, Mode: owner.Mode, TenantID: owner.TenantID,
			SessionID: observation.SessionID, EvaluationID: observation.EvaluationID,
			CompetencyID: observation.CompetencyID,
			Status:       observation.Status, Band: observation.Band,
			Confidence:         observation.Confidence,
			EvidenceCount:      int32(observation.EvidenceCount),
			Supporting:         int32(observation.Supporting),
			Contradictory:      int32(observation.Contradictory),
			Unverified:         int32(observation.Unverified),
			Gaps:               int32(observation.Gaps),
			RubricReference:    observation.RubricReference,
			RubricVersion:      observation.RubricVersion,
			RubricDigest:       observation.RubricDigest,
			AggregationVersion: observation.AggregationVersion,
			ExtractionVersion:  observation.ExtractionVersion,
			ModelVersion:       observation.ModelVersion,
			PolicyVersion:      observation.PolicyVersion,
			Supersedes:         observation.Supersedes,
			ObservedAt:         observation.ObservedAt,
		}); err != nil {
			return fmt.Errorf("progression: appending an observation: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// History answers the owner's whole observation history, oldest first,
// every version of every reading included.
func (s *Store) History(ctx context.Context, owner Owner) ([]Observation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: beginning history: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, owner); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).ListObservations(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: listing history: %w", err)
	}
	observations := make([]Observation, 0, len(rows))
	for _, row := range rows {
		observations = append(observations, Observation{
			ID: row.ID, SessionID: row.SessionID, EvaluationID: row.EvaluationID,
			CompetencyID: row.CompetencyID,
			Status:       row.Status, Band: row.Band, Confidence: row.Confidence,
			EvidenceCount: int(row.EvidenceCount),
			Supporting:    int(row.Supporting), Contradictory: int(row.Contradictory),
			Unverified: int(row.Unverified), Gaps: int(row.Gaps),
			RubricReference: row.RubricReference, RubricVersion: row.RubricVersion,
			RubricDigest:       row.RubricDigest,
			AggregationVersion: row.AggregationVersion,
			ExtractionVersion:  row.ExtractionVersion,
			ModelVersion:       row.ModelVersion, PolicyVersion: row.PolicyVersion,
			Supersedes: row.Supersedes,
			ObservedAt: row.ObservedAt,
		})
	}
	return observations, nil
}
