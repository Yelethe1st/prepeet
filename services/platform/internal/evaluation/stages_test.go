package evaluation_test

import (
	"errors"
	"testing"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
)

// The budget arithmetic and what it produces for the candidate: EVL-07's
// second and third boxes as pure functions, before any database.

func outcome(stage, status, reason string, required, retryable bool, cost int) evaluation.StageOutcome {
	return evaluation.StageOutcome{
		Stage: stage, Status: status, Reason: reason,
		Required: required, Retryable: retryable, CostUnits: cost,
	}
}

func TestAStageAffordsUntilItsBudgetIsSpent(t *testing.T) {
	policy := policyFixture(t)

	// Nothing spent yet.
	affords, err := evaluation.Affords(policy, nil, evaluation.StageArticulation)
	if err != nil || !affords {
		t.Fatalf("a fresh session cannot afford articulation: %v %v", affords, err)
	}

	// Part spent still affords; the whole budget does not.
	partly := []evaluation.StageOutcome{outcome("articulation", "completed", "", false, false, 59)}
	if affords, _ := evaluation.Affords(policy, partly, evaluation.StageArticulation); !affords {
		t.Fatal("59 of 60 spent should still afford")
	}
	spent := []evaluation.StageOutcome{outcome("articulation", "completed", "", false, false, 60)}
	affords, err = evaluation.Affords(policy, spent, evaluation.StageArticulation)
	if err != nil || affords {
		t.Fatalf("a spent budget still affords: %v %v", affords, err)
	}
}

func TestAnUnbudgetedStageCannotRun(t *testing.T) {
	// Silence is not permission: a stage the policy does not name must
	// not spend by default.
	if _, err := evaluation.Affords(policyFixture(t), nil, "invented"); err == nil {
		t.Fatal("a stage the policy never named was allowed to run")
	}
}

func TestARequiredStageOutOfBudgetIsAFailureNotAnOmission(t *testing.T) {
	// Omitting a required stage would produce a result that is quietly
	// less than a result. The caller is not offered that choice.
	spent := []evaluation.StageOutcome{outcome("aggregation", "completed", "", true, false, 20)}
	affords, err := evaluation.Affords(policyFixture(t), spent, evaluation.StageAggregation)
	if affords {
		t.Fatal("a required stage was allowed past its budget")
	}
	if !errors.Is(err, evaluation.ErrRequiredStageUnaffordable) {
		t.Fatalf("err = %v, want ErrRequiredStageUnaffordable", err)
	}
}

func TestSpendAccumulatesAcrossAttempts(t *testing.T) {
	// Append-only means a retried stage has several rows, and what it has
	// spent is all of them: a retry that ignored earlier spend would let
	// a stage run forever.
	outcomes := []evaluation.StageOutcome{
		outcome("articulation", "failed", "FAILURE_CODE_PROVIDER_TIMEOUT", false, true, 25),
		outcome("articulation", "failed", "FAILURE_CODE_PROVIDER_TIMEOUT", false, true, 25),
	}
	if spent := evaluation.Spent(outcomes, evaluation.StageArticulation); spent != 50 {
		t.Fatalf("spent = %d, want 50", spent)
	}
	if affords, _ := evaluation.Affords(policyFixture(t), outcomes, evaluation.StageArticulation); !affords {
		t.Fatal("50 of 60 should still afford one more attempt")
	}
	outcomes = append(outcomes, outcome("articulation", "failed", "x", false, true, 10))
	if affords, _ := evaluation.Affords(policyFixture(t), outcomes, evaluation.StageArticulation); affords {
		t.Fatal("60 of 60 spent still afforded")
	}
}

func TestTheStandingIsTheLatestAttempt(t *testing.T) {
	outcomes := []evaluation.StageOutcome{
		outcome("articulation", "failed", "FAILURE_CODE_PROVIDER_TIMEOUT", false, true, 10),
		outcome("articulation", "completed", "", false, false, 10),
	}
	standing, found := evaluation.Standing(outcomes, evaluation.StageArticulation)
	if !found || standing.Status != "completed" {
		t.Fatalf("standing = %+v", standing)
	}
	if _, found := evaluation.Standing(outcomes, evaluation.StageCoaching); found {
		t.Fatal("a stage that never ran has a standing")
	}
}

func TestOmissionsNameOnlyTheOptionalPartsThatAreMissing(t *testing.T) {
	outcomes := []evaluation.StageOutcome{
		outcome("evidence", "completed", "", true, false, 5),
		outcome("aggregation", "completed", "", true, false, 2),
		outcome("articulation", "omitted", evaluation.ReasonBudgetExhausted, false, false, 0),
		outcome("coaching", "failed", "FAILURE_CODE_PROVIDER_TIMEOUT", false, true, 3),
	}

	omissions := evaluation.Omissions(outcomes)
	if len(omissions) != 2 {
		t.Fatalf("omissions = %+v", omissions)
	}
	byStage := map[string]evaluation.Omission{}
	for _, omission := range omissions {
		byStage[omission.Stage] = omission
	}
	if byStage["articulation"].Reason != evaluation.ReasonBudgetExhausted || byStage["articulation"].Retryable {
		t.Fatalf("articulation = %+v", byStage["articulation"])
	}
	// Terminal and retryable are different things to be told, so the
	// distinction is carried rather than inferred from the wording.
	if !byStage["coaching"].Retryable {
		t.Fatalf("coaching = %+v", byStage["coaching"])
	}
}

func TestACompleteEvaluationOmitsNothing(t *testing.T) {
	outcomes := []evaluation.StageOutcome{
		outcome("evidence", "completed", "", true, false, 5),
		outcome("articulation", "completed", "", false, false, 10),
		outcome("coaching", "completed", "", false, false, 4),
	}
	if omissions := evaluation.Omissions(outcomes); len(omissions) != 0 {
		t.Fatalf("a complete evaluation reported %+v", omissions)
	}
	// A required stage is never an omission: a result without one does
	// not exist to have omissions.
	failed := []evaluation.StageOutcome{outcome("evidence", "failed", "X", true, false, 0)}
	if omissions := evaluation.Omissions(failed); len(omissions) != 0 {
		t.Fatalf("a required stage was reported as an omission: %+v", omissions)
	}
}
