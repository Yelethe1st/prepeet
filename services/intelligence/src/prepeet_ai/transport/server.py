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
import os
from collections.abc import Mapping
from concurrent import futures
from pathlib import Path

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
from prepeet_ai.transport.tracing import tracing_interceptor

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


class TLSConfigError(Exception):
    """Configuration that cannot produce the transport it claims to."""


@dataclasses.dataclass(frozen=True)
class TLSConfig:
    """The server half of this hop's transport security.

    The control plane sends interview briefs over this connection and receives
    transcripts back, so it carries candidate speech. Both ends were plaintext
    with no way to change that, while a comment on the Go dial said the
    deployed path had TLS. This is that path, on the serving side.

    Empty is plaintext, deliberately: `make dev` must not need a certificate
    authority, and refusing to start without one would push every contributor
    towards a shared certificate, which is worse than none. The environments
    where plaintext is not acceptable refuse it in the *client's*
    configuration, where the environment name is known; a server cannot tell
    whether the plaintext it was asked for is a laptop or a mistake.
    """

    cert_file: str = ""
    key_file: str = ""
    # Present means client certificates are required, not merely accepted. A
    # mutual configuration that only requests them authenticates nobody while
    # reading as though it does.
    client_ca_file: str = ""

    @classmethod
    def from_env(cls, env: Mapping[str, str]) -> TLSConfig:
        """Read the configuration without consulting the real environment."""
        return cls(
            cert_file=env.get("PREPEET_RPC_TLS_CERT_FILE", "").strip(),
            key_file=env.get("PREPEET_RPC_TLS_KEY_FILE", "").strip(),
            client_ca_file=env.get("PREPEET_RPC_TLS_CLIENT_CA_FILE", "").strip(),
        )

    @property
    def enabled(self) -> bool:
        """Whether a server certificate was configured at all."""
        return bool(self.cert_file and self.key_file)

    @property
    def mutual(self) -> bool:
        """Whether the client must present a certificate this server trusts."""
        return self.enabled and bool(self.client_ca_file)

    def validate(self) -> None:
        """Refuse a configuration that would serve less than it appears to.

        Half a pair and a lone client authority both fail here rather than at
        bind time, because at bind time the only symptom is a server that came
        up plaintext and said nothing.
        """
        if bool(self.cert_file) != bool(self.key_file):
            raise TLSConfigError(
                "PREPEET_RPC_TLS_CERT_FILE and PREPEET_RPC_TLS_KEY_FILE "
                "must be set together or not at all"
            )
        if self.client_ca_file and not self.enabled:
            raise TLSConfigError(
                "PREPEET_RPC_TLS_CLIENT_CA_FILE needs a server certificate: "
                "client authentication cannot be required over plaintext"
            )

    def read(self) -> grpc.ServerCredentials:
        """Load the material and build the credentials.

        A path that does not resolve names itself. The material never reaches
        an error message: a key file that failed to parse must not have its
        contents quoted into a log line.
        """
        self.validate()
        certificate = _read_file(self.cert_file)
        key = _read_file(self.key_file)
        root = _read_file(self.client_ca_file) if self.client_ca_file else None
        return grpc.ssl_server_credentials(
            [(key, certificate)],
            root_certificates=root,
            require_client_auth=root is not None,
        )


def _read_file(path: str) -> bytes:
    try:
        return Path(path).read_bytes()
    except OSError as error:
        raise TLSConfigError(f"reading {path}: {error.strerror}") from None


def serve(port: int, tls: TLSConfig | None = None) -> tuple[grpc.Server, int]:
    """Start the server; the caller owns its lifetime.

    Returns the bound port as well as the server, because port 0 asks the
    operating system to choose and a caller - a test most of all - needs to
    know what it chose.

    `tls` defaults to plaintext so the local stack and the tests that call this
    directly need no certificate. What is served is logged either way, because
    the failure worth catching is a deployment that meant to be encrypted and
    silently was not.
    """
    # The interceptor joins each call to the trace the Go worker sent, so the
    # work this plane does appears under the request that caused it rather than
    # as an unattached island. PLT-08.
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=8),
        interceptors=[tracing_interceptor()],
    )
    intelligence_pb2_grpc.add_IntelligenceServiceServicer_to_server(IntelligenceService(), server)
    if tls is not None and tls.enabled:
        bound = server.add_secure_port(f"[::]:{port}", tls.read())
        transport_name = "mutual-tls" if tls.mutual else "tls"
    else:
        if tls is not None:
            # A half-configured pair would otherwise land here as plaintext.
            tls.validate()
        bound = server.add_insecure_port(f"[::]:{port}")
        transport_name = "plaintext"
    if bound == 0:
        raise RuntimeError(f"the server could not bind port {port}")
    server.start()
    logger.info(
        "intelligence serving",
        extra={"bound_port": bound, "transport": transport_name},
    )
    return server, bound


def main() -> None:
    """Serve until interrupted."""
    logging.basicConfig(level=logging.INFO)
    parser = argparse.ArgumentParser(description="The Prepeet intelligence plane.")
    parser.add_argument("--port", type=int, default=50051)
    arguments = parser.parse_args()

    server, _ = serve(arguments.port, TLSConfig.from_env(os.environ))
    server.wait_for_termination()


if __name__ == "__main__":
    main()
