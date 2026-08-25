package main

import (
	"errors"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/status"
	statuspb "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetrpc/intelligencev1"
	"github.com/Yelethe1st/prepeet/packages/generated/go/prepeetrpc/rpcv1"
	"github.com/Yelethe1st/prepeet/services/platform/internal/candidate"
)

// The adapter's translations, at the seams a wire test cannot isolate: the
// claim decoding that turns spans and confidences into columns, and the
// refusal translation that carries the contract's own retry decision.

func TestAClaimDecodesIntoASpanLinkedFact(t *testing.T) {
	fact, err := factFromClaim(&intelligencev1.ProfileClaim{
		Kind:       "skill",
		Value:      []byte(`{"name":"Go","confidence":0.8}`),
		SourceSpan: "120-122",
	}, "extract-1")
	if err != nil {
		t.Fatalf("decoding: %v", err)
	}

	if fact.SpanStart != 120 || fact.SpanEnd != 122 {
		t.Fatalf("span = %d-%d, want 120-122", fact.SpanStart, fact.SpanEnd)
	}
	if fact.Confidence != 0.8 {
		t.Fatalf("confidence = %v; the value's confidence must be lifted into the column", fact.Confidence)
	}
	if fact.ExtractorVersion != "extract-1" {
		t.Fatalf("extractor version = %q", fact.ExtractorVersion)
	}
}

func TestAClaimWithoutAParseableSpanIsRefused(t *testing.T) {
	// The span is the provenance; a claim that cannot state one is not
	// stored, which is the same rule the schema's NOT NULL enforces.
	_, err := factFromClaim(&intelligencev1.ProfileClaim{
		Kind: "skill", Value: []byte(`{}`), SourceSpan: "somewhere",
	}, "extract-1")
	if err == nil {
		t.Fatal("a spanless claim must be refused")
	}
}

func TestTheRefusalTranslationReadsTheContractsRetryDecision(t *testing.T) {
	detail, err := anypb.New(&rpcv1.Failure{
		Code:    rpcv1.FailureCode_FAILURE_CODE_UNASSESSABLE_INPUT,
		Message: "extract-1 cannot read application/pdf",
	})
	if err != nil {
		t.Fatalf("building detail: %v", err)
	}
	wire := statuspb.FromProto(&status.Status{
		Code: 3, Message: "unassessable", Details: []*anypb.Any{detail},
	})

	translated := translateExtractFailure(wire.Err())

	var failure *candidate.ExtractFailure
	if !errors.As(translated, &failure) {
		t.Fatalf("translated = %T, want *candidate.ExtractFailure", translated)
	}
	if failure.Code != "FAILURE_CODE_UNASSESSABLE_INPUT" {
		t.Fatalf("code = %q", failure.Code)
	}
	if failure.Retryable {
		t.Fatal("UNASSESSABLE_INPUT is declared non-retryable in the contract descriptor")
	}
}
