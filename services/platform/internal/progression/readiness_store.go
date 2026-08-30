package progression

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/progression/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Readiness storage: PRG-02's persisted half.
//
// A snapshot is kept rather than recomputed on every read for one reason
// that matters more than speed. The standard a readiness was computed
// against can be deprecated or rolled back, and the history it read can be
// corrected; a stored snapshot names the pin it used, so what a candidate
// was shown on a date stays answerable. Recomputation from the same pin
// and history reproduces it exactly.

// SaveReadiness stores one readiness against one pinned standard.
//
// Idempotent by the answer rather than by the moment: an unchanged answer
// converges on the snapshot already written, and a changed one appends
// beside it. Nothing is ever edited, so a chart of readiness over time is
// a record of what changed rather than of how often it was recomputed.
func (s *Store) SaveReadiness(ctx context.Context, owner Owner, readiness Readiness) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("progression: beginning the readiness save: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, owner); err != nil {
		return err
	}

	q := db.New(tx)
	snapshotID := id.New().String()
	written, err := q.InsertReadinessSnapshot(ctx, db.InsertReadinessSnapshotParams{
		ID:          snapshotID,
		CandidateID: owner.CandidateID, Mode: owner.Mode, TenantID: owner.TenantID,
		StandardReference: readiness.Standard.Reference,
		StandardVersion:   readiness.Standard.Version,
		StandardDigest:    readiness.Standard.Digest,
		RoleID:            readiness.Role,
		DisciplineID:      readiness.Discipline,
		RubricReference:   readiness.RubricReference,
		AnswerDigest:      answerDigest(readiness),
		ComputedAt:        readiness.ComputedAt,
	})
	if err != nil {
		return fmt.Errorf("progression: writing a readiness snapshot: %w", err)
	}
	// Nothing written means this exact answer is already stored, together
	// with its requirements, in this same transaction's view. Writing the
	// requirements again would say nothing new.
	if written == 0 {
		return tx.Commit(ctx)
	}

	for _, competency := range readiness.Competencies {
		var observedAt *time.Time
		if !competency.ObservedAt.IsZero() {
			at := competency.ObservedAt
			observedAt = &at
		}
		if err := q.InsertReadinessCompetency(ctx, db.InsertReadinessCompetencyParams{
			SnapshotID:  snapshotID,
			CandidateID: owner.CandidateID, Mode: owner.Mode, TenantID: owner.TenantID,
			CompetencyID: competency.CompetencyID,
			TargetBand:   competency.TargetBand,
			Outcome:      competency.Outcome,
			ObservedBand: competency.ObservedBand,
			// Empty becomes NULL in the query, which is what the schema
			// demands of an unassessed requirement.
			ObservationID: competency.ObservationID,
			ObservedAt:    observedAt,
			Reason:        competency.Reason,
		}); err != nil {
			return fmt.Errorf("progression: writing a readiness requirement: %w", err)
		}
	}
	return tx.Commit(ctx)
}

// Readiness answers the owner's current standing against every role
// standard they have one for, grouped by discipline then role.
//
// A list, never a total. Two roles come back as two answers here exactly
// as they were computed, because a combined figure across incomparable
// roles is the thing PRG-02 forbids and there is no layer at which it
// would be safe to introduce.
func (s *Store) Readiness(ctx context.Context, owner Owner) ([]Readiness, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: beginning the readiness read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, owner); err != nil {
		return nil, err
	}

	rows, err := db.New(tx).ListReadiness(ctx)
	if err != nil {
		return nil, fmt.Errorf("progression: listing readiness: %w", err)
	}

	readinesses := make([]Readiness, 0)
	current := ""
	for _, row := range rows {
		if row.SnapshotID != current {
			current = row.SnapshotID
			readinesses = append(readinesses, Readiness{
				Standard: Pin{
					Reference: row.StandardReference,
					Version:   row.StandardVersion,
					Digest:    row.StandardDigest,
				},
				Role:            row.RoleID,
				Discipline:      row.DisciplineID,
				RubricReference: row.RubricReference,
				ComputedAt:      row.ComputedAt,
			})
		}
		readiness := &readinesses[len(readinesses)-1]
		competency := CompetencyReadiness{
			CompetencyID:  row.CompetencyID,
			TargetBand:    row.TargetBand,
			Outcome:       row.Outcome,
			ObservedBand:  row.ObservedBand,
			Reason:        row.Reason,
			ObservationID: row.ObservationID,
		}
		if row.ObservedAt != nil {
			competency.ObservedAt = *row.ObservedAt
		}
		readiness.Competencies = append(readiness.Competencies, competency)
	}
	// Counted from the rows just read rather than from a stored summary,
	// which is why nothing above could have shown an unassessed
	// requirement as a pass.
	for index := range readinesses {
		readinesses[index].recount()
	}
	return readinesses, nil
}

// answerDigest identifies one readiness answer by its content.
//
// The pin and every resolved requirement go in, and the moment of
// computation stays out, which is what makes "nothing has changed" a
// recognisable fact rather than a comparison somebody has to remember to
// make. Requirements arrive sorted from Compute, so the digest is stable.
func answerDigest(readiness Readiness) string {
	var canonical strings.Builder
	fmt.Fprintf(&canonical, "%s|%s|%s|%s|%s|%s\n",
		readiness.Standard.Reference, readiness.Standard.Version,
		readiness.Standard.Digest, readiness.Role, readiness.Discipline,
		readiness.RubricReference)
	for _, competency := range readiness.Competencies {
		fmt.Fprintf(&canonical, "%s|%s|%s|%s|%s|%s\n",
			competency.CompetencyID, competency.TargetBand, competency.Outcome,
			competency.ObservedBand, competency.Reason, competency.ObservationID)
	}
	sum := sha256.Sum256([]byte(canonical.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}
