package evaluation_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
)

// EVL-02's control flow under Temporal's test environment: extraction
// feeds aggregation, a failure at either stage publishes the failed event
// with that stage's own code, and success publishes nothing extra here
// (the completed event is StoreResult's, same transaction as the row).

var evidenceInput = evaluation.EvidenceInput{
	SessionID:   "00000000-0000-7000-8000-0000000000a1",
	Mode:        "practice",
	CandidateID: "00000000-0000-7000-8000-0000000000a2",
}

func workflowEnvironment(t *testing.T) *testsuite.TestWorkflowEnvironment {
	t.Helper()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	var activities *evaluation.Activities
	env.RegisterActivity(activities.ExtractAndStore)
	env.RegisterActivity(activities.Aggregate)
	env.RegisterActivity(activities.PublishFailed)
	return env
}

func TestExtractionFeedsAggregationAndNothingElse(t *testing.T) {
	env := workflowEnvironment(t)

	outcome := evaluation.ExtractOutcome{
		ExtractionVersion: "evidence-1",
		Sealed: evaluation.SealedInput{
			SessionID:    evidenceInput.SessionID,
			Competencies: []evaluation.Competency{{ID: "systems-design", Name: "Systems design"}},
		},
	}
	env.OnActivity("ExtractAndStore", mock.Anything, evidenceInput).Return(outcome, nil)
	// Aggregation receives exactly what extraction produced: the version
	// and the competency list, never refetched, never invented.
	env.OnActivity("Aggregate", mock.Anything, evidenceInput,
		"evidence-1", outcome.Sealed).Return(nil)

	env.ExecuteWorkflow(evaluation.EvidenceWorkflow, evidenceInput)

	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("workflow did not complete cleanly: %v", env.GetWorkflowError())
	}
	env.AssertExpectations(t)
	env.AssertNotCalled(t, "PublishFailed", mock.Anything, mock.Anything, mock.Anything)
}

func TestAFabricationRefusalSkipsAggregationAndPublishesItsOwnCode(t *testing.T) {
	env := workflowEnvironment(t)

	refusal := temporal.NewNonRetryableApplicationError(
		"the quote is not what the transcript says",
		"FAILURE_CODE_SCHEMA_VALIDATION_FAILED", nil)
	env.OnActivity("ExtractAndStore", mock.Anything, evidenceInput).
		Return(evaluation.ExtractOutcome{}, refusal).Once()
	env.OnActivity("PublishFailed", mock.Anything, evidenceInput,
		"FAILURE_CODE_SCHEMA_VALIDATION_FAILED").Return(nil)

	env.ExecuteWorkflow(evaluation.EvidenceWorkflow, evidenceInput)

	if env.GetWorkflowError() == nil {
		t.Fatal("a refused extraction must fail the workflow, not succeed quietly")
	}
	env.AssertExpectations(t)
	env.AssertNotCalled(t, "Aggregate", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestAnAggregationFailureIsPublishedNotSwallowed(t *testing.T) {
	env := workflowEnvironment(t)

	outcome := evaluation.ExtractOutcome{ExtractionVersion: "evidence-1"}
	env.OnActivity("ExtractAndStore", mock.Anything, evidenceInput).Return(outcome, nil)
	// A bundle with no rubric pin: aggregation's own non-retryable refusal.
	refusal := temporal.NewNonRetryableApplicationError(
		"the session's bundle pins no rubric",
		"FAILURE_CODE_ARTIFACT_NOT_FOUND", nil)
	env.OnActivity("Aggregate", mock.Anything, evidenceInput,
		"evidence-1", outcome.Sealed).Return(refusal).Once()
	env.OnActivity("PublishFailed", mock.Anything, evidenceInput,
		"FAILURE_CODE_ARTIFACT_NOT_FOUND").Return(nil)

	env.ExecuteWorkflow(evaluation.EvidenceWorkflow, evidenceInput)

	if env.GetWorkflowError() == nil {
		t.Fatal("a failed aggregation must fail the workflow")
	}
	env.AssertExpectations(t)
}

func TestAnUnpublishableResultFailsVisiblyNotSilently(t *testing.T) {
	// EVL-05's third box: the publication gate's refusal becomes the
	// failed event with its own code, so the session lands in the
	// visible, retryable evaluation_failed state - never a published bad
	// result, never a swallowed error.
	env := workflowEnvironment(t)

	outcome := evaluation.ExtractOutcome{ExtractionVersion: "evidence-1"}
	env.OnActivity("ExtractAndStore", mock.Anything, evidenceInput).Return(outcome, nil)
	refusal := temporal.NewNonRetryableApplicationError(
		"the result does not match its stored evidence",
		"FAILURE_CODE_SCHEMA_VALIDATION_FAILED", evaluation.ErrUnpublishable)
	env.OnActivity("Aggregate", mock.Anything, evidenceInput,
		"evidence-1", outcome.Sealed).Return(refusal).Once()
	env.OnActivity("PublishFailed", mock.Anything, evidenceInput,
		"FAILURE_CODE_SCHEMA_VALIDATION_FAILED").Return(nil)

	env.ExecuteWorkflow(evaluation.EvidenceWorkflow, evidenceInput)

	if env.GetWorkflowError() == nil {
		t.Fatal("an unpublishable result must fail the workflow")
	}
	env.AssertExpectations(t)
}
