package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	sdkclient "go.temporal.io/sdk/client"

	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// startComposition turns a session_created event into a running composition
// workflow.
//
// The api already moved the session to composing in the creation flow; what
// must survive a crash is the workflow start, and the outbox is what makes
// it survive: an api process that died after commit still has the event, and
// the worker retries from here. The workflow id is the session id, so a
// redelivery collides and is treated as delivered, and the reuse policy
// rejects a restart after completion - a finished composition's outcome
// stands until an authorized retry moves the state machine, not until a
// stale event wanders back.
func startComposition(workflows sdkclient.Client, sessions *interview.Store) outbox.HandlerFunc {
	return func(ctx context.Context, event outbox.Pending) error {
		var payload struct {
			SessionID   string `json:"session_id"`
			CandidateID string `json:"candidate_id"`
			Mode        string `json:"mode"`
			BlueprintID string `json:"blueprint_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decoding %s: %w", event.Type, err)
		}

		// The session decides, not the event: a session already past
		// composing keeps its outcome, and one still in draft (the api died
		// between create and its composing transition) is moved here so the
		// workflow's own transitions start from the state they expect.
		session, err := sessions.Get(ctx, payload.SessionID, payload.Mode, payload.CandidateID, event.TenantID)
		if err != nil {
			return err
		}
		switch session.State {
		case interview.StateDraft:
			actor := interview.Actor{ID: payload.CandidateID, Type: "service"}
			if session, err = sessions.Transition(ctx, session, interview.StateComposing,
				interview.Effects{}, actor); err != nil {
				return err
			}
		case interview.StateComposing:
			// The normal path: the api moved it, the workflow start is ours.
		default:
			return nil // already decided; the surplus delivery ends quietly
		}

		_, err = workflows.ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
			ID:                    "compose-" + session.ID,
			TaskQueue:             interview.TaskQueue,
			WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		}, interview.CompositionWorkflow, interview.CompositionInput{
			SessionID:   session.ID,
			Mode:        session.Mode,
			CandidateID: session.CandidateID,
			TenantID:    session.TenantID,
			CampaignID:  session.CampaignID,
			BlueprintID: session.BlueprintID,
			ActorID:     session.CandidateID,
		})

		var already *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &already) {
			return nil
		}
		return err
	}
}
