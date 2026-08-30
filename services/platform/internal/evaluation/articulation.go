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
	// CostUnits is what the capability reported spending, recorded
	// against the stage's budget.
	CostUnits int
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

// ErrArticulationRefused says the analysis must not run for this session.
//
// Not retryable: a screening session will still be a screening session on the
// next attempt, so this ends the workflow rather than looping it.
var ErrArticulationRefused = errors.New("evaluation: delivery analysis refused")

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
	// policies answers the budget this session pinned. Optional: without
	// it delivery runs unbudgeted, which is the harness's case, never
	// cmd's.
	policies RubricSource
}

// NewArticulationActivities wires the store and the analyzer.
func NewArticulationActivities(store *Store, analyzer Analyzer) *ArticulationActivities {
	return &ArticulationActivities{store: store, analyzer: analyzer}
}

// WithPolicy adds the pinned budget delivery must run inside (EVL-07).
func (a *ArticulationActivities) WithPolicy(policies RubricSource) *ArticulationActivities {
	a.policies = policies
	return a
}

// AnalyzeAndStore asks the calculator and stores the answer, converging
// on the unique row on retry. A non-retryable refusal from the plane is
// this workflow's failure alone: the content evaluation never hears of it.
func (a *ArticulationActivities) AnalyzeAndStore(ctx context.Context, input EvidenceInput) (string, error) {
	// Refused again here, and this is not belt and braces for its own sake.
	// A workflow can be started by hand, by a replay, or by a future producer
	// that forgets the rule, and the boundary this protects is one the product
	// states in candidate-facing copy: screening never produces delivery
	// coaching, and a stored screening analysis would make that untrue
	// whatever reads it.
	if input.Mode != "practice" {
		return "", fmt.Errorf("%w: delivery analysis is practice only, not %q",
			ErrArticulationRefused, input.Mode)
	}

	ref := SessionRef{
		SessionID: input.SessionID, Mode: input.Mode,
		CandidateID: input.CandidateID, TenantID: input.TenantID,
	}
	// Can this session still afford delivery? An optional stage that
	// cannot is omitted, recorded and shown in words - never a failure,
	// and never a quietly thinner evaluation (ADR-0019, EVL-07).
	if a.policies != nil {
		affords, err := a.affords(ctx, ref)
		if err != nil {
			return "", err
		}
		if !affords {
			if err := a.store.RecordStage(ctx, ref, StageOutcome{
				Stage: StageArticulation, Status: "omitted",
				Reason: ReasonBudgetExhausted, Required: false,
			}); err != nil {
				return "", err
			}
			return "omitted", nil
		}
	}

	analysis, err := a.analyzer.Analyze(ctx, ref)
	if err != nil {
		var failure *ExtractFailure
		if errors.As(err, &failure) {
			// Delivery failing is delivery's own business: the record
			// says so and the content evaluation is untouched.
			_ = a.store.RecordStage(ctx, ref, StageOutcome{
				Stage: StageArticulation, Status: "failed", Reason: failure.Code,
				Retryable: failure.Retryable, Required: false,
			})
			if !failure.Retryable {
				return "", temporal.NewNonRetryableApplicationError(failure.Message, failure.Code, failure)
			}
		}
		return "", err
	}
	stored, err := a.store.StoreArticulation(ctx, ref, analysis)
	if err != nil {
		return "", err
	}
	if err := a.store.RecordStage(ctx, ref, StageOutcome{
		Stage: StageArticulation, Status: "completed", Required: false,
		CostUnits: analysis.CostUnits,
	}); err != nil {
		return "", err
	}
	return stored.Status, nil
}

// affords reads the pinned policy and what delivery has already spent.
func (a *ArticulationActivities) affords(ctx context.Context, ref SessionRef) (bool, error) {
	pin, err := a.policies.PinnedPolicy(ctx, ref)
	if err != nil {
		return false, err
	}
	policy, err := ParsePolicy(pin.Body)
	if err != nil {
		return false, temporal.NewNonRetryableApplicationError(
			err.Error(), "FAILURE_CODE_SCHEMA_VALIDATION_FAILED", err)
	}
	outcomes, err := a.store.StageOutcomes(ctx, ref)
	if err != nil {
		return false, err
	}
	return Affords(policy, outcomes, StageArticulation)
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
