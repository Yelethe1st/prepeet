package candidate_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
)

// The extraction workflow's decisions, under Temporal's test environment.
//
// What is asserted is the control flow: success stores the facts, an
// unsupported format is an outcome rather than a failure, a real failure is
// recorded as one, and a redelivery that finds the extraction already decided
// ends quietly. The activities' behaviour against the database is the
// integration suite's subject.

var extractionInput = candidate.ExtractionInput{
	DocumentID:  "00000000-0000-7000-8000-0000000000d1",
	CandidateID: "00000000-0000-7000-8000-0000000000c1",
}

func extractionEnvironment(t *testing.T) *testsuite.TestWorkflowEnvironment {
	t.Helper()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	var activities *candidate.ExtractionActivities
	env.RegisterActivity(activities.Extract)
	env.RegisterActivity(activities.StoreFacts)
	env.RegisterActivity(activities.MarkExtractionOutcome)
	return env
}

func TestSuccessStoresTheExtractedFacts(t *testing.T) {
	env := extractionEnvironment(t)

	facts := []candidate.ExtractedFact{{
		Kind: "skill", Value: json.RawMessage(`{"name":"Go","confidence":0.8}`),
		SpanStart: 10, SpanEnd: 12, Confidence: 0.8, ExtractorVersion: "extract-1",
	}}
	env.OnActivity("Extract", mock.Anything, extractionInput).Return(facts, nil)
	env.OnActivity("StoreFacts", mock.Anything, extractionInput, facts).Return(nil)

	env.ExecuteWorkflow(candidate.ExtractionWorkflow, extractionInput)

	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("workflow did not complete cleanly: %v", env.GetWorkflowError())
	}
	env.AssertExpectations(t)
	env.AssertNotCalled(t, "MarkExtractionOutcome", mock.Anything, mock.Anything, mock.Anything)
}

func TestAnUnsupportedFormatIsAnOutcomeNotAFailure(t *testing.T) {
	// The degradation criterion: a PDF the extractor cannot read completes
	// the workflow with the unsupported state, because "we do not read this
	// yet" is a fact about the product, not a fault in the pipeline.
	env := extractionEnvironment(t)

	refusal := temporal.NewNonRetryableApplicationError(
		"extract-1 cannot read application/pdf", candidate.UnsupportedDocumentCode, nil)
	env.OnActivity("Extract", mock.Anything, extractionInput).Return(nil, refusal)
	env.OnActivity("MarkExtractionOutcome", mock.Anything, extractionInput, "unsupported").Return(nil)

	env.ExecuteWorkflow(candidate.ExtractionWorkflow, extractionInput)

	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("an unsupported format must complete cleanly, got: %v", env.GetWorkflowError())
	}
	env.AssertExpectations(t)
	env.AssertNotCalled(t, "StoreFacts", mock.Anything, mock.Anything, mock.Anything)
}

func TestARealFailureIsRecordedAsFailed(t *testing.T) {
	env := extractionEnvironment(t)

	refusal := temporal.NewNonRetryableApplicationError(
		"the fetched bytes do not match the pinned digest", "FAILURE_CODE_ARTIFACT_NOT_FOUND", nil)
	env.OnActivity("Extract", mock.Anything, extractionInput).Return(nil, refusal)
	env.OnActivity("MarkExtractionOutcome", mock.Anything, extractionInput, "failed").Return(nil)

	env.ExecuteWorkflow(candidate.ExtractionWorkflow, extractionInput)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if env.GetWorkflowError() == nil {
		t.Fatal("a real failure must surface as a workflow error after being recorded")
	}
	env.AssertExpectations(t)
}

func TestAnAlreadyDecidedExtractionEndsQuietly(t *testing.T) {
	// The redelivery path: the guard in Extract refuses a document whose
	// extraction is already decided, and the workflow treats that as the
	// decision standing rather than as anything to record.
	env := extractionEnvironment(t)

	refusal := temporal.NewNonRetryableApplicationError(
		"document extraction is unsupported, not pending", "EXTRACTION_NOT_PENDING", nil)
	env.OnActivity("Extract", mock.Anything, extractionInput).Return(nil, refusal)

	env.ExecuteWorkflow(candidate.ExtractionWorkflow, extractionInput)

	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("an already-decided extraction must end cleanly: %v", env.GetWorkflowError())
	}
	env.AssertNotCalled(t, "StoreFacts", mock.Anything, mock.Anything, mock.Anything)
	env.AssertNotCalled(t, "MarkExtractionOutcome", mock.Anything, mock.Anything, mock.Anything)
}

func TestANonRetryableRefusalIsNotRetried(t *testing.T) {
	env := extractionEnvironment(t)

	calls := 0
	env.OnActivity("Extract", mock.Anything, extractionInput).Return(
		func(_ context.Context, _ candidate.ExtractionInput) ([]candidate.ExtractedFact, error) {
			calls++
			return nil, temporal.NewNonRetryableApplicationError(
				"refused", "FAILURE_CODE_INVALID_INPUT", nil)
		})
	env.OnActivity("MarkExtractionOutcome", mock.Anything, extractionInput, "failed").Return(nil)

	env.ExecuteWorkflow(candidate.ExtractionWorkflow, extractionInput)

	if calls != 1 {
		t.Fatalf("a non-retryable refusal was tried %d times", calls)
	}
	if env.GetWorkflowError() == nil {
		t.Fatal("the failure must surface")
	}
}
