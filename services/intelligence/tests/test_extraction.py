"""Extraction, proven at both layers.

The spans are exact, the honesty pass surfaces the rest, and the boundary
refuses what it cannot verify or honestly read.
"""

from __future__ import annotations

import hashlib
import http.server
import json
import threading
from collections.abc import Iterator

import pytest

from prepeet_ai.extraction import service
from prepeet_ai.extraction.extractor import extract
from prepeet_ai.transport.envelope import FailureCode, FailureError

CV = """Amara Osei

Senior Backend Engineer, Northwind Health
Mar 2020 - Present

- Led the migration of 40 services to a shared platform

Skills:
Go, PostgreSQL, distributed systems

I also volunteer at a local coding club teaching teenagers to build things.
"""


class TestExtractor:
    """The pure reading."""

    def test_every_span_reads_back_as_its_fact(self) -> None:
        """The first criterion at its root.

        The span IS the provenance, so slicing the source by it must yield
        the text the fact came from.
        """
        for fact in extract(CV):
            span = CV[fact.span_start : fact.span_end]
            assert span.strip(), f"{fact.kind} points at blank text"
            if fact.kind == "skill":
                assert span == fact.value["name"]
            if fact.kind == "date_range":
                assert fact.value["start"] in span

    def test_the_reading_finds_the_role_above_its_dates(self) -> None:
        """The role heading sits on the line above its date range."""
        roles = [f for f in extract(CV) if f.kind == "role"]
        assert len(roles) == 1
        assert roles[0].value["title"] == "Senior Backend Engineer, Northwind Health"

    def test_unreadable_text_is_surfaced_not_dropped(self) -> None:
        """The second criterion.

        The volunteering line matches no rule, and it must arrive as an
        unparsed fact rather than vanish.
        """
        unparsed = [f for f in extract(CV) if f.kind == "unparsed"]
        assert any("volunteer" in f.value["text"] for f in unparsed)

    def test_extraction_is_deterministic(self) -> None:
        """A retried extraction must converge, not duplicate."""
        assert extract(CV) == extract(CV)

    def test_nothing_at_all_still_answers(self) -> None:
        """An empty document extracts to nothing, without complaint."""
        assert extract("") == []


@pytest.fixture()
def served_cv() -> Iterator[tuple[str, str]]:
    """The CV behind a URL, as the presigned grant serves it."""
    body = CV.encode()

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self) -> None:
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args: object) -> None:
            """Quiet."""

    server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    url = f"http://127.0.0.1:{server.server_port}/cv.txt"
    digest = "sha256:" + hashlib.sha256(body).hexdigest()
    yield url, digest
    server.shutdown()


class TestTheBoundary:
    """Fetch, verify, refuse."""

    def test_fetches_verifies_and_extracts(self, served_cv: tuple[str, str]) -> None:
        """The happy path: fetched, verified, extracted, encoded."""
        url, digest = served_cv
        claims = service.extract_document(url, "text/plain", digest)

        kinds = {claim.kind for claim in claims}
        assert {"role", "date_range", "skill", "unparsed"} <= kinds
        # The claim value is the fact plus its confidence, decodable.
        first = json.loads(claims[0].value)
        assert 0 <= first["confidence"] <= 1
        # And the span travels as start-end.
        start, end = claims[0].source_span.split("-")
        assert int(end) > int(start) >= 0

    def test_a_bare_hex_digest_verifies_too(self, served_cv: tuple[str, str]) -> None:
        """The cross-language agreement, pinned.

        Go records the digest as bare hex at upload completion and sends it
        as ObjectRef declares; the prefixed form composition uses must keep
        working beside it.
        """
        url, digest = served_cv
        bare = digest.removeprefix("sha256:")
        assert service.extract_document(url, "text/plain", bare)

    def test_a_digest_mismatch_is_artifact_not_found(self, served_cv: tuple[str, str]) -> None:
        """Extracting from bytes that do not match the pin is refused.

        Facts from unverified bytes would carry provenance that lies.
        """
        url, _ = served_cv
        with pytest.raises(FailureError) as raised:
            service.extract_document(url, "text/plain", "sha256:" + "0" * 64)
        assert raised.value.failure.code is FailureCode.ARTIFACT_NOT_FOUND

    def test_an_unreadable_format_is_unassessable_not_half_read(self) -> None:
        """The degradation criterion.

        A PDF is refused by name, the caller records unsupported, and the
        profile continues manually.
        """
        with pytest.raises(FailureError) as raised:
            service.extract_document("http://unused.invalid/x", "application/pdf", "sha256:x")
        assert raised.value.failure.code is FailureCode.UNASSESSABLE_INPUT

    def test_a_dead_grant_is_artifact_not_found(self) -> None:
        """A URL that no longer answers means the pinned content is gone."""
        with pytest.raises(FailureError) as raised:
            service.extract_document("http://127.0.0.1:9/never", "text/plain", "sha256:" + "0" * 64)
        assert raised.value.failure.code is FailureCode.ARTIFACT_NOT_FOUND


