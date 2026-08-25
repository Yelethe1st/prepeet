from prepeet.rpc.v1 import method_policy_pb2 as _method_policy_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Purpose(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    PURPOSE_UNSPECIFIED: _ClassVar[Purpose]
    PURPOSE_PRACTICE: _ClassVar[Purpose]
    PURPOSE_SCREENING: _ClassVar[Purpose]

class ActionKind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    ACTION_KIND_UNSPECIFIED: _ClassVar[ActionKind]
    ACTION_KIND_ASK_QUESTION: _ClassVar[ActionKind]
    ACTION_KIND_FOLLOW_UP: _ClassVar[ActionKind]
    ACTION_KIND_TRANSITION_TOPIC: _ClassVar[ActionKind]
    ACTION_KIND_GIVE_OBLIGED_DISCLOSURE: _ClassVar[ActionKind]
    ACTION_KIND_WRAP_UP: _ClassVar[ActionKind]
    ACTION_KIND_END_SESSION: _ClassVar[ActionKind]
PURPOSE_UNSPECIFIED: Purpose
PURPOSE_PRACTICE: Purpose
PURPOSE_SCREENING: Purpose
ACTION_KIND_UNSPECIFIED: ActionKind
ACTION_KIND_ASK_QUESTION: ActionKind
ACTION_KIND_FOLLOW_UP: ActionKind
ACTION_KIND_TRANSITION_TOPIC: ActionKind
ACTION_KIND_GIVE_OBLIGED_DISCLOSURE: ActionKind
ACTION_KIND_WRAP_UP: ActionKind
ACTION_KIND_END_SESSION: ActionKind

class RequestContext(_message.Message):
    __slots__ = ("schema_version", "request_id", "tenant_id", "purpose", "correlation_id", "causation_id", "budget_cost_units", "approved_providers")
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    REQUEST_ID_FIELD_NUMBER: _ClassVar[int]
    TENANT_ID_FIELD_NUMBER: _ClassVar[int]
    PURPOSE_FIELD_NUMBER: _ClassVar[int]
    CORRELATION_ID_FIELD_NUMBER: _ClassVar[int]
    CAUSATION_ID_FIELD_NUMBER: _ClassVar[int]
    BUDGET_COST_UNITS_FIELD_NUMBER: _ClassVar[int]
    APPROVED_PROVIDERS_FIELD_NUMBER: _ClassVar[int]
    schema_version: str
    request_id: str
    tenant_id: str
    purpose: Purpose
    correlation_id: str
    causation_id: str
    budget_cost_units: int
    approved_providers: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, schema_version: _Optional[str] = ..., request_id: _Optional[str] = ..., tenant_id: _Optional[str] = ..., purpose: _Optional[_Union[Purpose, str]] = ..., correlation_id: _Optional[str] = ..., causation_id: _Optional[str] = ..., budget_cost_units: _Optional[int] = ..., approved_providers: _Optional[_Iterable[str]] = ...) -> None: ...

class ObjectRef(_message.Message):
    __slots__ = ("storage_key", "digest", "media_type")
    STORAGE_KEY_FIELD_NUMBER: _ClassVar[int]
    DIGEST_FIELD_NUMBER: _ClassVar[int]
    MEDIA_TYPE_FIELD_NUMBER: _ClassVar[int]
    storage_key: str
    digest: str
    media_type: str
    def __init__(self, storage_key: _Optional[str] = ..., digest: _Optional[str] = ..., media_type: _Optional[str] = ...) -> None: ...

class Usage(_message.Message):
    __slots__ = ("cost_units", "provider_calls", "latency_ms")
    COST_UNITS_FIELD_NUMBER: _ClassVar[int]
    PROVIDER_CALLS_FIELD_NUMBER: _ClassVar[int]
    LATENCY_MS_FIELD_NUMBER: _ClassVar[int]
    cost_units: int
    provider_calls: int
    latency_ms: int
    def __init__(self, cost_units: _Optional[int] = ..., provider_calls: _Optional[int] = ..., latency_ms: _Optional[int] = ...) -> None: ...

