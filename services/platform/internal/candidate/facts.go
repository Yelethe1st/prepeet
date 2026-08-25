package candidate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/candidate/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/database"
)

// The candidate's side of extraction: inspecting what was read and acting on
// it. Implements PRO-04's flows.
//
// The rule everything here bends around: extraction is assistive, never
// authoritative. The extracted value is written once by the pipeline and
// never rewritten by anything in this file; a correction is a second value
// beside the first, and rejection is a status, not a deletion. What
// downstream consumers get is the effective read - correction over
// extraction, rejected facts absent - so the candidate's word wins from the
// moment they give it while the original stays auditable underneath.

// maxCorrectionBytes bounds a corrected value. A correction is a fixed
// field or two, not a document.
const maxCorrectionBytes = 16 << 10

// reviewStatuses is what a candidate can move a fact to. proposed is the
// pipeline's word, not a move: un-reviewing is confirming or rejecting again.
var reviewStatuses = map[string]bool{"confirmed": true, "corrected": true, "rejected": true}

// Stable refusals for the review flows.
var (
	ErrFactNotFound = errors.New("candidate: FACT_NOT_FOUND: no such fact")
	ErrFactReview   = errors.New("candidate: FACT_REVIEW_INVALID: that is not a review this fact can take")
)

// Fact is one extracted fact as the candidate sees it: the extraction, the
// provenance, and whatever they have done about it.
type Fact struct {
	ID         string
	DocumentID string
	Kind       string
	// Value is what extraction read. Never rewritten.
	Value json.RawMessage
	// CorrectedValue is the candidate's version, nil unless status is
	// corrected.
	CorrectedValue   json.RawMessage
	SpanStart        int
	SpanEnd          int
	Confidence       float64
	ExtractorVersion string
	Status           string
	CreatedAt        time.Time
	ReviewedAt       *time.Time
}

// EffectiveFact is a fact as downstream consumers read it: one value, which
// is the correction where one exists.
type EffectiveFact struct {
	ID               string
	DocumentID       string
	Kind             string
	Value            json.RawMessage
	SpanStart        int
	SpanEnd          int
	Status           string
	ExtractorVersion string
}

// ListFacts returns one document's facts in span order.
//
// The document is read first so absence - including somebody else's document
// - answers as ErrDocumentNotFound rather than as an empty list that looks
// like an extraction that found nothing.
func (d *Documents) ListFacts(ctx context.Context, userID, documentID string) ([]Fact, error) {
	if _, _, err := d.getRow(ctx, documentID, userID); err != nil {
		return nil, err
	}

	tx, err := d.pool.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("candidate: beginning fact list: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return nil, err
	}

	rows, err := db.New(tx).ListFactsByDocument(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("candidate: listing facts: %w", err)
	}
	facts := make([]Fact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, factFrom(db.ReviewFactRow(row)))
	}
	return facts, nil
}

// ReviewFact records the candidate's move on one fact.
//
// The refusals are shape refusals, made before the database is asked: a
// correction must carry a JSON object within bounds, and the status must be
// one of the lifecycle's. Ownership is the row scope - a fact that is not
// theirs is a fact that does not exist.
func (d *Documents) ReviewFact(ctx context.Context, userID, factID, status string, corrected json.RawMessage) (Fact, error) {
	if !reviewStatuses[status] {
		return Fact{}, fmt.Errorf("%w: status %q", ErrFactReview, status)
	}
	if status == "corrected" {
		if err := validCorrection(corrected); err != nil {
			return Fact{}, err
		}
	} else if corrected != nil {
		return Fact{}, fmt.Errorf("%w: only a correction carries a corrected value", ErrFactReview)
	}

	tx, err := d.pool.pool.Begin(ctx)
	if err != nil {
		return Fact{}, fmt.Errorf("candidate: beginning review: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return Fact{}, err
	}

	row, err := db.New(tx).ReviewFact(ctx, db.ReviewFactParams{
		ID: factID, Status: status, CorrectedValue: corrected,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Fact{}, ErrFactNotFound
	}
	if err != nil {
		return Fact{}, fmt.Errorf("candidate: reviewing fact: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Fact{}, err
	}
	return factFrom(row), nil
}

// EffectiveFacts is the read composition consumes: corrections win, rejected
// facts are absent, everything in span order.
func (d *Documents) EffectiveFacts(ctx context.Context, userID string) ([]EffectiveFact, error) {
	tx, err := d.pool.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("candidate: beginning effective read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := database.SetUser(ctx, tx, userID); err != nil {
		return nil, err
	}

	rows, err := db.New(tx).ListEffectiveFacts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("candidate: effective facts: %w", err)
	}
	facts := make([]EffectiveFact, 0, len(rows))
	for _, row := range rows {
		facts = append(facts, EffectiveFact{
			ID: row.ID, DocumentID: row.DocumentID, Kind: row.Kind,
			Value:     json.RawMessage(row.Value),
			SpanStart: int(row.SpanStart), SpanEnd: int(row.SpanEnd),
			Status: row.Status, ExtractorVersion: row.ExtractorVersion,
		})
	}
	return facts, nil
}

// validCorrection admits a bounded JSON object and nothing else. An array or
// scalar would be a value no kind's schema shapes, and unbounded JSON is a
// document wearing a field's name.
func validCorrection(corrected json.RawMessage) error {
	if len(corrected) == 0 {
		return fmt.Errorf("%w: a correction needs its corrected value", ErrFactReview)
	}
	if len(corrected) > maxCorrectionBytes {
		return fmt.Errorf("%w: a correction is at most %d bytes", ErrFactReview, maxCorrectionBytes)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(corrected, &value); err != nil || value == nil {
		return fmt.Errorf("%w: a correction is a JSON object", ErrFactReview)
	}
	return nil
}

func factFrom(row db.ReviewFactRow) Fact {
	fact := Fact{
		ID: row.ID, DocumentID: row.DocumentID, Kind: row.Kind,
		Value:     json.RawMessage(row.Value),
		SpanStart: int(row.SpanStart), SpanEnd: int(row.SpanEnd),
		Confidence: row.Confidence, ExtractorVersion: row.ExtractorVersion,
		Status: row.Status, CreatedAt: row.CreatedAt, ReviewedAt: row.ReviewedAt,
	}
	// The query answers 'null'::jsonb for an absent correction so sqlc gets a
	// non-null column type; the port's word for absence is nil.
	if string(fact.Status) != "corrected" {
		return fact
	}
	fact.CorrectedValue = json.RawMessage(row.CorrectedValue)
	return fact
}
