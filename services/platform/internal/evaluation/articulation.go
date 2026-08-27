package evaluation

// Delivery analysis: ART-02's platform half.
//
// Articulation runs as its own workflow with its own row, so it can fail,
// be retried, or come back not_assessable without touching the content
// evaluation - the two share a session id and nothing else. The result is
// stored once per session (unique) and never edited.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/evaluation/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Analyzer is what the workflow asks of the intelligence plane: the
// analysis document for a session, with the versions that produced it.
type Analyzer interface {
	Analyze(ctx context.Context, ref SessionRef) (Analysis, error)
}

// Analysis is what the calculator answered.
type Analysis struct {
	Status             string
	Warnings           []string
	Document           json.RawMessage
	CalculationVersion string
	PolicyVersion      string
	InputDigest        string
}

// Articulation is one stored analysis.
type Articulation struct {
	ID                 string
	SessionID          string
	Status             string
	Warnings           []string
	Document           json.RawMessage
	CalculationVersion string
	PolicyVersion      string
	InputDigest        string
	CreatedAt          time.Time
}

// ErrNoArticulation says the session has no delivery analysis (yet).
var ErrNoArticulation = errors.New("evaluation: no articulation for this session")

// StoreArticulation persists one analysis, exactly once per session; a
// duplicate run reads the existing row back.
func (s *Store) StoreArticulation(ctx context.Context, ref SessionRef, analysis Analysis) (Articulation, error) {
	warnings := analysis.Warnings
	if warnings == nil {
		warnings = []string{}
	}
	encodedWarnings, err := json.Marshal(warnings)
	if err != nil {
		return Articulation{}, fmt.Errorf("evaluation: encoding warnings: %w", err)
	}
	document := analysis.Document
	if len(document) == 0 {
		document = json.RawMessage(`{}`)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Articulation{}, fmt.Errorf("evaluation: beginning articulation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, ref); err != nil {
		return Articulation{}, err
	}
	err = db.New(tx).InsertArticulation(ctx, db.InsertArticulationParams{
		ID: id.New().String(), SessionID: ref.SessionID, Mode: ref.Mode,
		CandidateID: ref.CandidateID, TenantID: ref.TenantID,
		Status: analysis.Status, Warnings: encodedWarnings, Analysis: document,
		CalculationVersion: analysis.CalculationVersion, PolicyVersion: analysis.PolicyVersion,
		InputDigest: analysis.InputDigest,
	})
	if err != nil {
		if isUnique(err) {
			return s.ArticulationOf(ctx, ref)
		}
		return Articulation{}, fmt.Errorf("evaluation: storing articulation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Articulation{}, err
	}
	return s.ArticulationOf(ctx, ref)
}

// ArticulationOf reads a session's analysis back.
func (s *Store) ArticulationOf(ctx context.Context, ref SessionRef) (Articulation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Articulation{}, fmt.Errorf("evaluation: beginning read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, ref); err != nil {
		return Articulation{}, err
	}
	row, err := db.New(tx).GetArticulation(ctx, ref.SessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Articulation{}, ErrNoArticulation
	}
	if err != nil {
		return Articulation{}, err
	}
	var warnings []string
	if err := json.Unmarshal(row.Warnings, &warnings); err != nil {
		return Articulation{}, fmt.Errorf("evaluation: decoding warnings: %w", err)
	}
	return Articulation{
		ID: row.ID, SessionID: row.SessionID, Status: row.Status, Warnings: warnings,
		Document: json.RawMessage(row.Analysis), CalculationVersion: row.CalculationVersion,
		PolicyVersion: row.PolicyVersion, InputDigest: row.InputDigest, CreatedAt: row.CreatedAt,
	}, nil
}

// ArticulationActivities is the delivery workflow's side of the world.
type ArticulationActivities struct {
	store    *Store
	analyzer Analyzer
}

// NewArticulationActivities wires the store and the analyzer.
func NewArticulationActivities(store *Store, analyzer Analyzer) *ArticulationActivities {
	return &ArticulationActivities{store: store, analyzer: analyzer}
}

// AnalyzeAndStore asks the calculator and stores the answer, converging
// on the unique row on retry. A non-retryable refusal from the plane is
// this workflow's failure alone: the content evaluation never hears of it.
func (a *ArticulationActivities) AnalyzeAndStore(ctx context.Context, input EvidenceInput) (string, error) {
	ref := SessionRef{
		SessionID: input.SessionID, Mode: input.Mode,
		CandidateID: input.CandidateID, TenantID: input.TenantID,
	}
	analysis, err := a.analyzer.Analyze(ctx, ref)
	if err != nil {
		var failure *ExtractFailure
		if errors.As(err, &failure) && !failure.Retryable {
			return "", temporal.NewNonRetryableApplicationError(failure.Message, failure.Code, failure)
		}
		return "", err
	}
	stored, err := a.store.StoreArticulation(ctx, ref, analysis)
	if err != nil {
		return "", err
	}
	return stored.Status, nil
}

// ArticulationWorkflow measures delivery for one session. Its own
// workflow id, its own task, its own failure: by construction nothing
// here can block, fail or alter EvidenceWorkflow's result.
func ArticulationWorkflow(ctx workflow.Context, input EvidenceInput) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 120 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    2 * time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    5,
		},
	})
	var activities *ArticulationActivities
	var status string
	return workflow.ExecuteActivity(ctx, activities.AnalyzeAndStore, input).Get(ctx, &status)
}
