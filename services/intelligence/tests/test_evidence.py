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


CONTRADICTING_TURNS = [
    {
        "sequence": 3,
        "speaker": "candidate",
        "text": "I led the payments migration team of 5 engineers.",
        "start_ms": 5000,
        "end_ms": 9000,
    },
    {
        "sequence": 5,
        "speaker": "candidate",
        "text": "The payments migration team I led was 12 people.",
        "start_ms": 15000,
        "end_ms": 19000,
    },
]


class TestContradictionsAreNeutralPairs:
    """EVL-04's detector: neutral pairs, both sides quoted on the clock."""

    def test_both_sides_are_quoted_exactly_with_timing_inside_their_turns(self) -> None:
        """Each side slices its own turn and sits on its clock."""
        from prepeet_ai.evaluation.evidence import extract_contradictions

        pairs = extract_contradictions(CONTRADICTING_TURNS)

        assert len(pairs) == 1
        pair = pairs[0]
        sides = ((pair.side_a, CONTRADICTING_TURNS[0]), (pair.side_b, CONTRADICTING_TURNS[1]))
        for side, turn in sides:
            assert side.segment_sequence == turn["sequence"]
            assert turn["text"][side.char_start : side.char_end] == side.quote
            assert turn["start_ms"] <= side.start_ms <= side.end_ms <= turn["end_ms"]
        assert pair.extraction_version == EXTRACTION_VERSION

    def test_a_restated_number_is_consistency_not_contradiction(self) -> None:
        """The same number twice is agreement."""
        from prepeet_ai.evaluation.evidence import extract_contradictions

        turns = [
            dict(CONTRADICTING_TURNS[0]),
            {
                "sequence": 5,
                "speaker": "candidate",
                "text": "The payments migration team had 5 engineers as I said.",
                "start_ms": 15000,
                "end_ms": 19000,
            },
        ]
        assert extract_contradictions(turns) == []

    def test_unrelated_numbers_do_not_pair(self) -> None:
        """Different measurements about different subjects stay apart."""
        from prepeet_ai.evaluation.evidence import extract_contradictions

        turns = [
            dict(CONTRADICTING_TURNS[0]),
            {
                "sequence": 5,
                "speaker": "candidate",
                "text": "Latency dropped 40 percent after the cache rollout.",
                "start_ms": 15000,
                "end_ms": 19000,
            },
        ]
        assert extract_contradictions(turns) == []

    def test_interviewer_statements_never_form_a_side(self) -> None:
        """Only the candidate's own words can conflict."""
        from prepeet_ai.evaluation.evidence import extract_contradictions

        turns = [
            {**CONTRADICTING_TURNS[0], "speaker": "interviewer"},
            dict(CONTRADICTING_TURNS[1]),
        ]
        assert extract_contradictions(turns) == []

    def test_detection_is_deterministic(self) -> None:
        """Same turns, same pairs, always."""
        from prepeet_ai.evaluation.evidence import extract_contradictions

        first = extract_contradictions(CONTRADICTING_TURNS)
        second = extract_contradictions(list(CONTRADICTING_TURNS))
        assert first == second

    def test_the_vocabulary_never_judges_the_person(self) -> None:
        """The pair describes two statements, never a character."""
        import dataclasses
        import json as jsonlib

        from prepeet_ai.evaluation.evidence import extract_contradictions

        pairs = extract_contradictions(CONTRADICTING_TURNS)
        serialized = jsonlib.dumps([dataclasses.asdict(pair) for pair in pairs]).lower()
        forbidden_terms = (
            "honest",
            "dishonest",
            "integrity",
            "credib",
            "lie",
            "lying",
            "deceit",
            "decept",
            "truth",
        )
        for forbidden in forbidden_terms:
            assert forbidden not in serialized


class TestContradictionsOverTheWire:
    """The pair reaches the observation stream, both sides intact."""

    def test_a_contradiction_arrives_as_its_own_observation_kind(self, tmp_path) -> None:
        """The pair reaches the stream with kind contradiction."""
        import hashlib
        import http.server
        import json as jsonlib
        import threading

        import grpc
        from prepeet.intelligence.v1 import intelligence_pb2, intelligence_pb2_grpc

        from prepeet_ai.transport import server as transport

        turns = CONTRADICTING_TURNS
        document = jsonlib.dumps(
            {"session_id": "ses-2", "competencies": COMPETENCIES, "turns": turns}
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
                        schema_version="1.0", request_id="ses-2"
                    ),
                    session_id="ses-2",
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

        contradictions = [
            jsonlib.loads(observation.observation)
            for observation in response.observations
            if jsonlib.loads(observation.observation)["kind"] == "contradiction"
        ]
        assert len(contradictions) == 1
        pair = contradictions[0]
        by_sequence = {turn["sequence"]: turn for turn in turns}
        for side in (pair["side_a"], pair["side_b"]):
            turn = by_sequence[side["segment_sequence"]]
            assert turn["text"][side["char_start"] : side["char_end"]] == side["quote"]
            assert turn["start_ms"] <= side["start_ms"] <= side["end_ms"] <= turn["end_ms"]
        # Neutral at the wire too: two statements, no verdict on a person.
        serialized = jsonlib.dumps(pair).lower()
        for forbidden in ("honest", "integrity", "credib", "lie", "decept", "truth"):
            assert forbidden not in serialized


class TestLatencyIsInvisibleToExtraction:
    """SES-05 on the extraction path: pauses shift clocks, nothing else."""

    def test_response_latency_changes_only_provenance_clocks(self) -> None:
        """The same words after a long pause read identically."""
        import dataclasses

        prompt_turns = [dict(turn) for turn in TURNS]
        delayed_turns = []
        for turn in TURNS:
            shifted = dict(turn)
            shifted["start_ms"] = int(turn["start_ms"]) + 90_000
            shifted["end_ms"] = int(turn["end_ms"]) + 90_000
            if "words" in shifted:
                shifted["words"] = [
                    {
                        **word,
                        "start_ms": word["start_ms"] + 90_000,
                        "end_ms": word["end_ms"] + 90_000,
                    }
                    for word in shifted["words"]
                ]
            delayed_turns.append(shifted)

        prompt = extract_evidence(prompt_turns, COMPETENCIES)
        delayed = extract_evidence(delayed_turns, COMPETENCIES)

        assert len(prompt) == len(delayed) > 0
        for a, b in zip(prompt, delayed, strict=True):
            fast = dataclasses.asdict(a)
            slow = dataclasses.asdict(b)
            fast.pop("start_ms"), fast.pop("end_ms")
            slow.pop("start_ms"), slow.pop("end_ms")
            assert fast == slow