@pytest.fixture(scope="module")
def stub():  # type: ignore[no-untyped-def]
    """The extraction RPC over a real server, matching the compose harness."""
    import grpc
    from prepeet.intelligence.v1 import intelligence_pb2_grpc

    from prepeet_ai.transport import server as transport

    served, port = transport.serve(port=0)
    channel = grpc.insecure_channel(f"localhost:{port}")
    yield intelligence_pb2_grpc.IntelligenceServiceStub(channel)
    channel.close()
    served.stop(grace=None)


class TestExtractionOverTheWire:
    """The served contract: claims with spans out, typed refusals as details."""

    def test_extracts_claims_with_meta(self, stub, served_cv: tuple[str, str]) -> None:
        """Claims arrive span-linked under the versioned meta."""
        from prepeet.intelligence.v1 import intelligence_pb2

        url, digest = served_cv
        response = stub.ExtractCandidateProfile(
            intelligence_pb2.ExtractCandidateProfileRequest(
                context=intelligence_pb2.RequestContext(schema_version="1.0", request_id="doc_1"),
                document=intelligence_pb2.ObjectRef(
                    storage_key="candidate/u/document/cv.txt",
                    digest=digest,
                    media_type="text/plain",
                    fetch_url=url,
                ),
            )
        )

        assert response.meta.calculation_version == service.EXTRACTOR_VERSION
        assert response.meta.input_digest == digest
        kinds = {claim.kind for claim in response.claims}
        assert "role" in kinds and "unparsed" in kinds
        for claim in response.claims:
            start, end = claim.source_span.split("-")
            assert int(end) > int(start) >= 0
            json.loads(claim.value)

    def test_an_unsupported_format_refuses_with_the_typed_code(self, stub) -> None:
        """The refusal crosses the wire as the taxonomy's own code."""
        import grpc
        from grpc_status import rpc_status
        from prepeet.intelligence.v1 import intelligence_pb2
        from prepeet.rpc.v1 import failure_pb2

        with pytest.raises(grpc.RpcError) as raised:
            stub.ExtractCandidateProfile(
                intelligence_pb2.ExtractCandidateProfileRequest(
                    context=intelligence_pb2.RequestContext(
                        schema_version="1.0", request_id="doc_2"
                    ),
                    document=intelligence_pb2.ObjectRef(
                        storage_key="k",
                        digest="sha256:" + "0" * 64,
                        media_type="application/pdf",
                        fetch_url="http://unused.invalid/x",
                    ),
                )
            )

        rich = rpc_status.from_call(raised.value)
        assert rich is not None
        failure = failure_pb2.Failure()
        assert rich.details[0].Unpack(failure)
        assert failure.code == failure_pb2.FAILURE_CODE_UNASSESSABLE_INPUT
