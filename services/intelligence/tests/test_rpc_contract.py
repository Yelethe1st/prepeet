"""The declarations CTR-02 requires, checked against the compiled descriptor.

Every RPC must declare its timeout, idempotency and failure codes, and every
failure code its retry decision. The declarations live as options in
packages/contracts/rpc; these tests walk the generated descriptor, so a method
added without its policy fails here rather than surfacing as a call with no
deadline in production.

The envelope in prepeet_ai.transport predates the Protobuf contract. Its
agreement with the descriptor is asserted rather than assumed, because two
failure taxonomies that drift apart give Go and Python different opinions about
whether to retry the same failure.
"""

from __future__ import annotations

from google.protobuf.descriptor import MethodDescriptor
from prepeet.intelligence.v1 import intelligence_pb2
from prepeet.rpc.v1 import annotations_pb2, failure_pb2, method_policy_pb2

from prepeet_ai.transport.envelope import FailureCode

SERVICE = intelligence_pb2.DESCRIPTOR.services_by_name["IntelligenceService"]

FAILURE_ENUM = failure_pb2.FailureCode.DESCRIPTOR


def method_policy(method: MethodDescriptor) -> method_policy_pb2.MethodPolicy:
    """Read the method_policy option from one method's descriptor."""
    return method.GetOptions().Extensions[method_policy_pb2.method_policy]


class TestEveryMethodDeclaresItsPolicy:
    """The second acceptance criterion of CTR-02, made mechanical."""

    def test_the_service_has_the_eight_capabilities(self) -> None:
        """A method disappearing is a contract change, not a refactor."""
        assert len(SERVICE.methods) == 8

    def test_every_method_declares_a_timeout(self) -> None:
        """A call with no deadline holds a workflow slot forever."""
        for method in SERVICE.methods:
            assert method_policy(method).timeout_ms > 0, f"{method.name} declares no timeout"

    def test_every_method_is_idempotent(self) -> None:
        """Retries happen in the workflow layer against exactly this promise."""
        for method in SERVICE.methods:
            assert method_policy(method).idempotent, (
                f"{method.name} is not declared idempotent, so nothing may retry it"
            )

    def test_every_method_declares_its_failure_codes(self) -> None:
        """What can go wrong is part of the contract, not of the history."""
        for method in SERVICE.methods:
            assert method_policy(method).failure_codes, f"{method.name} declares no failure codes"

    def test_the_live_path_has_the_tightest_deadline(self) -> None:
        """A candidate waits in silence while ProposeNextAction runs.

        The exact number is the contract's to choose; that it is the minimum
        across the service is the property that must survive edits.
        """
        timeouts = {m.name: method_policy(m).timeout_ms for m in SERVICE.methods}
        assert timeouts["ProposeNextAction"] == min(timeouts.values())


class TestEveryFailureCodeDeclaresRetryability:
    """A retry flag nobody declared is a retry decision made by accident."""

    def test_every_code_declares_the_option(self) -> None:
        """UNSPECIFIED is exempt: it is never emitted, so never retried."""
        for value in FAILURE_ENUM.values:
            if value.name == "FAILURE_CODE_UNSPECIFIED":
                continue
            assert value.GetOptions().HasExtension(annotations_pb2.retryable), (
                f"{value.name} does not say whether it may be retried"
            )

    def test_provider_failures_retry_and_budget_does_not(self) -> None:
        """The taxonomy the ticket names, with the decisions that matter most.

        Retrying a budget failure spends more money to fail the same way;
        retrying a provider outage is the whole point of the durable workflow.
        """

        def retryable(name: str) -> bool:
            value = FAILURE_ENUM.values_by_name[name]
            return value.GetOptions().Extensions[annotations_pb2.retryable]

        assert retryable("FAILURE_CODE_PROVIDER_UNAVAILABLE")
        assert retryable("FAILURE_CODE_PROVIDER_TIMEOUT")
        assert not retryable("FAILURE_CODE_BUDGET_EXHAUSTED")
        assert not retryable("FAILURE_CODE_INVALID_INPUT")


class TestTheEnvelopeAgreesWithTheDescriptor:
    """One taxonomy, two languages, no drift."""

    def test_the_code_sets_match(self) -> None:
        """A missing code is a failure one side cannot express.

        And on the other side, one it cannot handle.
        """
        proto_codes = {
            value.name.removeprefix("FAILURE_CODE_")
            for value in FAILURE_ENUM.values
            if value.name != "FAILURE_CODE_UNSPECIFIED"
        }
        envelope_codes = {member.code for member in FailureCode}

        assert proto_codes == envelope_codes

    def test_the_retry_decisions_match(self) -> None:
        """Go reads the descriptor and Python reads the envelope.

        A disagreement means the same failure is retried from one side and not
        the other.
        """
        for member in FailureCode:
            value = FAILURE_ENUM.values_by_name[f"FAILURE_CODE_{member.code}"]
            declared = value.GetOptions().Extensions[annotations_pb2.retryable]
            assert declared == member.retryable, (
                f"{member.code}: proto says retryable={declared}, envelope says {member.retryable}"
            )
