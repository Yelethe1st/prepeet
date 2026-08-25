package main

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetrpc/intelligencev1"
	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetrpc/rpcv1"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
)

// grpcComposer presents the intelligence plane as the Composer interview
// declared: CTR-02's contract on the wire, ADR-0005's translation in cmd.
type grpcComposer struct {
	client intelligencev1.IntelligenceServiceClient
}

// newComposer dials the intelligence plane.
//
// Insecure transport is a local-stack allowance, exactly like the Temporal
// client's: the deployed path gets TLS with PLT-07's workload identity, and
// refusing to start without it would make `make dev` need a CA.
func newComposer(address string) (*grpcComposer, *grpc.ClientConn, error) {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, fmt.Errorf("dialling the intelligence plane: %w", err)
	}
	return &grpcComposer{client: intelligencev1.NewIntelligenceServiceClient(conn)}, conn, nil
}

// Compose asks Python for the session bundle.
func (c *grpcComposer) Compose(ctx context.Context, request interview.ComposeRequest) (interview.ComposeResult, error) {
	purpose := intelligencev1.Purpose_PURPOSE_PRACTICE
	if request.Mode == "screening" {
		purpose = intelligencev1.Purpose_PURPOSE_SCREENING
	}

	response, err := c.client.ComposeSessionBundle(ctx, &intelligencev1.ComposeSessionBundleRequest{
		Context: &intelligencev1.RequestContext{
			SchemaVersion: "1.0",
			// The session id is the idempotency identity, matching the
			// workflow's own: a retried activity re-presents the same request
			// and the capability deduplicates on it.
			RequestId: request.SessionID,
			TenantId:  request.TenantID,
			Purpose:   purpose,
		},
		SessionId:   request.SessionID,
		BlueprintId: request.BlueprintID,
	})
	if err != nil {
		return interview.ComposeResult{}, translateFailure(err)
	}

	bundle := response.GetBundle()
	return interview.ComposeResult{
		BundleRef:      bundle.GetStorageKey(),
		BundleDigest:   bundle.GetDigest(),
		BundleRevision: int(response.GetBundleRevision()),
	}, nil
}

// translateFailure turns a gRPC error into interview's typed refusal.
//
// The retry decision comes from the contract itself: each failure code
// declares retryable as a descriptor option, and retryableCodes reads those
// options at startup. Go holds no second opinion that could drift from
// Python's - both sides read the same compiled descriptor, which was the
// point of putting the decision there.
func translateFailure(err error) error {
	failed, ok := status.FromError(err)
	if !ok {
		return err
	}

	for _, detail := range failed.Details() {
		if failure, ok := detail.(*rpcv1.Failure); ok {
			return &interview.ComposeFailure{
				Code:      failure.GetCode().String(),
				Retryable: retryableCodes[failure.GetCode()],
				Message:   failure.GetMessage(),
			}
		}
	}
	// No typed detail: a transport-level failure, which is the retryable kind
	// by nature - the capability was never reached.
	return err
}

// retryableCodes is the contract's own retry table, read from the descriptor.
var retryableCodes = func() map[rpcv1.FailureCode]bool {
	table := map[rpcv1.FailureCode]bool{}
	values := rpcv1.FailureCode(0).Descriptor().Values()
	for i := 0; i < values.Len(); i++ {
		value := values.Get(i)
		retryable, _ := proto.GetExtension(value.Options(), rpcv1.E_Retryable).(bool)
		table[rpcv1.FailureCode(value.Number())] = retryable
	}
	return table
}()
