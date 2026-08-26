package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	sdkclient "go.temporal.io/sdk/client"

	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// startEvidence turns a session_completed event into the evidence workflow,
// exactly-once by workflow identity.
func startEvidence(workflows sdkclient.Client) outbox.HandlerFunc {
	return func(ctx context.Context, event outbox.Pending) error {
		var payload struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decoding %s: %w", event.Type, err)
		}

		_, err := workflows.ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
			ID:                    "evidence-" + payload.SessionID,
			TaskQueue:             evaluation.TaskQueue,
			WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		}, evaluation.EvidenceWorkflow, evaluation.EvidenceInput{
			SessionID:   payload.SessionID,
			Mode:        event.Purpose,
			CandidateID: event.Actor.ID,
			TenantID:    event.TenantID,
		})

		var already *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &already) {
			return nil
		}
		return err
	}
}

// recordEvaluationFailure moves the session's state machine when the
// evaluation context reports failure: the cross-context act lives here in
// cmd, the one place that sees both.
func recordEvaluationFailure(sessions *interview.Store) outbox.HandlerFunc {
	return func(ctx context.Context, event outbox.Pending) error {
		var payload struct {
			SessionID string `json:"session_id"`
			Failure   string `json:"failure"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decoding %s: %w", event.Type, err)
		}

		session, err := sessions.Get(ctx, payload.SessionID, event.Purpose, event.Actor.ID, event.TenantID)
		if err != nil {
			return err
		}
		if session.State != interview.StateEvaluating {
			return nil // already decided; the surplus delivery ends quietly
		}
		actor := interview.Actor{ID: event.Actor.ID, Type: "service"}
		_, err = sessions.Transition(ctx, session, interview.StateEvaluationFailed,
			interview.Effects{FailureCode: payload.Failure}, actor)
		if errors.Is(err, interview.ErrStaleVersion) {
			return nil
		}
		return err
	}
}
