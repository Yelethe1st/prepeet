"""Contract tests for the typed RPC envelope Go and Python exchange."""

import dataclasses

import pytest

from prepeet_ai.transport import envelope


def test_result_carries_the_versions_that_produced_it() -> None:
    result = envelope.Result(
        payload={"words_per_minute": 172},
        schema_version="1.0",
        calculation_version="articulation-features-v1",
        policy_version="articulation-practice-v1",
    )

    assert result.schema_version == "1.0"
    assert result.calculation_version == "articulation-features-v1"
    assert result.policy_version == "articulation-practice-v1"


def test_result_is_immutable() -> None:
    """A result is evidence. Nothing downstream may edit it after the fact."""
    result = envelope.Result(
        payload={},
        schema_version="1.0",
        calculation_version="v1",
        policy_version="v1",
    )

    with pytest.raises(dataclasses.FrozenInstanceError):
        result.schema_version = "2.0"  # type: ignore[misc]


@pytest.mark.parametrize(
    "missing",
    ["schema_version", "calculation_version", "policy_version"],
)
def test_result_refuses_a_missing_version(missing: str) -> None:
    """An unversioned result cannot be reproduced, so it is not a valid result."""
    fields = {
        "payload": {},
        "schema_version": "1.0",
        "calculation_version": "v1",
        "policy_version": "v1",
    }
    fields[missing] = ""

    with pytest.raises(ValueError, match=missing):
        envelope.Result(**fields)  # type: ignore[arg-type]


def test_failure_states_whether_retrying_could_help() -> None:
    """Go decides whether to retry. It can only decide if the failure says so."""
    transient = envelope.Failure(
        code=envelope.FailureCode.PROVIDER_UNAVAILABLE,
        message="The model provider did not respond.",
    )
    permanent = envelope.Failure(
        code=envelope.FailureCode.INVALID_INPUT,
        message="The transcript was empty.",
    )

    assert transient.retryable is True
    assert permanent.retryable is False


def test_budget_exhaustion_is_not_retryable() -> None:
    """Retrying a budget failure spends more money to fail the same way.

    docs/architecture/evaluation-system.md requires budget exhaustion to omit
    optional narrative while retaining the deterministic result, rather than
    being retried until the budget is gone.
    """
    failure = envelope.Failure(
        code=envelope.FailureCode.BUDGET_EXHAUSTED,
        message="The evaluation budget for this session is spent.",
    )

    assert failure.retryable is False


def test_failure_message_is_for_humans_not_for_logic() -> None:
    """Callers branch on the code. The message is free to change wording."""
    failure = envelope.Failure(
        code=envelope.FailureCode.INVALID_INPUT,
        message="The transcript was empty.",
    )

    assert failure.code is envelope.FailureCode.INVALID_INPUT
    assert isinstance(failure.message, str)


def test_failure_refuses_an_empty_message() -> None:
    """An operator reading a failure with no message learns nothing from it."""
    with pytest.raises(ValueError, match="message"):
        envelope.Failure(code=envelope.FailureCode.INVALID_INPUT, message="")


def test_every_failure_code_declares_retryability() -> None:
    """A new code added without a retry decision would default to a guess."""
    for code in envelope.FailureCode:
        failure = envelope.Failure(code=code, message="checked")
        assert isinstance(failure.retryable, bool)