class ResponseMeta(_message.Message):
    __slots__ = ("schema_version", "calculation_version", "policy_version", "prompt_version", "model_version", "input_digest", "output_validated", "warnings", "usage")
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    CALCULATION_VERSION_FIELD_NUMBER: _ClassVar[int]
    POLICY_VERSION_FIELD_NUMBER: _ClassVar[int]
    PROMPT_VERSION_FIELD_NUMBER: _ClassVar[int]
    MODEL_VERSION_FIELD_NUMBER: _ClassVar[int]
    INPUT_DIGEST_FIELD_NUMBER: _ClassVar[int]
    OUTPUT_VALIDATED_FIELD_NUMBER: _ClassVar[int]
    WARNINGS_FIELD_NUMBER: _ClassVar[int]
    USAGE_FIELD_NUMBER: _ClassVar[int]
    schema_version: str
    calculation_version: str
    policy_version: str
    prompt_version: str
    model_version: str
    input_digest: str
    output_validated: bool
    warnings: _containers.RepeatedScalarFieldContainer[str]
    usage: Usage
    def __init__(self, schema_version: _Optional[str] = ..., calculation_version: _Optional[str] = ..., policy_version: _Optional[str] = ..., prompt_version: _Optional[str] = ..., model_version: _Optional[str] = ..., input_digest: _Optional[str] = ..., output_validated: _Optional[bool] = ..., warnings: _Optional[_Iterable[str]] = ..., usage: _Optional[_Union[Usage, _Mapping]] = ...) -> None: ...

class ExtractCandidateProfileRequest(_message.Message):
    __slots__ = ("context", "document", "policy_version")
    CONTEXT_FIELD_NUMBER: _ClassVar[int]
    DOCUMENT_FIELD_NUMBER: _ClassVar[int]
    POLICY_VERSION_FIELD_NUMBER: _ClassVar[int]
    context: RequestContext
    document: ObjectRef
    policy_version: str
    def __init__(self, context: _Optional[_Union[RequestContext, _Mapping]] = ..., document: _Optional[_Union[ObjectRef, _Mapping]] = ..., policy_version: _Optional[str] = ...) -> None: ...

class ExtractCandidateProfileResponse(_message.Message):
    __slots__ = ("meta", "claims")
    META_FIELD_NUMBER: _ClassVar[int]
    CLAIMS_FIELD_NUMBER: _ClassVar[int]
    meta: ResponseMeta
    claims: _containers.RepeatedCompositeFieldContainer[ProfileClaim]
    def __init__(self, meta: _Optional[_Union[ResponseMeta, _Mapping]] = ..., claims: _Optional[_Iterable[_Union[ProfileClaim, _Mapping]]] = ...) -> None: ...

class ProfileClaim(_message.Message):
    __slots__ = ("kind", "value", "source_span")
    KIND_FIELD_NUMBER: _ClassVar[int]
    VALUE_FIELD_NUMBER: _ClassVar[int]
    SOURCE_SPAN_FIELD_NUMBER: _ClassVar[int]
    kind: str
    value: bytes
    source_span: str
    def __init__(self, kind: _Optional[str] = ..., value: _Optional[bytes] = ..., source_span: _Optional[str] = ...) -> None: ...

class ComposeSessionBundleRequest(_message.Message):
    __slots__ = ("context", "session_id", "inputs", "blueprint_id", "pinned_inputs")
    CONTEXT_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    INPUTS_FIELD_NUMBER: _ClassVar[int]
    BLUEPRINT_ID_FIELD_NUMBER: _ClassVar[int]
    PINNED_INPUTS_FIELD_NUMBER: _ClassVar[int]
    context: RequestContext
    session_id: str
    inputs: _containers.RepeatedCompositeFieldContainer[ObjectRef]
    blueprint_id: str
    pinned_inputs: _containers.RepeatedCompositeFieldContainer[PinnedArtifact]
    def __init__(self, context: _Optional[_Union[RequestContext, _Mapping]] = ..., session_id: _Optional[str] = ..., inputs: _Optional[_Iterable[_Union[ObjectRef, _Mapping]]] = ..., blueprint_id: _Optional[str] = ..., pinned_inputs: _Optional[_Iterable[_Union[PinnedArtifact, _Mapping]]] = ...) -> None: ...

class PinnedArtifact(_message.Message):
    __slots__ = ("artifact_type", "reference", "version", "schema_version", "digest", "body")
    ARTIFACT_TYPE_FIELD_NUMBER: _ClassVar[int]
    REFERENCE_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    SCHEMA_VERSION_FIELD_NUMBER: _ClassVar[int]
    DIGEST_FIELD_NUMBER: _ClassVar[int]
    BODY_FIELD_NUMBER: _ClassVar[int]
    artifact_type: str
    reference: str
    version: str
    schema_version: str
    digest: str
    body: bytes
    def __init__(self, artifact_type: _Optional[str] = ..., reference: _Optional[str] = ..., version: _Optional[str] = ..., schema_version: _Optional[str] = ..., digest: _Optional[str] = ..., body: _Optional[bytes] = ...) -> None: ...

