package interview_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
)

// The workflow's decisions, under Temporal's test environment.
//
// What is asserted is the control flow the activities cannot see from inside
// themselves: that success marks ready with the composed bundle, that a
// refusal marks failed with the refusal's own code rather than a generic one,
// and that a non-retryable refusal is not retried. The activities' own
// behaviour against the database is the integration suite's subject.

var input = interview.CompositionInput{
	SessionID:   "00000000-0000-7000-8000-0000000000f1",
	Mode:        "practice",
	CandidateID: "00000000-0000-7000-8000-0000000000f2",
	BlueprintID: "bp_backend_v1",
	ActorID:     "00000000-0000-7000-8000-0000000000f2",
}

func environment(t *testing.T) *testsuite.TestWorkflowEnvironment {
	t.Helper()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	var activities *interview.Activities
	env.RegisterActivity(activities.Compose)
	env.RegisterActivity(activities.MarkReady)
	env.RegisterActivity(activities.MarkFailed)
	return env
}

func TestSuccessMarksReadyWithTheComposedBundle(t *testing.T) {
	env := environment(t)

	composed := interview.ComposeResult{
		BundleRef: "bundles/f1", BundleDigest: "sha256:d1", BundleRevision: 1,
	}
	env.OnActivity("Compose", mock.Anything, input).Return(composed, nil)
	env.OnActivity("MarkReady", mock.Anything, input, composed).Return(nil)

	env.ExecuteWorkflow(interview.CompositionWorkflow, input)

	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("workflow did not complete cleanly: %v", env.GetWorkflowError())
	}
	env.AssertExpectations(t)
	env.AssertNotCalled(t, "MarkFailed", mock.Anything, mock.Anything, mock.Anything)
}

func TestARefusalMarksFailedWithItsOwnCode(t *testing.T) {
	env := environment(t)

	// The code travels from the contract's taxonomy through the typed error
	// into the session row, where an operator reads it before deciding whether
	// retry is worth it. A generic code here would erase that judgment.
	refusal := temporal.NewNonRetryableApplicationError("no such blueprint",
		"FAILURE_CODE_ARTIFACT_NOT_FOUND", nil)
	env.OnActivity("Compose", mock.Anything, input).Return(interview.ComposeResult{}, refusal).Once()
	env.OnActivity("MarkFailed", mock.Anything, input, "FAILURE_CODE_ARTIFACT_NOT_FOUND").Return(nil)

	env.ExecuteWorkflow(interview.CompositionWorkflow, input)

	if env.GetWorkflowError() == nil {
		t.Fatal("a failed composition completed as success")
	}
	// Once() above is the non-retry assertion: a second Compose call would
	// fail the mock before it failed this.
	env.AssertExpectations(t)
	env.AssertNotCalled(t, "MarkReady", mock.Anything, mock.Anything, mock.Anything)
}

func TestExhaustedRetriesStillRecordTheFailure(t *testing.T) {
	env := environment(t)

	// A provider outage: retryable, and eventually out of attempts. However it
	// ends, the session must not be left in composing - a session stuck there
	// because the failure was never written is one nobody can retry from the
	// interface.
	env.OnActivity("Compose", mock.Anything, input).
		Return(interview.ComposeResult{}, errors.New("provider unreachable"))
	env.OnActivity("MarkFailed", mock.Anything, input, "COMPOSITION_FAILED").Return(nil)

	env.ExecuteWorkflow(interview.CompositionWorkflow, input)

	if env.GetWorkflowError() == nil {
		t.Fatal("an exhausted composition completed as success")
	}
	env.AssertExpectations(t)
}

// The activity's translation of the contract's taxonomy.
func TestComposeTranslatesANonRetryableRefusal(t *testing.T) {
	activities := interview.NewActivities(nil, refusingComposer{})

	_, err := activities.Compose(context.Background(), input)

	var applicationErr *temporal.ApplicationError
	if !errors.As(err, &applicationErr) {
		t.Fatalf("a typed refusal came back as %T", err)
	}
	if !applicationErr.NonRetryable() {
		t.Fatal("a non-retryable refusal was left retryable; the provider bill disagrees")
	}
	if applicationErr.Type() != "FAILURE_CODE_UNSUPPORTED_POLICY_VERSION" {
		t.Fatalf("the code was rewritten to %q", applicationErr.Type())
	}
}

type refusingComposer struct{}

func (refusingComposer) Compose(context.Context, interview.ComposeRequest) (interview.ComposeResult, error) {
	return interview.ComposeResult{}, &interview.ComposeFailure{
		Code:      "FAILURE_CODE_UNSUPPORTED_POLICY_VERSION",
		Retryable: false,
		Message:   "this deployment does not carry policy v9",
	}
}
