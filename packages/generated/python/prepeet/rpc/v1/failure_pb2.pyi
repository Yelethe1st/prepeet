from prepeet.rpc.v1 import annotations_pb2 as _annotations_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class FailureCode(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    FAILURE_CODE_UNSPECIFIED: _ClassVar[FailureCode]
    FAILURE_CODE_INVALID_INPUT: _ClassVar[FailureCode]
    FAILURE_CODE_UNSUPPORTED_CAPABILITY: _ClassVar[FailureCode]
    FAILURE_CODE_UNSUPPORTED_POLICY_VERSION: _ClassVar[FailureCode]
    FAILURE_CODE_ARTIFACT_NOT_FOUND: _ClassVar[FailureCode]
    FAILURE_CODE_SCHEMA_VALIDATION_FAILED: _ClassVar[FailureCode]
    FAILURE_CODE_BUDGET_EXHAUSTED: _ClassVar[FailureCode]
    FAILURE_CODE_UNASSESSABLE_INPUT: _ClassVar[FailureCode]
    FAILURE_CODE_STALE_CURSOR: _ClassVar[FailureCode]
    FAILURE_CODE_PROVIDER_UNAVAILABLE: _ClassVar[FailureCode]
    FAILURE_CODE_PROVIDER_TIMEOUT: _ClassVar[FailureCode]
    FAILURE_CODE_INTERNAL: _ClassVar[FailureCode]
FAILURE_CODE_UNSPECIFIED: FailureCode
FAILURE_CODE_INVALID_INPUT: FailureCode
FAILURE_CODE_UNSUPPORTED_CAPABILITY: FailureCode
FAILURE_CODE_UNSUPPORTED_POLICY_VERSION: FailureCode
FAILURE_CODE_ARTIFACT_NOT_FOUND: FailureCode
FAILURE_CODE_SCHEMA_VALIDATION_FAILED: FailureCode
FAILURE_CODE_BUDGET_EXHAUSTED: FailureCode
FAILURE_CODE_UNASSESSABLE_INPUT: FailureCode
FAILURE_CODE_STALE_CURSOR: FailureCode
FAILURE_CODE_PROVIDER_UNAVAILABLE: FailureCode
FAILURE_CODE_PROVIDER_TIMEOUT: FailureCode
FAILURE_CODE_INTERNAL: FailureCode

class Failure(_message.Message):
    __slots__ = ("code", "message", "detail")
    class DetailEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    CODE_FIELD_NUMBER: _ClassVar[int]
    MESSAGE_FIELD_NUMBER: _ClassVar[int]
    DETAIL_FIELD_NUMBER: _ClassVar[int]
    code: FailureCode
    message: str
    detail: _containers.ScalarMap[str, str]
    def __init__(self, code: _Optional[_Union[FailureCode, str]] = ..., message: _Optional[str] = ..., detail: _Optional[_Mapping[str, str]] = ...) -> None: ...
