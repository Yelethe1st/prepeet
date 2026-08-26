package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetrpc/intelligencev1"
	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetrpc/rpcv1"
	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
)

// grpcEvidence presents the intelligence plane as the Extractor evaluation
// declared. The one place allowed to see the seal, the object store and
// the wire together: it reads the seal for the pinned digest, presigns the
// sealed input's own key, has Python extract over verified bytes, and
// hands the activity both the document and the spans so validation runs
// against exactly what was served.
type grpcEvidence struct {
	client    intelligencev1.IntelligenceServiceClient
	store     *objectstore.S3Store
	completer *interview.Completer
}

const evidenceFetchTTL = 5 * time.Minute

func newEvidence(conn *grpc.ClientConn, store *objectstore.S3Store, completer *interview.Completer) *grpcEvidence {
	return &grpcEvidence{
		client:    intelligencev1.NewIntelligenceServiceClient(conn),
		store:     store,
		completer: completer,
	}
}

func (e *grpcEvidence) Extract(ctx context.Context, ref evaluation.SessionRef) (evaluation.SealedInput, []evaluation.Span, error) {
	seal, err := e.completer.SealOf(ctx, ref.SessionID, ref.Mode, ref.CandidateID, ref.TenantID)
	if err != nil {
		return evaluation.SealedInput{}, nil, fmt.Errorf("reading the seal: %w", err)
	}
	if seal.EvaluationInputDigest == "" {
		return evaluation.SealedInput{}, nil, &evaluation.ExtractFailure{
			Code: "FAILURE_CODE_ARTIFACT_NOT_FOUND", Retryable: false,
			Message: "the seal records no evaluation input object",
		}
	}

	key, err := objectstore.SealedInputKey(ref.Mode, ref.TenantID, ref.CandidateID, ref.SessionID)
	if err != nil {
		return evaluation.SealedInput{}, nil, err
	}
	fetchURL, err := e.store.PresignPlayback(ctx, key, evidenceFetchTTL)
	if err != nil {
		return evaluation.SealedInput{}, nil, fmt.Errorf("presigning the sealed input: %w", err)
	}

	response, err := e.client.EvaluateTurns(ctx, &intelligencev1.EvaluateTurnsRequest{
		Context: &intelligencev1.RequestContext{
			SchemaVersion: "1.0",
			RequestId:     ref.SessionID,
			TenantId:      ref.TenantID,
		},
		SessionId:    ref.SessionID,
		BundleDigest: seal.BundleDigest,
		Turns: []*intelligencev1.ObjectRef{{
			StorageKey: key.String(),
			Digest:     seal.EvaluationInputDigest,
			MediaType:  "application/json",
			FetchUrl:   fetchURL,
		}},
	})
	if err != nil {
		return evaluation.SealedInput{}, nil, translateEvidenceFailure(err)
	}

	spans := make([]evaluation.Span, 0, len(response.GetObservations()))
	for _, observation := range response.GetObservations() {
		var decoded struct {
			Kind              string `json:"kind"`
			Quote             string `json:"quote"`
			SegmentSequence   int    `json:"segment_sequence"`
			CharStart         int    `json:"char_start"`
			CharEnd           int    `json:"char_end"`
			StartMs           int    `json:"start_ms"`
			EndMs             int    `json:"end_ms"`
			ExtractionVersion string `json:"extraction_version"`
		}
		if err := json.Unmarshal(observation.GetObservation(), &decoded); err != nil {
			return evaluation.SealedInput{}, nil, &evaluation.ExtractFailure{
				Code: "FAILURE_CODE_SCHEMA_VALIDATION_FAILED", Retryable: false,
				Message: fmt.Sprintf("an observation does not decode: %v", err),
			}
		}
		spans = append(spans, evaluation.Span{
			CompetencyID: observation.GetCompetencyId(), Kind: decoded.Kind,
			SegmentSequence: decoded.SegmentSequence, Quote: decoded.Quote,
			CharStart: decoded.CharStart, CharEnd: decoded.CharEnd,
			StartMs: decoded.StartMs, EndMs: decoded.EndMs,
			ExtractionVersion: decoded.ExtractionVersion,
		})
	}

	// The document, fetched server-side for validation: the same bytes the
	// digest pinned, so the honesty gate judges what Python actually read.
	body, err := e.store.Fetch(ctx, key)
	if err != nil {
		return evaluation.SealedInput{}, nil, fmt.Errorf("fetching the sealed input: %w", err)
	}
	sealed, err := evaluation.DecodeSealedInput(body)
	if err != nil {
		return evaluation.SealedInput{}, nil, err
	}
	return sealed, spans, nil
}

// translateEvidenceFailure carries the contract's own retry decision.
func translateEvidenceFailure(err error) error {
	failed, ok := status.FromError(err)
	if !ok {
		return err
	}
	for _, detail := range failed.Details() {
		if failure, ok := detail.(*rpcv1.Failure); ok {
			return &evaluation.ExtractFailure{
				Code:      failure.GetCode().String(),
				Retryable: retryableCodes[failure.GetCode()],
				Message:   failure.GetMessage(),
			}
		}
	}
	return err
}
