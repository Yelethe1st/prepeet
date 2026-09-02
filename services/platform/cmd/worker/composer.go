package main

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetrpc/intelligencev1"
	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetrpc/rpcv1"
	"github.com/Yelethe1st/prepeet/services/platform/internal/content"
	"github.com/Yelethe1st/prepeet/services/platform/internal/evaluation"
	"github.com/Yelethe1st/prepeet/services/platform/internal/interview"
	"github.com/Yelethe1st/prepeet/services/platform/internal/recruiting"
	"github.com/Yelethe1st/prepeet/services/platform/platform/grpcdial"
)

// campaignConfig is what the composer needs of recruiting: the artifacts a
// campaign froze at open. Declared here per ADR-0005, so the composer says how
// narrow its need is, and satisfied by recruiting's store in cmd.
type campaignConfig interface {
	CampaignPins(ctx context.Context, tenantID, campaignID string) ([]recruiting.Pin, error)
}

// grpcComposer presents the intelligence plane as the Composer interview
// declared: CTR-02's contract on the wire, ADR-0005's translation in cmd.
//
// It also carries the registry and the campaign config, because resolving what
// to pin is the caller's half of the composition contract: the registry is
// Go's, the campaign's frozen choices are recruiting's, Python reads only what
// arrives pinned, and this adapter is the one place allowed to see all three.
type grpcComposer struct {
	client    intelligencev1.IntelligenceServiceClient
	registry  *content.Store
	campaigns campaignConfig
}

// newComposer dials the intelligence plane.
//
// The transport comes from configuration rather than being fixed here. It used
// to be insecure unconditionally, under a comment claiming the deployed path
// got TLS; nothing on either end provided that, and this hop carries briefs out
// and transcripts back. platform/config refuses undeclared plaintext outside
// local and preview, and the Python server binds a secure port when its own
// certificate is configured.
func newComposer(address string, transport grpcdial.Config, registry *content.Store, campaigns campaignConfig) (*grpcComposer, *grpc.ClientConn, error) {
	option, err := transport.DialOption()
	if err != nil {
		return nil, nil, fmt.Errorf("the intelligence plane transport: %w", err)
	}
	// The trace goes with it. Without this the intelligence plane does the
	// slowest work in the product and none of it can be connected to the
	// request that caused it, which is PLT-08's broken link at its widest.
	conn, err := grpc.NewClient(address, option, grpcdial.TraceOption())
	if err != nil {
		return nil, nil, fmt.Errorf("dialling the intelligence plane: %w", err)
	}
	return &grpcComposer{
		client:    intelligencev1.NewIntelligenceServiceClient(conn),
		registry:  registry,
		campaigns: campaigns,
	}, conn, nil
}

