"""Evidence extraction, proven at both layers.

Every span is an exact substring of a real turn with timing inside it, the
reading is deterministic, and silence about a competency yields nothing:
absence of evidence is never coerced into a span.
"""

from __future__ import annotations

from prepeet_ai.evaluation.evidence import EXTRACTION_VERSION, extract_evidence

_TURN_TEXT = (
    "I redesigned the checkout systems design to shard by region. "
    "Latency dropped 40 percent in three months. "
    "For debugging I am not sure I have a strong example."
)
_TURN_WORDS = _TURN_TEXT.split(" ")

COMPETENCIES = [
    {"id": "systems-design", "name": "Systems design"},
    {"id": "debugging", "name": "Debugging & incident response"},
]

TURNS = [
    {
        "sequence": 2,
        "speaker": "interviewer",
        "text": "Tell me about a systems design decision you made.",
        "start_ms": 1000,
        "end_ms": 4000,
    },
    {
        "sequence": 3,
        "speaker": "candidate",
        "text": _TURN_TEXT,
        "start_ms": 5000,
        "end_ms": 20000,
        "words": [
            {"w": w, "start_ms": 5000 + i * 500, "end_ms": 5300 + i * 500, "confidence": 0.95}
            for i, w in enumerate(_TURN_WORDS)
        ],
    },
]


class TestSpansAreHonest:
    """The anti-fabrication properties, at the source."""

    def test_every_span_is_an_exact_substring_with_timing_inside_its_turn(self) -> None:
        """Slicing the turn by the span yields the quote, timing inside."""
        spans = extract_evidence(TURNS, COMPETENCIES)

        assert spans, "the fixture plainly evidences systems design"
        by_sequence = {turn["sequence"]: turn for turn in TURNS}
        for span in spans:
            turn = by_sequence[span.segment_sequence]
            assert turn["text"][span.char_start : span.char_end] == span.quote
            assert turn["start_ms"] <= span.start_ms < span.end_ms <= turn["end_ms"]
            assert span.extraction_version == EXTRACTION_VERSION

    def test_only_candidate_turns_yield_evidence(self) -> None:
        """The interviewer's words are context, never the candidate's evidence."""
        spans = extract_evidence(TURNS, COMPETENCIES)

        assert all(span.segment_sequence == 3 for span in spans)

    def test_a_measured_outcome_is_supporting_and_uncertainty_is_a_gap(self) -> None:
        """A number makes support; admitted uncertainty is a gap, not a zero."""
        spans = extract_evidence(TURNS, COMPETENCIES)
        kinds = {(span.competency_id, span.kind) for span in spans}

        assert ("systems-design", "supporting") in kinds
        assert ("debugging", "gap") in kinds

    def test_silence_about_a_competency_yields_nothing(self) -> None:
        """Absence of evidence is absence: never coerced into a span."""
        quiet = [
            {
                "sequence": 3,
                "speaker": "candidate",
                "text": "I mostly wrote documentation last year.",
                "start_ms": 0,
                "end_ms": 3000,
            }
        ]
        spans = extract_evidence(quiet, COMPETENCIES)

        assert spans == []

    def test_extraction_is_deterministic(self) -> None:
        """A retried extraction must converge, not duplicate."""
        first = extract_evidence(TURNS, COMPETENCIES)
        second = extract_evidence(TURNS, COMPETENCIES)

        assert first == second

    def test_word_timing_tightens_the_span_when_words_exist(self) -> None:
        """The clock range belongs to the quoted words, not the whole turn."""
        spans = extract_evidence(TURNS, COMPETENCIES)
        supporting = next(s for s in spans if s.kind == "supporting")
        turn = TURNS[1]
        turn_words = turn["words"]

        # The span's clock range is the quoted words' own, not the whole
        # turn's: it starts at the quote's first word and ends before the
        # turn does, because the quote does.
        first_word = len(turn["text"][: supporting.char_start].split())
        assert supporting.start_ms == turn_words[first_word]["start_ms"]
        assert supporting.end_ms < turn["end_ms"]


class TestOverTheWire:
    """The served contract, with the fetch and digest verification real."""

    def test_observations_arrive_span_linked(self, tmp_path) -> None:
        """Each observation's quote slices its turn exactly, over the wire."""
        import hashlib
        import http.server
        import json as jsonlib
        import threading

        import grpc
        from prepeet.intelligence.v1 import intelligence_pb2, intelligence_pb2_grpc

        from prepeet_ai.transport import server as transport

        document = jsonlib.dumps(
            {"session_id": "ses-1", "competencies": COMPETENCIES, "turns": TURNS}
        ).encode()
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
            response = stub.EvaluateTurns(
                intelligence_pb2.EvaluateTurnsRequest(
                    context=intelligence_pb2.RequestContext(
                        schema_version="1.0", request_id="ses-1"
                    ),
                    session_id="ses-1",
                    bundle_digest="sha256:bundle",
                    turns=[
                        intelligence_pb2.ObjectRef(
                            storage_key="candidate/u/session/s/transcript/evaluation-input.json",
                            digest=digest,
                            media_type="application/json",
                            fetch_url=f"http://127.0.0.1:{httpd.server_port}/input.json",
                        )
                    ],
                )
            )
        finally:
            channel.close()
            served.stop(grace=None)
            httpd.shutdown()

        assert response.meta.calculation_version == EXTRACTION_VERSION
        assert response.observations
        by_sequence = {turn["sequence"]: turn for turn in TURNS}
        for observation in response.observations:
            span = jsonlib.loads(observation.observation)
            turn = by_sequence[span["segment_sequence"]]
            assert turn["text"][span["char_start"] : span["char_end"]] == span["quote"]
            assert observation.turn_id == str(span["segment_sequence"])
