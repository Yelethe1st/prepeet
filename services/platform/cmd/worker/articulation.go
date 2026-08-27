package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	sdkclient "go.temporal.io/sdk/client"
	"google.golang.org/grpc"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetrpc/intelligencev1"
	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
	"github.com/Yelethe1st/prepeet/services/platform/platform/outbox"
)

// grpcArticulation presents the intelligence plane as the Analyzer the
// evaluation context declared: the seal for the pinned digest, a presigned
// read of the sealed input, and Python's measurement over verified bytes.
type grpcArticulation struct {
	client    intelligencev1.IntelligenceServiceClient
	store     *objectstore.S3Store
	completer *interview.Completer
}

func newArticulation(conn *grpc.ClientConn, store *objectstore.S3Store, completer *interview.Completer) *grpcArticulation {
	return &grpcArticulation{
		client: intelligencev1.NewIntelligenceServiceClient(conn), store: store, completer: completer,
	}
}

func (a *grpcArticulation) Analyze(ctx context.Context, ref evaluation.SessionRef) (evaluation.Analysis, error) {
	seal, err := a.completer.SealOf(ctx, ref.SessionID, ref.Mode, ref.CandidateID, ref.TenantID)
	if err != nil {
		return evaluation.Analysis{}, fmt.Errorf("reading the seal: %w", err)
	}
	if seal.EvaluationInputDigest == "" {
		return evaluation.Analysis{}, &evaluation.ExtractFailure{
			Code: "FAILURE_CODE_ARTIFACT_NOT_FOUND", Retryable: false,
			Message: "the seal records no evaluation input object",
		}
	}
	key, err := objectstore.SealedInputKey(ref.Mode, ref.TenantID, ref.CandidateID, ref.SessionID)
	if err != nil {
		return evaluation.Analysis{}, err
	}
	fetchURL, err := a.store.PresignPlayback(ctx, key, evidenceFetchTTL)
	if err != nil {
		return evaluation.Analysis{}, fmt.Errorf("presigning the sealed input: %w", err)
	}

	response, err := a.client.AnalyzeArticulation(ctx, &intelligencev1.AnalyzeArticulationRequest{
		Context: &intelligencev1.RequestContext{
			SchemaVersion: "1.0", RequestId: ref.SessionID, TenantId: ref.TenantID,
		},
		SessionId:    ref.SessionID,
		BundleDigest: seal.BundleDigest,
		Manifest: &intelligencev1.ObjectRef{
			StorageKey: key.String(), Digest: seal.EvaluationInputDigest,
			MediaType: "application/json", FetchUrl: fetchURL,
		},
	})
	if err != nil {
		return evaluation.Analysis{}, translateEvidenceFailure(err)
	}

	var decoded struct {
		Assessability struct {
			Status   string   `json:"status"`
			Warnings []string `json:"warnings"`
		} `json:"assessability"`
	}
	if err := json.Unmarshal(response.GetAnalysis(), &decoded); err != nil {
		return evaluation.Analysis{}, &evaluation.ExtractFailure{
			Code: "FAILURE_CODE_SCHEMA_VALIDATION_FAILED", Retryable: false,
			Message: fmt.Sprintf("the analysis does not decode: %v", err),
		}
	}
	return evaluation.Analysis{
		Status: decoded.Assessability.Status, Warnings: decoded.Assessability.Warnings,
		Document:           json.RawMessage(response.GetAnalysis()),
		CalculationVersion: response.GetMeta().GetCalculationVersion(),
		PolicyVersion:      response.GetMeta().GetPolicyVersion(),
		InputDigest:        seal.EvaluationInputDigest,
	}, nil
}

// startArticulation turns session_completed into the delivery workflow,
// exactly once per session, under its own workflow id: a failure here is
// this workflow's alone, and EvidenceWorkflow neither waits for it nor
// hears of it.
func startArticulation(workflows sdkclient.Client) outbox.HandlerFunc {
	return func(ctx context.Context, event outbox.Pending) error {
		var payload struct {
			SessionID string `json:"session_id"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decoding %s: %w", event.Type, err)
		}
		_, err := workflows.ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
			ID:                    "articulation-" + payload.SessionID,
			TaskQueue:             evaluation.TaskQueue,
			WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		}, evaluation.ArticulationWorkflow, evaluation.EvidenceInput{
			SessionID: payload.SessionID, Mode: event.Purpose,
			CandidateID: event.Actor.ID, TenantID: event.TenantID,
		})
		var already *serviceerror.WorkflowExecutionAlreadyStarted
		if errors.As(err, &already) {
			return nil
		}
		return err
	}
}
