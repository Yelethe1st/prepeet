package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/evaluation/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// The result: one per session, immutable, with its notification in the
// same transaction. Retry safety is structural: the session's uniqueness
// makes a second insert converge on the first, and because the event rides
// the same transaction, exactly one result means exactly one notification.

// RubricPin is the rubric exactly as the session's bundle pinned it.
type RubricPin struct {
	Reference string
	Version   string
	Digest    string
	Body      []byte
}

// ErrNoResult says the session has no stored evaluation yet: distinct
// from the session not existing, because "still evaluating" and "not
// yours to see" must never share an answer.
var ErrNoResult = errors.New("evaluation: no result for this session")

// Result is one stored evaluation.
type Result struct {
	ID                 string
	SessionID          string
	RubricReference    string
	RubricVersion      string
	RubricDigest       string
	AggregationVersion string
	ExtractionVersion  string
	ModelVersion       string
	PolicyVersion      string
	Aggregation        Aggregation
	ResultDigest       string
	Warnings           []string
	CreatedAt          time.Time
}

// StoreResult persists one aggregation with its completed event, exactly
// once per session. A duplicate run reads the existing result back.
func (s *Store) StoreResult(ctx context.Context, events *outbox.Store, ref SessionRef, pin RubricPin, extractionVersion string, aggregation Aggregation, warnings []string) (Result, error) {
	competencies, err := json.Marshal(aggregation.Competencies)
	if err != nil {
		return Result{}, fmt.Errorf("evaluation: encoding competencies: %w", err)
	}
	sum := sha256.Sum256(competencies)
	resultDigest := "sha256:" + hex.EncodeToString(sum[:])
	if warnings == nil {
		warnings = []string{}
	}
	encodedWarnings, err := json.Marshal(warnings)
	if err != nil {
		return Result{}, fmt.Errorf("evaluation: encoding warnings: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("evaluation: beginning result: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, ref); err != nil {
		return Result{}, err
	}
	q := db.New(tx)

	resultID := id.New().String()
	err = q.InsertResult(ctx, db.InsertResultParams{
		ID: resultID, SessionID: ref.SessionID, Mode: ref.Mode,
		CandidateID: ref.CandidateID, TenantID: ref.TenantID,
		RubricReference: pin.Reference, RubricVersion: pin.Version, RubricDigest: pin.Digest,
		AggregationVersion: AggregationVersion, ExtractionVersion: extractionVersion,
		ModelVersion: "none", PolicyVersion: "none",
		Competencies: competencies, ResultDigest: resultDigest,
		CoveredCompetencies: int32(aggregation.CoveredCompetencies),
		TotalCompetencies:   int32(aggregation.TotalCompetencies),
		Warnings:            encodedWarnings,
	})
	if err != nil {
		if isUnique(err) {
			// A previous run won; its result and its notification stand.
			return s.ResultOf(ctx, ref)
		}
		return Result{}, fmt.Errorf("evaluation: storing the result: %w", err)
	}

	payload, err := json.Marshal(map[string]any{
		"evaluation_id":  resultID,
		"session_id":     ref.SessionID,
		"rubric_id":      pin.Reference,
		"rubric_version": pin.Version,
		"result_digest":  resultDigest,
	})
	if err != nil {
		return Result{}, fmt.Errorf("evaluation: encoding the completed event: %w", err)
	}
	if _, err := events.Publish(ctx, tx, outbox.Event{
		Type:          "evaluation.completed.v1",
		SchemaVersion: "1.0",
		TenantID:      ref.TenantID,
		Producer:      "evaluation",
		Actor:         outbox.Actor{Type: "service", ID: ref.CandidateID},
		Purpose:       ref.Mode,
		Payload:       payload,
	}); err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return s.ResultOf(ctx, ref)
}

// ResultOf reads a session's result back.
func (s *Store) ResultOf(ctx context.Context, ref SessionRef) (Result, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("evaluation: beginning read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, ref); err != nil {
		return Result{}, err
	}

	row, err := db.New(tx).GetResult(ctx, ref.SessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Result{}, ErrNoResult
	}
	if err != nil {
		return Result{}, err
	}
	var competencies []CompetencyResult
	if err := json.Unmarshal(row.Competencies, &competencies); err != nil {
		return Result{}, fmt.Errorf("evaluation: decoding competencies: %w", err)
	}
	var warnings []string
	if err := json.Unmarshal(row.Warnings, &warnings); err != nil {
		return Result{}, fmt.Errorf("evaluation: decoding warnings: %w", err)
	}
	return Result{
		ID: row.ID, SessionID: row.SessionID,
		RubricReference: row.RubricReference, RubricVersion: row.RubricVersion,
		RubricDigest: row.RubricDigest, AggregationVersion: row.AggregationVersion,
		ExtractionVersion: row.ExtractionVersion, ModelVersion: row.ModelVersion,
		PolicyVersion: row.PolicyVersion,
		Aggregation: Aggregation{
			Competencies:        competencies,
			Coverage:            CoverageOf(competencies),
			CoveredCompetencies: int(row.CoveredCompetencies),
			TotalCompetencies:   int(row.TotalCompetencies),
		},
		ResultDigest: row.ResultDigest, Warnings: warnings, CreatedAt: row.CreatedAt,
	}, nil
}

func isUnique(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}
