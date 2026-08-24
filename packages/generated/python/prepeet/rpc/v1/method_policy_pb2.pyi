from google.protobuf import descriptor_pb2 as _descriptor_pb2
from prepeet.rpc.v1 import failure_pb2 as _failure_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor
METHOD_POLICY_FIELD_NUMBER: _ClassVar[int]
method_policy: _descriptor.FieldDescriptor

class MethodPolicy(_message.Message):
    __slots__ = ("timeout_ms", "idempotent", "failure_codes")
    TIMEOUT_MS_FIELD_NUMBER: _ClassVar[int]
    IDEMPOTENT_FIELD_NUMBER: _ClassVar[int]
    FAILURE_CODES_FIELD_NUMBER: _ClassVar[int]
    timeout_ms: int
    idempotent: bool
    failure_codes: _containers.RepeatedScalarFieldContainer[_failure_pb2.FailureCode]
    def __init__(self, timeout_ms: _Optional[int] = ..., idempotent: _Optional[bool] = ..., failure_codes: _Optional[_Iterable[_Union[_failure_pb2.FailureCode, str]]] = ...) -> None: ...