class ComposeSessionBundleResponse(_message.Message):
    __slots__ = ("meta", "bundle", "bundle_revision", "bundle_body")
    META_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_REVISION_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_BODY_FIELD_NUMBER: _ClassVar[int]
    meta: ResponseMeta
    bundle: ObjectRef
    bundle_revision: int
    bundle_body: bytes
    def __init__(self, meta: _Optional[_Union[ResponseMeta, _Mapping]] = ..., bundle: _Optional[_Union[ObjectRef, _Mapping]] = ..., bundle_revision: _Optional[int] = ..., bundle_body: _Optional[bytes] = ...) -> None: ...

class ReduceInterviewEventsRequest(_message.Message):
    __slots__ = ("context", "session_id", "bundle_digest", "events", "cursor")
    CONTEXT_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    EVENTS_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    context: RequestContext
    session_id: str
    bundle_digest: str
    events: _containers.RepeatedScalarFieldContainer[bytes]
    cursor: int
    def __init__(self, context: _Optional[_Union[RequestContext, _Mapping]] = ..., session_id: _Optional[str] = ..., bundle_digest: _Optional[str] = ..., events: _Optional[_Iterable[bytes]] = ..., cursor: _Optional[int] = ...) -> None: ...

class ReduceInterviewEventsResponse(_message.Message):
    __slots__ = ("meta", "cursor")
    META_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    meta: ResponseMeta
    cursor: int
    def __init__(self, meta: _Optional[_Union[ResponseMeta, _Mapping]] = ..., cursor: _Optional[int] = ...) -> None: ...

class ProposeNextActionRequest(_message.Message):
    __slots__ = ("context", "session_id", "bundle_digest", "cursor")
    CONTEXT_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    context: RequestContext
    session_id: str
    bundle_digest: str
    cursor: int
    def __init__(self, context: _Optional[_Union[RequestContext, _Mapping]] = ..., session_id: _Optional[str] = ..., bundle_digest: _Optional[str] = ..., cursor: _Optional[int] = ...) -> None: ...

class ProposeNextActionResponse(_message.Message):
    __slots__ = ("meta", "proposal")
    META_FIELD_NUMBER: _ClassVar[int]
    PROPOSAL_FIELD_NUMBER: _ClassVar[int]
    meta: ResponseMeta
    proposal: ActionProposal
    def __init__(self, meta: _Optional[_Union[ResponseMeta, _Mapping]] = ..., proposal: _Optional[_Union[ActionProposal, _Mapping]] = ...) -> None: ...

class ActionProposal(_message.Message):
    __slots__ = ("proposal_id", "cursor", "action", "obligation", "content", "reason_code", "state_patch")
    PROPOSAL_ID_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    ACTION_FIELD_NUMBER: _ClassVar[int]
    OBLIGATION_FIELD_NUMBER: _ClassVar[int]
    CONTENT_FIELD_NUMBER: _ClassVar[int]
    REASON_CODE_FIELD_NUMBER: _ClassVar[int]
    STATE_PATCH_FIELD_NUMBER: _ClassVar[int]
    proposal_id: str
    cursor: int
    action: ActionKind
    obligation: str
    content: str
    reason_code: str
    state_patch: bytes
    def __init__(self, proposal_id: _Optional[str] = ..., cursor: _Optional[int] = ..., action: _Optional[_Union[ActionKind, str]] = ..., obligation: _Optional[str] = ..., content: _Optional[str] = ..., reason_code: _Optional[str] = ..., state_patch: _Optional[bytes] = ...) -> None: ...

class EvaluateTurnsRequest(_message.Message):
    __slots__ = ("context", "session_id", "bundle_digest", "turns")
    CONTEXT_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    TURNS_FIELD_NUMBER: _ClassVar[int]
    context: RequestContext
    session_id: str
    bundle_digest: str
    turns: _containers.RepeatedCompositeFieldContainer[ObjectRef]
    def __init__(self, context: _Optional[_Union[RequestContext, _Mapping]] = ..., session_id: _Optional[str] = ..., bundle_digest: _Optional[str] = ..., turns: _Optional[_Iterable[_Union[ObjectRef, _Mapping]]] = ...) -> None: ...

class EvaluateTurnsResponse(_message.Message):
    __slots__ = ("meta", "observations")
    META_FIELD_NUMBER: _ClassVar[int]
    OBSERVATIONS_FIELD_NUMBER: _ClassVar[int]
    meta: ResponseMeta
    observations: _containers.RepeatedCompositeFieldContainer[CompetencyObservation]
    def __init__(self, meta: _Optional[_Union[ResponseMeta, _Mapping]] = ..., observations: _Optional[_Iterable[_Union[CompetencyObservation, _Mapping]]] = ...) -> None: ...

