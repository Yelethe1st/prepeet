"""The gRPC server: CTR-02's contract, served.

The transport layer owns exactly two translations and nothing else: a request
envelope into capability arguments, and a capability's answer - result or
FailureError - into the response or the gRPC status with the typed Failure
detail. Capability code never imports grpc, and this module never makes a
capability decision; a rule appearing here would be a rule the tests over the
capabilities cannot see.

Run locally with `uv run python -m prepeet_ai.transport.server`; the worker's
PREPEET_INTELLIGENCE_ADDRESS points at it.

Implements the serving half of CTR-02 and the floor of CAT-02.
"""

from __future__ import annotations

import argparse
import dataclasses
import json
import logging
from concurrent import futures

import grpc
from google.rpc import code_pb2, status_pb2
from grpc_status import rpc_status
from prepeet.intelligence.v1 import intelligence_pb2, intelligence_pb2_grpc
from prepeet.rpc.v1 import failure_pb2

from prepeet_ai.articulation import service as articulation
from prepeet_ai.composition import composer
from prepeet_ai.evaluation import service as evidence
from prepeet_ai.extraction import service as extraction
from prepeet_ai.transport.envelope import Failure, FailureCode, FailureError

logger = logging.getLogger(__name__)

_PURPOSES = {
    intelligence_pb2.PURPOSE_PRACTICE: "practice",
    intelligence_pb2.PURPOSE_SCREENING: "screening",
}


# The envelope taxonomy's wire values, keyed by the internal codes. The
# conformance tests in tests/test_rpc_contract.py hold the two enums equal, so
# a lookup by name is safe and a drift is a test failure rather than a KeyError
# in production.
def _wire_code(code_name: str) -> int:
    return int(failure_pb2.FailureCode.Value(f"FAILURE_CODE_{code_name}"))


def _abort_with(context: grpc.ServicerContext, error: FailureError) -> None:
    """Turn a capability refusal into the status the contract promises.

    Canonical status plus the typed Failure detail, per
    docs/contracts/internal-rpc.md: the caller branches on the detail's code,
    and the retry decision lives on that code in the descriptor, so nothing
    here says anything about retrying.
    """
    detail = failure_pb2.Failure(
        code=_wire_code(error.failure.code.code),
        message=error.failure.message,
        detail={key: str(value) for key, value in error.failure.detail.items()},
    )
    status = status_pb2.Status(
        code=code_pb2.INVALID_ARGUMENT,
        message=error.failure.message,
    )
    status.details.add().Pack(detail)
    context.abort_with_status(rpc_status.to_status(status))


