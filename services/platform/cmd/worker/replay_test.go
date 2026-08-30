package main

import (
	"context"
	"encoding/json"
	"testing"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	sdkclient "go.temporal.io/sdk/client"

	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// INT-02's last criterion: replaying a delivery does not duplicate its effect.
//
// At-least-once means a consumer will see the same event twice. It happens
// when a dispatcher dies between the handler succeeding and the row being
// marked, which is a window that cannot be closed, only made harmless.
//
// The mechanism is the same in every route that starts a workflow: an id
// derived from the entity, REJECT_DUPLICATE, and the resulting
// WorkflowExecutionAlreadyStarted swallowed as success. Each of those three is
// load bearing and none is obvious, so each is asserted here rather than left
// to a reading of the code.

// fakeWorkflows records what was started and refuses a repeat the way Temporal
// does under REJECT_DUPLICATE.
//
// The interface is embedded rather than implemented: every method this test
// does not need is nil, so reaching one panics instead of quietly returning a
// zero value. A test that calls something it did not mean to should say so.
type fakeWorkflows struct {
	sdkclient.Client
	started  []string
	policies []sdkclient.StartWorkflowOptions
}

func (f *fakeWorkflows) ExecuteWorkflow(
	_ context.Context, options sdkclient.StartWorkflowOptions, _ any, _ ...any,
) (sdkclient.WorkflowRun, error) {
	f.policies = append(f.policies, options)
	for _, seen := range f.started {
		if seen == options.ID {
			// What the server answers for a second start of the same id under
			// REJECT_DUPLICATE.
			return nil, serviceerror.NewWorkflowExecutionAlreadyStarted(
				"already started", "", options.ID)
		}
	}
	f.started = append(f.started, options.ID)
	return nil, nil
}

func event(t *testing.T, eventType string, payload any) outbox.Pending {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encoding the payload: %v", err)
	}
	// A realistic row rather than a bare type and payload. The id and the
	// attempt count are what a replay actually varies, and leaving them zero
	// made an earlier version of these tests pass against a workflow id built
	// from `event.ID`: the probe changed nothing because the field was empty.
	// Stamped practice, because a real completion is. The articulation route
	// refuses anything else, and a fixture with no purpose would test the
	// refusal rather than the replay behaviour it is here for.
	return outbox.Pending{
		ID: "01a0301d-aa10-7000-8f3e-000000000001", Type: eventType,
		SchemaVersion: "1.0", Producer: "api", Purpose: "practice", Payload: body,
	}
}

// The routes that start a workflow straight from the event, which is where the
// replay guarantee has to hold on its own.
func workflowStartingRoutes() map[string]struct {
	handler func(sdkclient.Client) outbox.HandlerFunc
	event   func(t *testing.T) outbox.Pending
	wantID  string
} {
	return map[string]struct {
		handler func(sdkclient.Client) outbox.HandlerFunc
		event   func(t *testing.T) outbox.Pending
		wantID  string
	}{
		"extraction": {
			handler: startExtraction,
			event: func(t *testing.T) outbox.Pending {
				return event(t, "candidate.document_uploaded.v1", map[string]string{
					"document_id": "doc-1", "candidate_id": "cand-1",
				})
			},
			wantID: "extract-doc-1",
		},
		"evidence": {
			handler: startEvidence,
			event: func(t *testing.T) outbox.Pending {
				return event(t, "interview.session_completed.v1", map[string]string{
					"session_id": "ses-1",
				})
			},
			wantID: "evidence-ses-1",
		},
		"articulation": {
			handler: startArticulation,
			event: func(t *testing.T) outbox.Pending {
				return event(t, "interview.session_completed.v1", map[string]string{
					"session_id": "ses-1",
				})
			},
			wantID: "articulation-ses-1",
		},
	}
}

func TestReplayingADeliveryStartsTheWorkflowOnce(t *testing.T) {
	for name, route := range workflowStartingRoutes() {
		t.Run(name, func(t *testing.T) {
			workflows := &fakeWorkflows{}
			handle := route.handler(workflows)
			delivery := route.event(t)

			// The same row, delivered twice, which is what at-least-once means
			// and what a dispatcher dying between the handler succeeding and
			// the row being marked actually produces. The attempt count is the
			// one thing that differs, which is exactly why nothing derived
			// from it may reach the workflow id.
			if err := handle(context.Background(), delivery); err != nil {
				t.Fatalf("first delivery: %v", err)
			}
			replayed := delivery
			replayed.Attempts = delivery.Attempts + 1
			if err := handle(context.Background(), replayed); err != nil {
				// The second must succeed, not merely be harmless. An error
				// here would make the outbox retry the row forever and
				// eventually dead letter an event that was fully handled.
				t.Fatalf("replayed delivery: %v", err)
			}

			if len(workflows.started) != 1 {
				t.Fatalf("started %d workflows for one event: %v",
					len(workflows.started), workflows.started)
			}
		})
	}
}

