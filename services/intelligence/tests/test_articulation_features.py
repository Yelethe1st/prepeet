"""articulation-features-v1: counted, never judged, reproducible forever.

Known fixtures produce known values within a documented tolerance; every
value carries its calculator version and its turn; no model is involved
and none could be; thin or unreliable speech is not assessable rather
than low.
"""

from __future__ import annotations

import dataclasses
import json
import pathlib

from prepeet_ai.articulation.features import (
    CALCULATION_VERSION,
    session_features,
    turn_features,
)

FIXTURE = json.loads(
    (pathlib.Path(__file__).parent / "fixtures/articulation_known.json").read_text()
)


def _turn(sequence: int) -> dict[str, object]:
    return next(t for t in FIXTURE["turns"] if t["sequence"] == sequence)


class TestKnownFixturesProduceKnownValues:
    """The third box: hand-computed expectations, held within tolerance."""

    def test_the_assessable_turn_measures_as_expected(self) -> None:
        """Rate, fillers, pauses, restarts and repeats match the hand count."""
        got = dataclasses.asdict(turn_features(_turn(3)))
        expected = FIXTURE["expected"]["turn_3"]
        tolerance = FIXTURE["tolerance"]
        for key, value in expected.items():
            if key in tolerance:
                assert abs(got[key] - value) <= tolerance[key], (key, got[key], value)
            else:
                assert got[key] == value, (key, got[key], value)

    def test_a_short_turn_is_not_assessable_never_low(self) -> None:
        """Five words carry no rate worth stating; the status says so."""
        got = turn_features(_turn(5))
        assert got.status == "not_assessable"
        assert list(got.warnings) == FIXTURE["expected"]["turn_5"]["warnings"]

    def test_the_session_aggregates_only_what_is_assessable(self) -> None:
        """Totals over assessable turns, status partial when some were not."""
        got = session_features(FIXTURE["turns"])
        expected = FIXTURE["expected"]["session"]
        assert got.status == expected["status"]
        assert got.words == expected["words"]
        assert (
            abs(got.words_per_minute - expected["words_per_minute"])
            <= FIXTURE["tolerance"]["words_per_minute"]
        )
        assert "AUDIO_QUALITY_NOT_COMPUTED" in got.warnings


class TestReproducibilityAndProvenance:
    """The first two boxes."""

    def test_same_inputs_same_values_and_every_value_names_its_calculator(self) -> None:
        """Deterministic, and versioned on every turn and the session."""
        first = session_features(FIXTURE["turns"])
        second = session_features(json.loads(json.dumps(FIXTURE["turns"])))
        assert first == second
        assert first.calculation_version == CALCULATION_VERSION
        for turn in first.turns:
            assert turn.calculation_version == CALCULATION_VERSION
            assert turn.sequence in (3, 5)

    def test_no_model_is_involved(self) -> None:
        """The module imports nothing that could consult a model."""
        import prepeet_ai.articulation.features as features

        source = pathlib.Path(features.__file__).read_text()
        for forbidden in ("anthropic", "openai", "livekit", "requests", "urllib"):
            assert forbidden not in source


class TestHonestFloors:
    """What the calculator refuses to guess."""

    def test_no_word_timing_means_not_assessable(self) -> None:
        """Text alone cannot yield pauses; the turn says so rather than zero."""
        turn = {**_turn(3), "words": []}
        got = turn_features(turn)
        assert got.status == "not_assessable"
        assert "NO_WORD_TIMING" in got.warnings

    def test_low_confidence_transcript_is_not_assessable(self) -> None:
        """An unreliable transcript yields no rate a coach may quote."""
        turn = dict(_turn(3))
        turn["words"] = [{**w, "confidence": 0.3} for w in turn["words"]]
        got = turn_features(turn)
        assert got.status == "not_assessable"
        assert "TRANSCRIPT_CONFIDENCE_LOW" in got.warnings

    def test_only_candidate_turns_are_measured(self) -> None:
        """The interviewer's delivery is nobody's business here."""
        got = session_features(FIXTURE["turns"])
        assert all(t.sequence != 2 for t in got.turns)

    def test_silence_is_not_a_low_result(self) -> None:
        """No candidate speech at all is not_assessable with the reason named."""
        got = session_features([_turn(2)])
        assert got.status == "not_assessable"
        assert "NO_CANDIDATE_SPEECH" in got.warnings


class TestOverTheWire:
    """AnalyzeArticulation serves the calculator, digest-verified, model-free."""

    def test_the_analysis_arrives_with_its_calculator_version(self, tmp_path) -> None:
        """Measured values on the wire, named by the calculator that produced them."""
        import hashlib
        import http.server
        import threading

        import grpc
        from prepeet.intelligence.v1 import intelligence_pb2, intelligence_pb2_grpc

        from prepeet_ai.transport import server as transport

        document = json.dumps({"session_id": "ses-3", "turns": FIXTURE["turns"]}).encode()
        digest = "sha256:" + hashlib.sha256(document).hexdigest()

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self) -> None:
                self.send_response(200)
                self.send_header("Content-Length", str(len(document)))
                self.end_headers()
                self.wfile.write(document)

            def log_message(self, *args: object) -> None:
                """Quiet."""

        httpd = http.server.HTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=httpd.serve_forever, daemon=True).start()
        served, port = transport.serve(port=0)
        channel = grpc.insecure_channel(f"localhost:{port}")
        stub = intelligence_pb2_grpc.IntelligenceServiceStub(channel)
        try:
            response = stub.AnalyzeArticulation(
                intelligence_pb2.AnalyzeArticulationRequest(
                    context=intelligence_pb2.RequestContext(schema_version="1.0", request_id="r"),
                    session_id="ses-3",
                    bundle_digest="sha256:bundle",
                    manifest=intelligence_pb2.ObjectRef(
                        storage_key="candidate/u/session/s/transcript/evaluation-input.json",
                        digest=digest,
                        media_type="application/json",
                        fetch_url=f"http://127.0.0.1:{httpd.server_port}/input.json",
                    ),
                )
            )
        finally:
            channel.close()
            served.stop(grace=None)
            httpd.shutdown()

        assert response.meta.calculation_version == CALCULATION_VERSION
        assert response.meta.usage.provider_calls == 0
        analysis = json.loads(response.analysis)
        assert analysis["assessability"]["status"] == "partially_assessable"
        assert analysis["assessability"]["audio_quality"] is None
        assert abs(analysis["metrics"]["words_per_minute"] - 120.0) <= 0.5