// resolvePins resolves the registry artifacts this composition reads.
//
// The floor pins exactly the plan the blueprint names; the composition
// tickets that follow widen the set. A blueprint the registry cannot resolve
// is refused here, before the wire, with the same taxonomy code Python would
// use: the failure belongs to the request, not to the transport, and
// retrying it spends nothing but repeats nothing either.
func (c *grpcComposer) resolvePins(ctx context.Context, request interview.ComposeRequest) ([]*intelligencev1.PinnedArtifact, error) {
	if request.Mode == "screening" {
		return c.screeningPins(ctx, request)
	}
	if request.BlueprintID == "" {
		// Python's own validation refuses this with INVALID_INPUT; sending it
		// keeps that refusal's home in one place.
		return nil, nil
	}

	plan, err := c.registry.Resolve(ctx, request.BlueprintID, request.TenantID)
	if err != nil {
		if errors.Is(err, content.ErrNotFound) {
			return nil, &interview.ComposeFailure{
				Code:      "FAILURE_CODE_ARTIFACT_NOT_FOUND",
				Retryable: retryableCodes[rpcv1.FailureCode_FAILURE_CODE_ARTIFACT_NOT_FOUND],
				Message:   fmt.Sprintf("the registry resolves no artifact for blueprint %q", request.BlueprintID),
			}
		}
		return nil, fmt.Errorf("resolving the blueprint: %w", err)
	}

	pins := []*intelligencev1.PinnedArtifact{{
		ArtifactType:  plan.Type,
		Reference:     plan.Reference,
		Version:       plan.Version,
		SchemaVersion: plan.SchemaVersion,
		Digest:        plan.Digest,
		Body:          plan.Body,
	}}

	// EVL-02: the rubric is pinned at composition, so evaluation judges by
	// what was in force when the session was made, never by whatever is
	// published later. Practice uses the platform default; screening pins the
	// campaign's own, resolved by digest in screeningPins.
	rubric, err := c.registry.Resolve(ctx, practiceRubricReference, request.TenantID)
	if err != nil {
		if errors.Is(err, content.ErrNotFound) {
			return nil, &interview.ComposeFailure{
				Code:      "FAILURE_CODE_ARTIFACT_NOT_FOUND",
				Retryable: retryableCodes[rpcv1.FailureCode_FAILURE_CODE_ARTIFACT_NOT_FOUND],
				Message:   "the registry resolves no practice rubric; publish content first",
			}
		}
		return nil, fmt.Errorf("resolving the rubric: %w", err)
	}
	pins = append(pins, &intelligencev1.PinnedArtifact{
		ArtifactType:  rubric.Type,
		Reference:     rubric.Reference,
		Version:       rubric.Version,
		SchemaVersion: rubric.SchemaVersion,
		Digest:        rubric.Digest,
		Body:          rubric.Body,
	})

	// EVL-07: the model policy is pinned too, so what a session was
	// allowed to spend per stage is answerable from the session itself
	// rather than from whatever is configured when it is asked.
	policy, err := c.registry.Resolve(ctx, evaluation.PolicyReference, request.TenantID)
	if err != nil {
		if errors.Is(err, content.ErrNotFound) {
			return nil, &interview.ComposeFailure{
				Code:      "FAILURE_CODE_ARTIFACT_NOT_FOUND",
				Retryable: retryableCodes[rpcv1.FailureCode_FAILURE_CODE_ARTIFACT_NOT_FOUND],
				Message:   "the registry resolves no model policy; publish content first",
			}
		}
		return nil, fmt.Errorf("resolving the model policy: %w", err)
	}
	pins = append(pins, &intelligencev1.PinnedArtifact{
		ArtifactType:  policy.Type,
		Reference:     policy.Reference,
		Version:       policy.Version,
		SchemaVersion: policy.SchemaVersion,
		Digest:        policy.Digest,
		Body:          policy.Body,
	})
	return pins, nil
}

