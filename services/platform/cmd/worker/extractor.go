package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetrpc/intelligencev1"
	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetrpc/rpcv1"
	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
	"github.com/Yelethe1st/prepeet/services/platform/platform/objectstore"
)

// grpcExtractor presents the intelligence plane as the Extractor candidate
// declared: same contract, same taxonomy, same descriptor-read retry table as
// the composer beside it.
//
// It also holds the object store, because the capability reads the document
// through a fetch URL rather than a credential: the adapter presigns a
// short-lived GET scoped to exactly the pinned object, which is the narrowest
// grant that lets Python verify the digest itself.
type grpcExtractor struct {
	client intelligencev1.IntelligenceServiceClient
	store  *objectstore.S3Store
}

// fetchTTL is how long the extraction grant lives. One workflow attempt,
// not one working day: the URL exists to carry a single fetch.
const fetchTTL = 5 * time.Minute

// newExtractor wires the extractor over an already-dialled connection, so the
// worker holds one channel to the intelligence plane, not one per capability.
func newExtractor(conn *grpc.ClientConn, store *objectstore.S3Store) *grpcExtractor {
	return &grpcExtractor{client: intelligencev1.NewIntelligenceServiceClient(conn), store: store}
}

// Extract asks Python to read the document into span-linked claims.
func (e *grpcExtractor) Extract(ctx context.Context, request candidate.ExtractRequest) ([]candidate.ExtractedFact, error) {
	name := request.StorageKey
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	key, err := objectstore.NewCandidateKey(request.CandidateID, name)
	if err != nil {
		return nil, err
	}
	fetchURL, err := e.store.PresignPlayback(ctx, key, fetchTTL)
	if err != nil {
		return nil, fmt.Errorf("presigning the extraction fetch: %w", err)
	}

	response, err := e.client.ExtractCandidateProfile(ctx, &intelligencev1.ExtractCandidateProfileRequest{
		Context: &intelligencev1.RequestContext{
			SchemaVersion: "1.0",
			// The document id is the idempotency identity, matching the
			// workflow's own: a retried activity re-presents the same request.
			RequestId: request.DocumentID,
			Purpose:   intelligencev1.Purpose_PURPOSE_PRACTICE,
		},
		Document: &intelligencev1.ObjectRef{
			StorageKey: request.StorageKey,
			// Bare hex, as ObjectRef declares; the row recorded it that way
			// at upload completion and Python verifies what it fetched
			// against exactly this value.
			Digest:    request.SHA256,
			MediaType: request.MediaType,
			FetchUrl:  fetchURL,
		},
	})
	if err != nil {
		return nil, translateExtractFailure(err)
	}

	version := response.GetMeta().GetCalculationVersion()
	facts := make([]candidate.ExtractedFact, 0, len(response.GetClaims()))
	for _, claim := range response.GetClaims() {
		fact, err := factFromClaim(claim, version)
		if err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

// factFromClaim decodes one wire claim into the port's fact.
//
// The confidence travels inside the claim's JSON value; it is lifted out here
// because Go stores it as a column PRO-04's review surface sorts by, and the
// span string decodes into the half-open range the schema requires.
func factFromClaim(claim *intelligencev1.ProfileClaim, version string) (candidate.ExtractedFact, error) {
	var start, end int
	if _, err := fmt.Sscanf(claim.GetSourceSpan(), "%d-%d", &start, &end); err != nil {
		return candidate.ExtractedFact{}, fmt.Errorf("claim %q carries span %q, not start-end: %w",
			claim.GetKind(), claim.GetSourceSpan(), err)
	}

	var value struct {
		Confidence float64 `json:"confidence"`
	}
	if err := json.Unmarshal(claim.GetValue(), &value); err != nil {
		return candidate.ExtractedFact{}, fmt.Errorf("claim %q carries an undecodable value: %w", claim.GetKind(), err)
	}

	return candidate.ExtractedFact{
		Kind:             claim.GetKind(),
		Value:            json.RawMessage(claim.GetValue()),
		SpanStart:        start,
		SpanEnd:          end,
		Confidence:       value.Confidence,
		ExtractorVersion: version,
	}, nil
}

// translateExtractFailure is translateFailure for the extraction port: same
// typed detail, same descriptor-owned retry decision, candidate's error type.
func translateExtractFailure(err error) error {
	failed, ok := status.FromError(err)
	if !ok {
		return err
	}
	for _, detail := range failed.Details() {
		if failure, ok := detail.(*rpcv1.Failure); ok {
			return &candidate.ExtractFailure{
				Code:      failure.GetCode().String(),
				Retryable: retryableCodes[failure.GetCode()],
				Message:   failure.GetMessage(),
			}
		}
	}
	return err
}
