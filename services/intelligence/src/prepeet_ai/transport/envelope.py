"""The typed envelope every RPC between Go and this service travels in.

The taxonomy itself is owned by packages/contracts/rpc, where each code
declares its retry decision as a descriptor option; tests/test_rpc_contract.py
fails if this module and the descriptor disagree, so neither can be edited
alone. docs/contracts/internal-rpc.md fixes two rules that this module enforces
rather than documents. Every result carries the versions that produced it, so an
evaluation can be reconstructed a year later against the same inputs. Every
failure states whether retrying could help, because Go owns the retry decision
and cannot make it by inspecting a message string.

Implements part of CTR-02.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum
from typing import Any

__all__ = ["Failure", "FailureCode", "Result"]


@dataclass(frozen=True, slots=True)
class Result:
    """A successful RPC result and the versions that produced it.

    Attributes:
        payload: The capability specific result body. Go validates its shape
            against the published schema before persisting or publishing it.
        schema_version: Version of the payload schema.
        calculation_version: Version of the deterministic calculation, where the
            capability has one. A model cannot invent a measured value, so the
            calculator that produced it is recorded separately from the model.
        policy_version: Version of the policy applied, such as the coaching
            policy or the evaluation rubric policy.

    Raises:
        ValueError: If any version is empty. An unversioned result cannot be
            reproduced, and an irreproducible result is not evidence.
    """

    payload: dict[str, Any]
    schema_version: str
    calculation_version: str
    policy_version: str

    def __post_init__(self) -> None:
        """Reject a result that could not be reproduced."""
        for name in ("schema_version", "calculation_version", "policy_version"):
            if not getattr(self, name):
                raise ValueError(f"{name} is required: a result without it cannot be reproduced")


class FailureCode(Enum):
    """Stable failure codes. Callers branch on these, never on the message.

    Each member carries whether retrying the same call could plausibly succeed.
    Pairing the two here means a new code cannot be added without someone
    deciding its retry behaviour.
    """

    INVALID_INPUT = ("INVALID_INPUT", False)
    UNSUPPORTED_CAPABILITY = ("UNSUPPORTED_CAPABILITY", False)
    # The request pinned a policy or schema version this deployment does not
    # carry. The fix is a rollout or a re-pin, not a retry.
    UNSUPPORTED_POLICY_VERSION = ("UNSUPPORTED_POLICY_VERSION", False)
    ARTIFACT_NOT_FOUND = ("ARTIFACT_NOT_FOUND", False)
    SCHEMA_VALIDATION_FAILED = ("SCHEMA_VALIDATION_FAILED", False)
    # Silence where speech was expected, audio below the quality floor, an
    # unsupported language. Never coerced into a low score: a distinct code is
    # what makes that refusal expressible.
    UNASSESSABLE_INPUT = ("UNASSESSABLE_INPUT", False)
    # The proposal cursor is behind the accepted sequence. The caller
    # refreshes and asks again; retrying replays the same stale state.
    STALE_CURSOR = ("STALE_CURSOR", False)
    # Retrying a budget failure spends more money to fail the same way. The
    # caller degrades instead: it keeps the deterministic result and omits the
    # optional narrative. See docs/architecture/evaluation-system.md.
    BUDGET_EXHAUSTED = ("BUDGET_EXHAUSTED", False)
    PROVIDER_UNAVAILABLE = ("PROVIDER_UNAVAILABLE", True)
    PROVIDER_TIMEOUT = ("PROVIDER_TIMEOUT", True)
    INTERNAL = ("INTERNAL", True)

    def __init__(self, code: str, retryable: bool) -> None:
        """Pair the wire value with its retry decision."""
        self._code = code
        self._retryable = retryable

    @property
    def code(self) -> str:
        """The stable wire value."""
        return self._code

    @property
    def retryable(self) -> bool:
        """Whether retrying the same call could plausibly succeed."""
        return self._retryable


@dataclass(frozen=True, slots=True)
class Failure:
    """A failed RPC result.

    Attributes:
        code: The stable code the caller branches on.
        message: A human readable explanation for an operator. It is never
            machine logic and it may be reworded at any time.
        detail: Optional structured context, such as which field was invalid.
            It must never carry transcript content or any other restricted data,
            because failures are logged. See docs/security/data-classification.md.

    Raises:
        ValueError: If the message is empty. A failure an operator cannot read
            is a failure they cannot act on.
    """

    code: FailureCode
    message: str
    detail: dict[str, Any] = field(default_factory=dict)

    def __post_init__(self) -> None:
        """Reject a failure an operator could not act on."""
        if not self.message:
            raise ValueError("message is required: an empty failure cannot be acted on")

    @property
    def retryable(self) -> bool:
        """Whether the caller should retry. Delegates to the code."""
        return self.code.retryable
