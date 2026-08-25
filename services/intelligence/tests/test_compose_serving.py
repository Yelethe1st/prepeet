"""Composition over the real wire: the served contract, not the module.

A real server on an ephemeral port and a real channel into it, because the
translations under test - envelope in, status-with-detail out - only exist at
the boundary a stub would remove.
"""

from __future__ import annotations

from collections.abc import Iterator

import grpc
import pytest
from grpc_status import rpc_status
from prepeet.intelligence.v1 import intelligence_pb2, intelligence_pb2_grpc
from prepeet.rpc.v1 import failure_pb2

from prepeet_ai.composition import composer
from prepeet_ai.transport import server as transport


@pytest.fixture(scope="module")
def stub() -> Iterator[intelligence_pb2_grpc.IntelligenceServiceStub]:
    """One served instance for the module, on a port the OS picks."""
    served, port = transport.serve(port=0)

    channel = grpc.insecure_channel(f"localhost:{port}")
    yield intelligence_pb2_grpc.IntelligenceServiceStub(channel)

    channel.close()
    served.stop(grace=None)


def pinned_plan(reference: str = "bp_backend_v1") -> intelligence_pb2.PinnedArtifact:
    """A pinned plan whose digest genuinely matches its body."""
    import hashlib as _hashlib
    import json as _json

    body = _json.dumps(
        {"stages": ["intro", "core"]}, sort_keys=True, separators=(",", ":")
    ).encode()
    return intelligence_pb2.PinnedArtifact(
        artifact_type="plan",
        reference=reference,
        version="1.0.0",
        schema_version="1.0",
        digest="sha256:" + _hashlib.sha256(body).hexdigest(),
        body=body,
    )


def request(
    session_id: str = "ses_1", blueprint: str = "bp_backend_v1"
) -> intelligence_pb2.ComposeSessionBundleRequest:
    """A valid compose request; tests break one thing each."""
    return intelligence_pb2.ComposeSessionBundleRequest(
        context=intelligence_pb2.RequestContext(
            schema_version="1.0",
            request_id=session_id,
            purpose=intelligence_pb2.PURPOSE_PRACTICE,
        ),
        session_id=session_id,
        blueprint_id=blueprint,
        pinned_inputs=[pinned_plan(blueprint)],
    )