class IntelligenceService(intelligence_pb2_grpc.IntelligenceServiceServicer):  # type: ignore[misc]
    """The service.

    Capabilities not yet built answer UNIMPLEMENTED, which is the honest
    gRPC vocabulary for exactly that.
    """

    def EvaluateTurns(  # noqa: N802 - the generated stub's casing
        self,
        request: intelligence_pb2.EvaluateTurnsRequest,
        context: grpc.ServicerContext,
    ) -> intelligence_pb2.EvaluateTurnsResponse:
        """Extract competency-linked evidence spans from sealed turns."""
        if not request.turns:
            _abort_with(
                context,
                FailureError(
                    Failure(
                        code=FailureCode.INVALID_INPUT,
                        message="there are no turns to evaluate",
                    )
                ),
            )
            raise AssertionError("abort returns") from None

        try:
            spans = []
            contradictions = []
            for ref in request.turns:
                ref_spans, ref_pairs = evidence.evidence_from_ref(ref.fetch_url, ref.digest)
                spans.extend(ref_spans)
                contradictions.extend(ref_pairs)
        except FailureError as error:
            logger.info(
                "evidence extraction refused",
                extra={"code": error.failure.code.code},
            )
            _abort_with(context, error)
            raise AssertionError("abort returns") from None

        observations = []
        for span in spans:
            observation = json.dumps(
                {
                    "kind": span.kind,
                    "quote": span.quote,
                    "segment_sequence": span.segment_sequence,
                    "char_start": span.char_start,
                    "char_end": span.char_end,
                    "start_ms": span.start_ms,
                    "end_ms": span.end_ms,
                    "extraction_version": span.extraction_version,
                },
                sort_keys=True,
                separators=(",", ":"),
            ).encode()
            observations.append(
                intelligence_pb2.CompetencyObservation(
                    competency_id=span.competency_id,
                    turn_id=str(span.segment_sequence),
                    observation=observation,
                )
            )

        for pair in contradictions:
            # A contradiction binds two statements, not a competency: the
            # observation rides with an empty competency id and the kind
            # as its discriminator. The framing is descriptive throughout;
            # nothing in this stream judges the person.
            observation = json.dumps(
                {
                    "kind": "contradiction",
                    "topic": list(pair.topic),
                    "side_a": dataclasses.asdict(pair.side_a),
                    "side_b": dataclasses.asdict(pair.side_b),
                    "extraction_version": pair.extraction_version,
                },
                sort_keys=True,
                separators=(",", ":"),
            ).encode()
            observations.append(
                intelligence_pb2.CompetencyObservation(
                    competency_id="",
                    turn_id=str(pair.side_a.segment_sequence),
                    observation=observation,
                )
            )

        return intelligence_pb2.EvaluateTurnsResponse(
            meta=intelligence_pb2.ResponseMeta(
                schema_version=evidence.SCHEMA_VERSION,
                calculation_version=evidence.EXTRACTION_VERSION,
                policy_version="none",
                input_digest=request.turns[0].digest,
                output_validated=True,
                usage=intelligence_pb2.Usage(cost_units=0, provider_calls=0),
            ),
            observations=observations,
        )

    def AnalyzeArticulation(  # noqa: N802 - the generated stub's casing
        self,
        request: intelligence_pb2.AnalyzeArticulationRequest,
        context: grpc.ServicerContext,
    ) -> intelligence_pb2.AnalyzeArticulationResponse:
        """Measure delivery deterministically from the sealed transcript (ART-01)."""
        try:
            analysis = articulation.analysis_from_ref(
                request.manifest.fetch_url, request.manifest.digest
            )
        except FailureError as error:
            logger.info("articulation refused", extra={"code": error.failure.code.code})
            _abort_with(context, error)
            raise AssertionError("abort returns") from None

        return intelligence_pb2.AnalyzeArticulationResponse(
            meta=intelligence_pb2.ResponseMeta(
                schema_version=articulation.SCHEMA_VERSION,
                calculation_version=articulation.CALCULATION_VERSION,
                policy_version=articulation.POLICY_VERSION,
                input_digest=request.manifest.digest,
                output_validated=True,
                usage=intelligence_pb2.Usage(cost_units=0, provider_calls=0),
            ),
            analysis=analysis,
        )

    def ExtractCandidateProfile(  # noqa: N802 - the generated stub's casing
        self,
        request: intelligence_pb2.ExtractCandidateProfileRequest,
        context: grpc.ServicerContext,
    ) -> intelligence_pb2.ExtractCandidateProfileResponse:
        """Read a candidate document into span-linked structured facts."""
        try:
            claims = extraction.extract_document(
                fetch_url=request.document.fetch_url,
                media_type=request.document.media_type,
                digest=request.document.digest,
            )
        except FailureError as error:
            logger.info(
                "extraction refused",
                extra={"code": error.failure.code.code},
            )
            _abort_with(context, error)
            raise AssertionError("abort returns") from None

        return intelligence_pb2.ExtractCandidateProfileResponse(
            meta=intelligence_pb2.ResponseMeta(
                schema_version=extraction.SCHEMA_VERSION,
                calculation_version=extraction.EXTRACTOR_VERSION,
                policy_version="none",
                input_digest=request.document.digest,
                output_validated=True,
                usage=intelligence_pb2.Usage(cost_units=0, provider_calls=0),
            ),
            claims=[
                intelligence_pb2.ProfileClaim(
                    kind=claim.kind,
                    value=claim.value,
                    source_span=claim.source_span,
                )
                for claim in claims
            ],
        )

    def ComposeSessionBundle(  # noqa: N802 - the generated stub's casing
        self,
        request: intelligence_pb2.ComposeSessionBundleRequest,
        context: grpc.ServicerContext,
    ) -> intelligence_pb2.ComposeSessionBundleResponse:
        """Compose the immutable bundle for one session."""
        purpose = _PURPOSES.get(request.context.purpose, "")

        pinned = [
            composer.Pinned(
                artifact_type=pin.artifact_type,
                reference=pin.reference,
                version=pin.version,
                schema_version=pin.schema_version,
                digest=pin.digest,
                body=bytes(pin.body),
            )
            for pin in request.pinned_inputs
        ]

        try:
            bundle = composer.compose(
                session_id=request.session_id,
                blueprint_id=request.blueprint_id,
                purpose=purpose,
                pinned=pinned,
            )
        except FailureError as error:
            logger.info(
                "composition refused",
                extra={"code": error.failure.code.code, "session_id": request.session_id},
            )
            _abort_with(context, error)
            raise AssertionError("abort returns") from None

        return intelligence_pb2.ComposeSessionBundleResponse(
            meta=intelligence_pb2.ResponseMeta(
                schema_version=composer.SCHEMA_VERSION,
                calculation_version=composer.CALCULATION_VERSION,
                policy_version="none",
                output_validated=True,
                usage=intelligence_pb2.Usage(cost_units=0, provider_calls=0),
            ),
            bundle=intelligence_pb2.ObjectRef(
                storage_key=bundle.storage_key,
                digest=bundle.digest,
                media_type="application/json",
            ),
            bundle_revision=bundle.revision,
            bundle_body=bundle.body,
        )


def serve(port: int) -> tuple[grpc.Server, int]:
    """Start the server; the caller owns its lifetime.

    Returns the bound port as well as the server, because port 0 asks the
    operating system to choose and a caller - a test most of all - needs to
    know what it chose.
    """
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=8))
    intelligence_pb2_grpc.add_IntelligenceServiceServicer_to_server(IntelligenceService(), server)
    bound = server.add_insecure_port(f"[::]:{port}")
    server.start()
    logger.info("intelligence serving", extra={"bound_port": bound})
    return server, bound


def main() -> None:
    """Serve until interrupted."""
    logging.basicConfig(level=logging.INFO)
    parser = argparse.ArgumentParser(description="The Prepeet intelligence plane.")
    parser.add_argument("--port", type=int, default=50051)
    arguments = parser.parse_args()

    server, _ = serve(arguments.port)
    server.wait_for_termination()


if __name__ == "__main__":
    main()
