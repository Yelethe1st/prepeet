package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	sdkclient "go.temporal.io/sdk/client"

	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// startExtraction turns a document_uploaded event into a running extraction
// workflow.
//
// The workflow id is the document id, so the outbox's at-least-once becomes
// exactly-one-extraction: a redelivery collides with the id and is treated as
// delivered. The reuse policy rejects a restart after completion too, because
// a finished extraction's outcome stands until a new upload - which is a new
// document id - not until a stale event wanders back.
func startExtraction(workflows sdkclient.Client) outbox.HandlerFunc {
	return func(ctx context.Context, event outbox.Pending) error {
		var payload struct {
			DocumentID  string `json:"document_id"`
			CandidateID string `json:"candidate_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decoding %s: %w", event.Type, err)
		}

		_, err := workflows.ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
			ID:                    "extract-" + payload.DocumentID,
			TaskQueue:             candidate.ExtractionTaskQueue,
			WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		}, candidate.ExtractionWorkflow, candidate.ExtractionInput{
			DocumentID:  payload.DocumentID,
			CandidateID: payload.CandidateID,
		})

		var already *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &already) {
			return nil
		}
		return err
	}
}
