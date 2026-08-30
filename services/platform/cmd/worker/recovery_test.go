package main

import (
	"context"
	"sync"
	"testing"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	sdkclient "go.temporal.io/sdk/client"

	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// The property OPS-03's first criterion rests on, at the seam where it is
// decided: a delivery that happens twice starts one workflow.
//
// Everything above this - the operator's retry, the transition that only fires
// once, the dispatcher's at-least-once claim - is a reason a duplicate delivery
// is unlikely. This is the reason it does not matter. The workflow id is
// derived from the entity and the reuse policy rejects a duplicate, so the
// second start is refused by Temporal rather than by anybody remembering to
// check.

// temporalStub is a Temporal frontend that enforces exactly the rule this
// design depends on, and nothing else.
//
// It embeds the client interface rather than implementing it: the handlers
// under test call ExecuteWorkflow and nothing else, and a stub that implements
// forty methods to be honest about one is a stub nobody reads. Any other call
// panics on the nil interface, which is the correct answer to "this test
// started depending on something it does not model".
//
// The important part is that it obeys the policy rather than assuming it. A
// start with WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE against an id that is
// already there is refused; a start under any other policy is allowed through
// and counted. So if the production handler ever stops asking for duplicate
// rejection, this stub starts the workflow twice and the assertions below fail,
// which is the only way an assertion about a policy can be worth making.
type temporalStub struct {
	sdkclient.Client

	mu      sync.Mutex
	starts  map[string]int
	options map[string]sdkclient.StartWorkflowOptions
}

func newTemporalStub() *temporalStub {
	return &temporalStub{
		starts:  map[string]int{},
		options: map[string]sdkclient.StartWorkflowOptions{},
	}
}

func (s *temporalStub) ExecuteWorkflow(_ context.Context, options sdkclient.StartWorkflowOptions,
	_ any, _ ...any) (sdkclient.WorkflowRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, running := s.starts[options.ID]; running &&
		options.WorkflowIDReusePolicy == enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE {
		return nil, serviceerror.NewWorkflowExecutionAlreadyStarted(
			"workflow execution already started", options.ID, "run-1")
	}

	s.starts[options.ID]++
	s.options[options.ID] = options
	// A nil run is enough: the handlers deliberately do not wait on the
	// workflow, because a handler that waited would hold its outbox claim for
	// the length of an evaluation.
	return nil, nil
}

func (s *temporalStub) startsOf(workflowID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.starts[workflowID]
}

// completion is the event a finished session publishes.
const completedSession = "00000000-0000-7000-8000-0000000000a7"

func completionEvent() outbox.Pending {
	return outbox.Pending{
		ID:            "00000000-0000-7000-8000-0000000000a8",
		Type:          "interview.session_completed.v1",
		SchemaVersion: "1.0",
		Producer:      "interview",
		Purpose:       "practice",
		Actor:         outbox.Actor{Type: "service", ID: "00000000-0000-7000-8000-0000000000a9"},
		Payload: []byte(`{"session_id":"` + completedSession +
			`","completion":"completed","turn_count":8,"duration_seconds":600}`),
	}
}

func TestARedeliveredCompletionStartsOneEvaluation(t *testing.T) {
	t.Parallel()

	stub := newTemporalStub()
	deliver := startEvidence(stub)
	event := completionEvent()

	for attempt := range 2 {
		if err := deliver(context.Background(), event); err != nil {
			t.Fatalf("delivery %d: %v", attempt+1, err)
		}
	}

	if starts := stub.startsOf("evidence-" + completedSession); starts != 1 {
		t.Errorf("two deliveries started %d evaluations, want exactly one", starts)
	}
}

// The two properties that make the above true rather than lucky: the id is the
// entity's, so a redelivery collides with itself, and the policy is the one
// that refuses a collision. Asserted separately because a test that only counts
// starts would pass against a stub that had quietly stopped enforcing anything.
func TestTheEvaluationWorkflowIsIdentifiedByItsSession(t *testing.T) {
	t.Parallel()

	stub := newTemporalStub()
	if err := startEvidence(stub)(context.Background(), completionEvent()); err != nil {
		t.Fatalf("delivery: %v", err)
	}

	options, started := stub.options["evidence-"+completedSession]
	if !started {
		t.Fatal("no workflow was started under an id derived from the session")
	}
	if options.WorkflowIDReusePolicy != enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE {
		t.Errorf("reuse policy is %v, want REJECT_DUPLICATE: without it a retry evaluates twice",
			options.WorkflowIDReusePolicy)
	}
}