// screeningPins pins a screening session to exactly what its campaign froze.
//
// The campaign chose its rubric, calibration, persona and plan at open and
// stored each by digest; this reads them and resolves each digest to the
// immutable body content holds for it, so the session runs and is judged by the
// configuration the campaign committed to and not by whatever is published when
// the interview happens. That is EVL-02 for screening: the practice path pins
// the platform default rubric, and this pins the campaign's, from the same
// need. The blueprint the request carries is deliberately unused here; the
// campaign's pinned plan is the plan, and a screening session's blueprint is a
// record of the request, not a second source of truth.
//
// The model policy is pinned too, current at composition, exactly as practice
// pins it, because a campaign fixes what a session is judged against, not what
// each stage may spend, which is the platform's to set (EVL-07).
func (c *grpcComposer) screeningPins(ctx context.Context, request interview.ComposeRequest) ([]*intelligencev1.PinnedArtifact, error) {
	pins, err := c.campaigns.CampaignPins(ctx, request.TenantID, request.CampaignID)
	if err != nil {
		return nil, fmt.Errorf("reading the campaign's pins: %w", err)
	}
	if len(pins) == 0 {
		// A screening session whose campaign pinned nothing cannot be composed
		// into anything meaningful. It is the request's fault, not the
		// transport's, and retrying it composes the same nothing.
		return nil, &interview.ComposeFailure{
			Code:      "FAILURE_CODE_ARTIFACT_NOT_FOUND",
			Retryable: retryableCodes[rpcv1.FailureCode_FAILURE_CODE_ARTIFACT_NOT_FOUND],
			Message:   "the campaign has pinned no configuration to compose against",
		}
	}

	resolved := make([]*intelligencev1.PinnedArtifact, 0, len(pins)+1)
	for _, pin := range pins {
		artifact, err := c.registry.GetByDigest(ctx, pin.Digest, request.TenantID)
		if err != nil {
			if errors.Is(err, content.ErrNotFound) {
				// The campaign pinned a digest the registry no longer holds.
				// That is a data integrity failure, not a transient one, and
				// retrying re-reads the same missing artifact.
				return nil, &interview.ComposeFailure{
					Code:      "FAILURE_CODE_ARTIFACT_NOT_FOUND",
					Retryable: retryableCodes[rpcv1.FailureCode_FAILURE_CODE_ARTIFACT_NOT_FOUND],
					Message:   fmt.Sprintf("the campaign pinned %s at %s, which the registry no longer resolves", pin.Reference, pin.Digest),
				}
			}
			return nil, fmt.Errorf("resolving pinned %s: %w", pin.Reference, err)
		}
		resolved = append(resolved, &intelligencev1.PinnedArtifact{
			ArtifactType:  artifact.Type,
			Reference:     artifact.Reference,
			Version:       artifact.Version,
			SchemaVersion: artifact.SchemaVersion,
			Digest:        artifact.Digest,
			Body:          artifact.Body,
		})
	}

	policy, err := c.registry.Resolve(ctx, evaluation.PolicyReference, request.TenantID)
	if err != nil {
		if errors.Is(err, content.ErrNotFound) {
			return nil, &interview.ComposeFailure{
				Code:      "FAILURE_CODE_ARTIFACT_NOT_FOUND",
				Retryable: retryableCodes[rpcv1.FailureCode_FAILURE_CODE_ARTIFACT_NOT_FOUND],
				Message:   "the registry resolves no model policy; publish content first",
			}
		}
		return nil, fmt.Errorf("resolving the model policy: %w", err)
	}
	resolved = append(resolved, &intelligencev1.PinnedArtifact{
		ArtifactType:  policy.Type,
		Reference:     policy.Reference,
		Version:       policy.Version,
		SchemaVersion: policy.SchemaVersion,
		Digest:        policy.Digest,
		Body:          policy.Body,
	})
	return resolved, nil
}

// practiceRubricReference is the platform default every practice session
// is judged by. Screening pins the campaign's own instead; see screeningPins.
const practiceRubricReference = "rubric/practice-default"

// Compose asks Python for the session bundle.
func (c *grpcComposer) Compose(ctx context.Context, request interview.ComposeRequest) (interview.ComposeResult, error) {
	purpose := intelligencev1.Purpose_PURPOSE_PRACTICE
	if request.Mode == "screening" {
		purpose = intelligencev1.Purpose_PURPOSE_SCREENING
	}

	pins, err := c.resolvePins(ctx, request)
	if err != nil {
		return interview.ComposeResult{}, err
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
		SessionId:    request.SessionID,
		BlueprintId:  request.BlueprintID,
		PinnedInputs: pins,
	})
	if err != nil {
		return interview.ComposeResult{}, translateFailure(err)
	}

	bundle := response.GetBundle()
	return interview.ComposeResult{
		BundleRef:      bundle.GetStorageKey(),
		BundleDigest:   bundle.GetDigest(),
		BundleRevision: int(response.GetBundleRevision()),
		BundleBody:     response.GetBundleBody(),
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