class CompetencyObservation(_message.Message):
    __slots__ = ("competency_id", "turn_id", "observation")
    COMPETENCY_ID_FIELD_NUMBER: _ClassVar[int]
    TURN_ID_FIELD_NUMBER: _ClassVar[int]
    OBSERVATION_FIELD_NUMBER: _ClassVar[int]
    competency_id: str
    turn_id: str
    observation: bytes
    def __init__(self, competency_id: _Optional[str] = ..., turn_id: _Optional[str] = ..., observation: _Optional[bytes] = ...) -> None: ...

class EvaluateSessionRequest(_message.Message):
    __slots__ = ("context", "session_id", "bundle_digest", "manifest", "rubric_id", "rubric_version")
    CONTEXT_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    MANIFEST_FIELD_NUMBER: _ClassVar[int]
    RUBRIC_ID_FIELD_NUMBER: _ClassVar[int]
    RUBRIC_VERSION_FIELD_NUMBER: _ClassVar[int]
    context: RequestContext
    session_id: str
    bundle_digest: str
    manifest: ObjectRef
    rubric_id: str
    rubric_version: str
    def __init__(self, context: _Optional[_Union[RequestContext, _Mapping]] = ..., session_id: _Optional[str] = ..., bundle_digest: _Optional[str] = ..., manifest: _Optional[_Union[ObjectRef, _Mapping]] = ..., rubric_id: _Optional[str] = ..., rubric_version: _Optional[str] = ...) -> None: ...

class EvaluateSessionResponse(_message.Message):
    __slots__ = ("meta", "result", "uncovered_competency_ids")
    META_FIELD_NUMBER: _ClassVar[int]
    RESULT_FIELD_NUMBER: _ClassVar[int]
    UNCOVERED_COMPETENCY_IDS_FIELD_NUMBER: _ClassVar[int]
    meta: ResponseMeta
    result: bytes
    uncovered_competency_ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, meta: _Optional[_Union[ResponseMeta, _Mapping]] = ..., result: _Optional[bytes] = ..., uncovered_competency_ids: _Optional[_Iterable[str]] = ...) -> None: ...

class AnalyzeArticulationRequest(_message.Message):
    __slots__ = ("context", "session_id", "bundle_digest", "manifest")
    CONTEXT_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    BUNDLE_DIGEST_FIELD_NUMBER: _ClassVar[int]
    MANIFEST_FIELD_NUMBER: _ClassVar[int]
    context: RequestContext
    session_id: str
    bundle_digest: str
    manifest: ObjectRef
    def __init__(self, context: _Optional[_Union[RequestContext, _Mapping]] = ..., session_id: _Optional[str] = ..., bundle_digest: _Optional[str] = ..., manifest: _Optional[_Union[ObjectRef, _Mapping]] = ...) -> None: ...

class AnalyzeArticulationResponse(_message.Message):
    __slots__ = ("meta", "analysis")
    META_FIELD_NUMBER: _ClassVar[int]
    ANALYSIS_FIELD_NUMBER: _ClassVar[int]
    meta: ResponseMeta
    analysis: bytes
    def __init__(self, meta: _Optional[_Union[ResponseMeta, _Mapping]] = ..., analysis: _Optional[bytes] = ...) -> None: ...

class GeneratePracticeCoachingRequest(_message.Message):
    __slots__ = ("context", "session_id", "evaluation", "policy_version")
    CONTEXT_FIELD_NUMBER: _ClassVar[int]
    SESSION_ID_FIELD_NUMBER: _ClassVar[int]
    EVALUATION_FIELD_NUMBER: _ClassVar[int]
    POLICY_VERSION_FIELD_NUMBER: _ClassVar[int]
    context: RequestContext
    session_id: str
    evaluation: ObjectRef
    policy_version: str
    def __init__(self, context: _Optional[_Union[RequestContext, _Mapping]] = ..., session_id: _Optional[str] = ..., evaluation: _Optional[_Union[ObjectRef, _Mapping]] = ..., policy_version: _Optional[str] = ...) -> None: ...

class GeneratePracticeCoachingResponse(_message.Message):
    __slots__ = ("meta", "coaching")
    META_FIELD_NUMBER: _ClassVar[int]
    COACHING_FIELD_NUMBER: _ClassVar[int]
    meta: ResponseMeta
    coaching: bytes
    def __init__(self, meta: _Optional[_Union[ResponseMeta, _Mapping]] = ..., coaching: _Optional[bytes] = ...) -> None: ...
