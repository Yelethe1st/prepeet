package interview_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
)

// The grace workflow's decisions, under Temporal's test environment.
//
// What is asserted is the control flow: that the timer waits out exactly the
// window the activity reports, that a session which recovered stands the
// timer down without touching anything, and that a window already lapsed is
// finalized without a pointless sleep. The activities' own behaviour against
// the database is the integration suite's subject.

var graceInput = interview.GraceInput{
	SessionID:   "00000000-0000-7000-8000-0000000000f1",
	Mode:        "practice",
	CandidateID: "00000000-0000-7000-8000-0000000000f2",
	ActorID:     "00000000-0000-7000-8000-0000000000f2",
}

func graceEnvironment(t *testing.T) *testsuite.TestWorkflowEnvironment {
	t.Helper()
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()
	var activities *interview.GraceActivities
	env.RegisterActivity(activities.RemainingGrace)
	env.RegisterActivity(activities.ExpireGrace)
	return env
}

func TestGraceWaitsOutTheWindowThenExpires(t *testing.T) {
	env := graceEnvironment(t)

	env.OnActivity("RemainingGrace", mock.Anything, graceInput).
		Return(interview.GraceCheck{Reconnecting: true, RemainingSeconds: 90}, nil)
	env.OnActivity("ExpireGrace", mock.Anything, graceInput).Return(nil)

	env.ExecuteWorkflow(interview.GraceWorkflow, graceInput)

	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("workflow did not complete cleanly: %v", env.GetWorkflowError())
	}
	env.AssertExpectations(t)
}

func TestGraceStandsDownWhenTheSessionRecovered(t *testing.T) {
	env := graceEnvironment(t)

	// The session resumed, or completed, before the timer ever slept: the
	// workflow ends without expiring anything.
	env.OnActivity("RemainingGrace", mock.Anything, graceInput).
		Return(interview.GraceCheck{Reconnecting: false}, nil)

	env.ExecuteWorkflow(interview.GraceWorkflow, graceInput)

	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("workflow did not complete cleanly: %v", env.GetWorkflowError())
	}
	env.AssertNotCalled(t, "ExpireGrace", mock.Anything, mock.Anything)
}

func TestGraceExpiresImmediatelyWhenAlreadyDue(t *testing.T) {
	env := graceEnvironment(t)

	// A worker that was down past the whole window has nothing to wait for.
	env.OnActivity("RemainingGrace", mock.Anything, graceInput).
		Return(interview.GraceCheck{Reconnecting: true, RemainingSeconds: 0}, nil)
	env.OnActivity("ExpireGrace", mock.Anything, graceInput).Return(nil)

	env.ExecuteWorkflow(interview.GraceWorkflow, graceInput)

	if !env.IsWorkflowCompleted() || env.GetWorkflowError() != nil {
		t.Fatalf("workflow did not complete cleanly: %v", env.GetWorkflowError())
	}
	env.AssertExpectations(t)
}
