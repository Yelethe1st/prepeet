package evaluation

// Stage outcomes and the budget they spend against: EVL-07.
//
// Three questions are answered from one record. What did each stage do,
// so an operator can see a failure and tell whether it is worth retrying.
// What has a stage spent, so the next optional stage can be told it
// cannot afford to run. And what is absent from this result and why, so
// the candidate is shown an omission in words rather than a silently
// thinner evaluation.

import (
	"context"
	"errors"
	"fmt"
	"time"

	db "github.com/Yelethe1st/prepeet/services/platform/internal/evaluation/db"
	"github.com/Yelethe1st/prepeet/services/platform/platform/id"
)

// Stage names, matching the policy artifact's own.
const (
	StageEvidence     = "evidence"
	StageAggregation  = "aggregation"
	StageArticulation = "articulation"
	StageCoaching     = "coaching"
)

// ReasonBudgetExhausted is why an optional stage did not run.
const ReasonBudgetExhausted = "BUDGET_EXHAUSTED"

// ErrRequiredStageUnaffordable refuses to skip a stage the result needs.
//
// A required stage out of budget is not an omission, because there is no
// result to omit it from: it is a failure, loudly, so the policy that
// budgeted it too low is fixed rather than quietly producing less.
var ErrRequiredStageUnaffordable = errors.New(
	"evaluation: BUDGET_EXHAUSTED: a required stage has no budget left")

// StageOutcome is one recorded stage attempt.
type StageOutcome struct {
	Stage     string
	Status    string
	Reason    string
	Retryable bool
	Required  bool
	CostUnits int
	CreatedAt time.Time
}

// RecordStage appends one stage attempt. Append-only: a retry is a new
// row, so "failed once then worked" stays distinguishable from "worked".
func (s *Store) RecordStage(ctx context.Context, ref SessionRef, outcome StageOutcome) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("evaluation: beginning stage record: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, ref); err != nil {
		return err
	}
	if err := db.New(tx).InsertStageOutcome(ctx, db.InsertStageOutcomeParams{
		ID: id.New().String(), SessionID: ref.SessionID, Mode: ref.Mode,
		CandidateID: ref.CandidateID, TenantID: ref.TenantID,
		Stage: outcome.Stage, Status: outcome.Status, Reason: outcome.Reason,
		Retryable: outcome.Retryable, Required: outcome.Required,
		CostUnits: int32(outcome.CostUnits),
	}); err != nil {
		return fmt.Errorf("evaluation: recording the stage outcome: %w", err)
	}
	return tx.Commit(ctx)
}

// StageOutcomes answers every attempt for a session, oldest first.
func (s *Store) StageOutcomes(ctx context.Context, ref SessionRef) ([]StageOutcome, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("evaluation: beginning stage read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := scope(ctx, tx, ref); err != nil {
		return nil, err
	}
	rows, err := db.New(tx).ListStageOutcomes(ctx, ref.SessionID)
	if err != nil {
		return nil, fmt.Errorf("evaluation: listing stage outcomes: %w", err)
	}
	outcomes := make([]StageOutcome, 0, len(rows))
	for _, row := range rows {
		outcomes = append(outcomes, StageOutcome{
			Stage: row.Stage, Status: row.Status, Reason: row.Reason,
			Retryable: row.Retryable, Required: row.Required,
			CostUnits: int(row.CostUnits), CreatedAt: row.CreatedAt,
		})
	}
	return outcomes, nil
}

// Spent totals what one stage has already cost this session.
func Spent(outcomes []StageOutcome, stage string) int {
	total := 0
	for _, outcome := range outcomes {
		if outcome.Stage == stage {
			total += outcome.CostUnits
		}
	}
	return total
}

// Standing answers a stage's latest attempt, if it has one.
func Standing(outcomes []StageOutcome, stage string) (StageOutcome, bool) {
	var latest StageOutcome
	found := false
	for _, outcome := range outcomes {
		if outcome.Stage == stage {
			latest, found = outcome, true
		}
	}
	return latest, found
}

// Affords reports whether a stage may still run under its policy.
//
// A stage the policy does not name cannot run: a stage nobody budgeted
// for must not spend by default. A required stage that cannot afford to
// run is an error rather than a false, because omitting it is not an
// option the caller may take.
func Affords(policy Policy, outcomes []StageOutcome, stage string) (bool, error) {
	required, budget, known := policy.Stage(stage)
	if !known {
		return false, fmt.Errorf("evaluation: stage %q is not in the pinned policy", stage)
	}
	if Spent(outcomes, stage) < budget {
		return true, nil
	}
	if required {
		return false, fmt.Errorf("%w: %s", ErrRequiredStageUnaffordable, stage)
	}
	return false, nil
}

// Omission is an optional part of the evaluation that is not there, and
// why - the candidate-facing half of EVL-07.
type Omission struct {
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
	// Retryable says whether this is expected to resolve on its own. The
	// difference decides what the person is told to do, so it is carried
	// rather than inferred from the reason's wording.
	Retryable bool `json:"retryable"`
}

// Omissions answers what is missing from a result and why: optional
// stages that failed or could not afford to run. Required stages are not
// omissions - a result without one does not exist.
func Omissions(outcomes []StageOutcome) []Omission {
	seen := map[string]bool{}
	omissions := make([]Omission, 0)
	for _, stage := range []string{StageArticulation, StageCoaching} {
		standing, present := Standing(outcomes, stage)
		if !present || standing.Required || standing.Status == "completed" || seen[stage] {
			continue
		}
		seen[stage] = true
		omissions = append(omissions, Omission{
			Stage: stage, Reason: standing.Reason, Retryable: standing.Retryable,
		})
	}
	return omissions
}