class TestComposeOverTheWire:
    """The happy path and the properties the workflow relies on."""

    def test_composes_a_bundle_with_its_meta(
        self, stub: intelligence_pb2_grpc.IntelligenceServiceStub
    ) -> None:
        """The response carries the reproducibility contract, filled in."""
        response = stub.ComposeSessionBundle(request())

        assert response.bundle.storage_key == "sessions/ses_1/bundle"
        assert response.bundle.digest.startswith("sha256:")
        assert response.bundle_revision == 1
        assert response.meta.schema_version == composer.SCHEMA_VERSION
        assert response.meta.calculation_version == composer.CALCULATION_VERSION
        assert response.meta.output_validated

        # The middle box: the bundle document records every pin.
        import json as _json

        bundle = _json.loads(response.bundle_body)
        assert bundle["blueprint_id"] == "bp_backend_v1"
        assert bundle["pinned_inputs"] == [
            {
                "artifact_type": "plan",
                "reference": "bp_backend_v1",
                "version": "1.0.0",
                "schema_version": "1.0",
                "digest": pinned_plan().digest,
            }
        ]

    def test_composition_is_deterministic(
        self, stub: intelligence_pb2_grpc.IntelligenceServiceStub
    ) -> None:
        """The property the restart proof leans on.

        A retried activity re-presents the same request and must converge on
        the same digest, or a worker death forks the session's identity.
        """
        first = stub.ComposeSessionBundle(request())
        second = stub.ComposeSessionBundle(request())

        assert first.bundle.digest == second.bundle.digest

    def test_a_pin_whose_body_lies_is_refused(
        self, stub: intelligence_pb2_grpc.IntelligenceServiceStub
    ) -> None:
        """A lying pin is refused.

        A body not matching its digest would make the bundle assert inputs it
        did not read.
        """
        bad = request()
        bad.pinned_inputs[0].body = b'{"stages":["tampered"]}'

        with pytest.raises(grpc.RpcError) as raised:
            stub.ComposeSessionBundle(bad)

        assert raised.value.code() == grpc.StatusCode.INVALID_ARGUMENT

    def test_a_missing_plan_is_artifact_not_found(
        self, stub: intelligence_pb2_grpc.IntelligenceServiceStub
    ) -> None:
        """The blueprint must be among the pins as a plan."""
        unplanned = request()
        unplanned.pinned_inputs[0].artifact_type = "persona"
        # The persona's digest still matches its body; only the type is wrong.

        with pytest.raises(grpc.RpcError) as raised:
            stub.ComposeSessionBundle(unplanned)

        status = rpc_status.from_call(raised.value)
        failure = failure_pb2.Failure()
        assert status is not None
        found = [d for d in status.details if d.Is(failure_pb2.Failure.DESCRIPTOR)]
        assert found, "no typed Failure detail"
        found[0].Unpack(failure)
        assert failure.code == failure_pb2.FAILURE_CODE_ARTIFACT_NOT_FOUND

    def test_different_inputs_produce_different_digests(
        self, stub: intelligence_pb2_grpc.IntelligenceServiceStub
    ) -> None:
        """Determinism the cheap way would pass the test above.

        One constant passes it; different inputs diverging is what proves the
        digest covers the inputs.
        """
        one = stub.ComposeSessionBundle(request(blueprint="bp_backend_v1"))
        other = stub.ComposeSessionBundle(request(blueprint="bp_frontend_v1"))
        # Each request pins its own plan, so the differing input is the pin
        # itself, which is exactly what the digest must cover.

        assert one.bundle.digest != other.bundle.digest


class TestRefusalsOverTheWire:
    """The failure taxonomy, as a caller receives it."""

    def test_a_missing_blueprint_is_a_typed_refusal(
        self, stub: intelligence_pb2_grpc.IntelligenceServiceStub
    ) -> None:
        """The refusal, as the contract promises it.

        INVALID_ARGUMENT at the transport, and the Failure detail carrying
        the taxonomy code Go's adapter branches on.
        """
        with pytest.raises(grpc.RpcError) as raised:
            stub.ComposeSessionBundle(request(blueprint=""))

        assert raised.value.code() == grpc.StatusCode.INVALID_ARGUMENT

        status = rpc_status.from_call(raised.value)
        assert status is not None
        failures = [
            detail for detail in status.details if detail.Is(failure_pb2.Failure.DESCRIPTOR)
        ]
        assert len(failures) == 1

        failure = failure_pb2.Failure()
        failures[0].Unpack(failure)
        assert failure.code == failure_pb2.FAILURE_CODE_INVALID_INPUT
        assert "blueprint" in failure.message

    def test_an_unknown_purpose_is_refused_not_defaulted(
        self, stub: intelligence_pb2_grpc.IntelligenceServiceStub
    ) -> None:
        """PURPOSE_UNSPECIFIED must never quietly become practice.

        The purpose decides whose authority the session lives under.
        """
        bad = request()
        bad.context.purpose = intelligence_pb2.PURPOSE_UNSPECIFIED

        with pytest.raises(grpc.RpcError) as raised:
            stub.ComposeSessionBundle(bad)

        assert raised.value.code() == grpc.StatusCode.INVALID_ARGUMENT

    def test_an_unbuilt_capability_says_unimplemented(
        self, stub: intelligence_pb2_grpc.IntelligenceServiceStub
    ) -> None:
        """The honest answer for what does not exist yet.

        And distinguishable from every real refusal.
        """
        with pytest.raises(grpc.RpcError) as raised:
            stub.EvaluateSession(intelligence_pb2.EvaluateSessionRequest(session_id="ses_1"))

        assert raised.value.code() == grpc.StatusCode.UNIMPLEMENTED