// The id is what makes the duplicate detectable. Derived from anything that
// varies per delivery, REJECT_DUPLICATE would never fire and the test above
// would pass while the property was gone.
func TestTheWorkflowIdIsDerivedFromTheEntity(t *testing.T) {
	for name, route := range workflowStartingRoutes() {
		t.Run(name, func(t *testing.T) {
			workflows := &fakeWorkflows{}

			if err := route.handler(workflows)(context.Background(), route.event(t)); err != nil {
				t.Fatalf("delivery: %v", err)
			}

			if len(workflows.policies) != 1 {
				t.Fatalf("want one start, got %d", len(workflows.policies))
			}
			if got := workflows.policies[0].ID; got != route.wantID {
				t.Fatalf("workflow id = %q, want %q derived from the entity", got, route.wantID)
			}
		})
	}
}

// Without the policy the second start would begin a second run rather than be
// refused, and the handler's own duplicate check would never be reached.
func TestEveryWorkflowStartRefusesADuplicateId(t *testing.T) {
	for name, route := range workflowStartingRoutes() {
		t.Run(name, func(t *testing.T) {
			workflows := &fakeWorkflows{}

			if err := route.handler(workflows)(context.Background(), route.event(t)); err != nil {
				t.Fatalf("delivery: %v", err)
			}

			policy := workflows.policies[0].WorkflowIDReusePolicy
			if policy != enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE {
				t.Fatalf("reuse policy is %s, which lets a replay start a second run", policy)
			}
		})
	}
}

// A malformed payload must fail rather than be swallowed. Returning nil would
// mark the row delivered and lose the event in the quietest possible way,
// which is the failure the router's unknown-type rule already refuses.
func TestAnUndecodablePayloadFailsRatherThanBeingMarkedDone(t *testing.T) {
	workflows := &fakeWorkflows{}

	err := startExtraction(workflows)(context.Background(), outbox.Pending{
		Type: "candidate.document_uploaded.v1", Payload: []byte("{not json"),
	})

	if err == nil {
		t.Fatal("a malformed payload was reported as handled")
	}
	if len(workflows.started) != 0 {
		t.Fatal("a workflow was started from a payload that could not be read")
	}
}

// Regression from the ART review: startArticulation forwarded whatever mode
// the completion event carried, so a completed screening session whose pinned
// policy included the articulation stage would have had its transcript turned
// into delivery measurements and stored under a tenant scope. The product says
// in candidate-facing copy that screening never produces delivery coaching.
func TestArticulationIgnoresAScreeningCompletion(t *testing.T) {
	workflows := &fakeWorkflows{}
	delivery := event(t, "interview.session_completed.v1", map[string]string{
		"session_id": "ses-1",
	})
	delivery.Purpose = "screening"

	// Handled, not failed: a screening completion is nothing to do here, and
	// returning an error would retry it forever and then dead letter it.
	if err := startArticulation(workflows)(context.Background(), delivery); err != nil {
		t.Fatalf("a screening completion was reported as a failure: %v", err)
	}

	if len(workflows.started) != 0 {
		t.Fatalf("delivery analysis began for a screening session: %v", workflows.started)
	}
}

func TestArticulationStillRunsForPractice(t *testing.T) {
	workflows := &fakeWorkflows{}
	if err := startArticulation(workflows)(context.Background(),
		event(t, "interview.session_completed.v1", map[string]string{"session_id": "ses-1"})); err != nil {
		t.Fatalf("practice: %v", err)
	}

	if len(workflows.started) != 1 {
		t.Fatalf("practice delivery analysis did not start: %v", workflows.started)
	}
}

// A purpose nobody set is not practice. Defaulting to running would make the
// guard depend on every producer remembering to stamp the field.
func TestArticulationIgnoresACompletionWithNoPurpose(t *testing.T) {
	workflows := &fakeWorkflows{}

	unstamped := event(t, "interview.session_completed.v1", map[string]string{"session_id": "ses-1"})
	unstamped.Purpose = ""

	if err := startArticulation(workflows)(context.Background(), unstamped); err != nil {
		t.Fatalf("no purpose: %v", err)
	}

	if len(workflows.started) != 0 {
		t.Fatalf("delivery analysis began for an unstamped completion: %v", workflows.started)
	}
}
