package evaluation

// Whether the coaching described the person it was about: ART-09.
//
// QUA-03 calibrates against human benchmarks and QUA-06 monitors production,
// and neither had an input from the candidate reading the output. The one
// person who knows whether "your opening establishes a clear, defensible
// position" is true of their own answer is the person who gave it.
//
// A verdict is a report about the coaching, not a way to edit it: nothing
// here is read by anything that decides what the candidate is shown.

import (
	"context"
	"errors"
	"fmt"
	"time"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/evaluation/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// The kinds of generated insight a candidate can answer. A closed set,
// because the column has a CHECK and a typo should fail here rather than in
// the database, where the message names a constraint instead of a field.
const (
	InsightStrength = "strength"
	InsightPriority = "priority"
	InsightDrill    = "drill"
)

// ErrUnknownInsightKind is a kind the schema will not accept.
var ErrUnknownInsightKind = errors.New("evaluation: unknown insight kind")

// ErrScreeningFeedback says feedback was offered on a screening session.
//
// Refused here as well as by the schema. A screening candidate rating their
// own assessment would be a channel for pressure, and the caller deserves a
// sentence rather than a constraint violation.
var ErrScreeningFeedback = errors.New("evaluation: insight feedback is practice only")

// InsightVerdict is one answer about one generated insight.
type InsightVerdict struct {
	// Kind and Key together identify what was answered: the dimension a
	// strength or priority was generated for, or the drill's key.
	Kind      string
	Key       string
	Dimension string
	Helpful   bool
	// ArtifactDigest and PolicyVersion are what produced the insight, taken
	// at the moment it was on screen rather than at the moment it is read
	// back, so a drop is attributable to a version rather than to a date.
	ArtifactDigest string
	PolicyVersion  string
	UpdatedAt      time.Time
}

func validKind(kind string) bool {
	return kind == InsightStrength || kind == InsightPriority || kind == InsightDrill
}

// RecordInsightFeedback stores one verdict, once per insight and changeable.
//
// Pressing the other thumb corrects the row rather than adding a second
// opinion from the same person about the same sentence, which is what the
// unique constraint and the upsert are between them for.
func (s *Store) RecordInsightFeedback(ctx context.Context, ref SessionRef, verdict InsightVerdict) error {
	if ref.Mode != "practice" {
		return ErrScreeningFeedback
	}
	if !validKind(verdict.Kind) {
		return fmt.Errorf("%w: %q", ErrUnknownInsightKind, verdict.Kind)
	}
	if verdict.Key == "" {
		return errors.New("evaluation: insight feedback needs the insight it is about")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("evaluation: beginning insight feedback: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, ref); err != nil {
		return err
	}
	err = db.New(tx).RecordInsightFeedback(ctx, db.RecordInsightFeedbackParams{
		ID:             id.New().String(),
		SessionID:      ref.SessionID,
		CandidateID:    ref.CandidateID,
		InsightKind:    verdict.Kind,
		InsightKey:     verdict.Key,
		Dimension:      verdict.Dimension,
		Helpful:        verdict.Helpful,
		ArtifactDigest: verdict.ArtifactDigest,
		PolicyVersion:  verdict.PolicyVersion,
	})
	if err != nil {
		return fmt.Errorf("evaluation: recording insight feedback: %w", err)
	}
	return tx.Commit(ctx)
}

// InsightFeedbackFor reads back what this candidate already said, so the
// screen can show which thumb is pressed rather than asking again.
func (s *Store) InsightFeedbackFor(ctx context.Context, ref SessionRef) ([]InsightVerdict, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("evaluation: beginning insight feedback read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, ref); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).ListInsightFeedback(ctx, ref.SessionID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: reading insight feedback: %w", err)
	}
	verdicts := make([]InsightVerdict, 0, len(rows))
	for _, row := range rows {
		verdicts = append(verdicts, InsightVerdict{
			Kind:      row.InsightKind,
			Key:       row.InsightKey,
			Dimension: row.Dimension.String,
			Helpful:   row.Helpful,
			UpdatedAt: row.UpdatedAt,
		})
	}
	return verdicts, nil
}
