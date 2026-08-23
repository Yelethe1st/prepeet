# Internal RPC

**Status:** Proposed; Protobuf is authoritative after implementation  
**Owner:** Go/Python platform teams  
**Last updated:** 2026-08-23

## Boundary

Go calls Python through versioned gRPC/Protobuf capability contracts. Services never import each other's source or share domain/persistence models.

```protobuf
service IntelligenceService {
  rpc ExtractCandidateProfile(ExtractCandidateProfileRequest)
      returns (ExtractCandidateProfileResponse);
  rpc ComposeSessionBundle(ComposeSessionBundleRequest)
      returns (ComposeSessionBundleResponse);
  rpc ReduceInterviewEvents(ReduceInterviewEventsRequest)
      returns (ReduceInterviewEventsResponse);
  rpc ProposeNextAction(ProposeNextActionRequest)
      returns (ProposeNextActionResponse);
  rpc EvaluateTurns(EvaluateTurnsRequest)
      returns (EvaluateTurnsResponse);
  rpc EvaluateSession(EvaluateSessionRequest)
      returns (EvaluateSessionResponse);
  rpc AnalyzeArticulation(AnalyzeArticulationRequest)
      returns (AnalyzeArticulationResponse);
  rpc GeneratePracticeCoaching(GeneratePracticeCoachingRequest)
      returns (GeneratePracticeCoachingResponse);
}
```

## Request envelope requirements

- schema version;
- request/idempotency ID;
- tenant and purpose scope;
- immutable input references and digests;
- relevant bundle/policy versions;
- deadline/cancellation;
- trace/correlation/causation;
- capability budget and approved provider constraints.

Large documents/audio use short-lived, purpose-scoped object references rather than embedded bytes.

## Response requirements

- typed capability result;
- result/schema/calculator/prompt/model policy versions;
- input digest confirmation;
- assessability and warnings;
- validation status;
- usage, cost units, and latency;
- retryability/failure category;
- evidence references where material.

## Runtime proposal

The next-action response includes proposal ID, accepted sequence cursor, action enum, obligation, content, reason code, state patch, and policy version. Go must validate freshness, lifecycle, authority, obligation, and schema before application.

## Failure model

Use canonical gRPC status plus typed details distinguishing validation, unsupported policy/version, deadline/provider, budget, unassessable input, stale cursor, and internal failure. Do not return raw provider errors or secrets.

Retries occur at the durable workflow/application layer using idempotency; gRPC middleware must not blindly retry non-idempotent operations.

## Governance

Buf lint/breaking checks, reserved removed fields, additive evolution, unknown-enum safety, generated Go/Python stubs, cross-language fixtures, deadline tests, and conformance tests.

