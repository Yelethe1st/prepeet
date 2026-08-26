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
	"github.com/Yelethe1st/prepeet/services/platform/internal/progression"
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

// recordEvaluationCompleted moves the session to review_ready when its
// evaluation lands: EVL-02's notification consumed as the state machine's
// input, in the one place that sees both contexts.
func recordEvaluationCompleted(sessions *interview.Store) outbox.HandlerFunc {
	return func(ctx context.Context, event outbox.Pending) error {
		var payload struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decoding %s: %w", event.Type, err)
		}

		session, err := sessions.Get(ctx, payload.SessionID, event.Purpose, event.Actor.ID, event.TenantID)
		if err != nil {
			return err
		}
		if session.State != interview.StateEvaluating {
			return nil // already moved; the surplus delivery ends quietly
		}
		actor := interview.Actor{ID: event.Actor.ID, Type: "service"}
		_, err = sessions.Transition(ctx, session, interview.StateReviewReady, interview.Effects{}, actor)
		if errors.Is(err, interview.ErrStaleVersion) {
			return nil
		}
		return err
	}
}

// appendObservations projects one published evaluation into the
// candidate's append-only competency history (PRG-01), in the one place
// that may see both contexts. Idempotent end to end: a redelivered event
// re-reads the same immutable result and converges on the unique
// (evaluation, competency) rows.
func appendObservations(results *evaluation.Store, history *progression.Store) outbox.HandlerFunc {
	return func(ctx context.Context, event outbox.Pending) error {
		var payload struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decoding %s: %w", event.Type, err)
		}

		result, err := results.ResultOf(ctx, evaluation.SessionRef{
			SessionID: payload.SessionID, Mode: event.Purpose,
			CandidateID: event.Actor.ID, TenantID: event.TenantID,
		})
		if err != nil {
			return err
		}

		observations := make([]progression.Observation, 0, len(result.Aggregation.Competencies))
		for _, competency := range result.Aggregation.Competencies {
			observations = append(observations, progression.Observation{
				SessionID: result.SessionID, EvaluationID: result.ID,
				CompetencyID: competency.CompetencyID,
				Status:       competency.Status, Band: competency.Band,
				Confidence:    competency.Confidence,
				EvidenceCount: competency.EvidenceCount,
				Supporting:    competency.Supporting, Contradictory: competency.Contradictory,
				Unverified: competency.Unverified, Gaps: competency.Gaps,
				RubricReference: result.RubricReference, RubricVersion: result.RubricVersion,
				RubricDigest:       result.RubricDigest,
				AggregationVersion: result.AggregationVersion,
				ExtractionVersion:  result.ExtractionVersion,
				ModelVersion:       result.ModelVersion, PolicyVersion: result.PolicyVersion,
				ObservedAt: result.CreatedAt,
			})
		}
		return history.Append(ctx, progression.Owner{
			Mode: event.Purpose, CandidateID: event.Actor.ID, TenantID: event.TenantID,
		}, observations)
	}
}
