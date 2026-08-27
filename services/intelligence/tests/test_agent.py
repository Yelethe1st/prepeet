"""The voice agent's conversation, proven against fakes.

The agent is the transcript's source of truth: everything said becomes a
final segment with words on the room clock, in order, with turn
boundaries the agent decided. The room, providers and network are ports;
what is asserted here is the orchestration and the timeline client's
request shape.
"""

from __future__ import annotations

import asyncio
import http.server
import json
import threading
from collections.abc import AsyncIterator

from prepeet_ai.agent.clock import RoomClock
from prepeet_ai.agent.conversation import Conversation, spoken_segment
from prepeet_ai.agent.ports import TranscriptSegment, TranscriptWord
from prepeet_ai.agent.scripted import ScriptedInterviewer
from prepeet_ai.agent.timeline import PlatformTimeline, TimelineRefusedError, TimelineTarget


class FakeTTS:
    """Records what was spoken; each utterance takes 100 ms per word."""

    def __init__(self, clock: FakeClock) -> None:
        """Advance the shared clock as if speaking took time."""
        self.spoken: list[str] = []
        self._clock = clock

    async def speak(self, text: str) -> int:
        """Pretend to speak, advancing the clock."""
        self.spoken.append(text)
        duration = 100 * len(text.split())
        self._clock.advance(duration)
        return duration


class FakeClock(RoomClock):
    """A room clock the test advances by hand."""

    def __init__(self) -> None:
        """Start at zero."""
        self._ms = 0

    def advance(self, ms: int) -> None:
        """Move time forward."""
        self._ms += ms

    def now_ms(self, now: float | None = None) -> int:
        """The hand-advanced time."""
        return self._ms


class FakeSTT:
    """Yields the candidate's scripted answers, each after a pause."""

    def __init__(self, clock: FakeClock, answers: list[str]) -> None:
        """Remember the answers and the clock to stamp them on."""
        self._clock = clock
        self._answers = answers

    async def segments(self) -> AsyncIterator[TranscriptSegment]:
        """Yield one final segment per answer, timed on the room clock."""
        for answer in self._answers:
            self._clock.advance(500)
            start = self._clock.now_ms()
            words = answer.split()
            timed = tuple(
                TranscriptWord(
                    text=word,
                    start_ms=start + i * 300,
                    end_ms=start + i * 300 + 250,
                    confidence=0.9,
                )
                for i, word in enumerate(words)
            )
            end = timed[-1].end_ms if timed else start + 1
            self._clock.advance(end - start)
            yield TranscriptSegment(
                speaker="candidate",
                text=answer,
                start_ms=start,
                end_ms=end,
                confidence=0.9,
                words=timed,
            )


class FakeTimeline:
    """Records every batch posted."""

    def __init__(self) -> None:
        """Start empty."""
        self.batches: list[list[dict[str, object]]] = []

    async def post(self, events: list[dict[str, object]]) -> None:
        """Keep the batch."""
        self.batches.append(events)


def run_conversation(answers: list[str]) -> tuple[FakeTTS, FakeTimeline]:
    """Run a scripted interview end to end against the fakes."""
    clock = FakeClock()
    tts = FakeTTS(clock)
    timeline = FakeTimeline()
    conversation = Conversation(
        interviewer=ScriptedInterviewer(
            "Welcome. Tell me about yourself.",
            ["What did you build last year?", "What would you change?"],
        ),
        stt=FakeSTT(clock, answers),
        tts=tts,
        timeline=timeline,
        clock=clock,
    )
    asyncio.run(conversation.run())
    return tts, timeline


class TestTheConversationIsTheTranscriptsSourceOfTruth:
    """Everything said lands, in order, with words on one clock."""

    def test_the_agent_speaks_the_opening_and_posts_it_before_listening(self) -> None:
        """The first batch is the agent's own words as a final segment."""
        tts, timeline = run_conversation(["I built a payments migration"])

        assert tts.spoken[0] == "Welcome. Tell me about yourself."
        first = timeline.batches[0]
        assert first[0]["type"] == "transcript.segment.final"
        payload = first[0]["payload"]
        assert isinstance(payload, dict)
        assert payload["speaker"] == "interviewer"
        assert payload["text"] == "Welcome. Tell me about yourself."
        assert first[1]["type"] == "turn.boundary"
        boundary = first[1]["payload"]
        assert isinstance(boundary, dict)
        assert boundary["speaker"] == "interviewer"
        # Measured, stored: ADR-0012's budget is checked against numbers.
        assert "latency_ms" in boundary

    def test_every_segment_has_words_inside_its_span_on_a_monotonic_clock(self) -> None:
        """The contract's rule, held here before the server ever sees it."""
        _, timeline = run_conversation(["I built a payments migration", "Less coupling"])

        last_end = -1
        for batch in timeline.batches:
            for event in batch:
                if event["type"] != "transcript.segment.final":
                    continue
                payload = event["payload"]
                assert isinstance(payload, dict)
                assert payload["start_ms"] >= last_end
                assert payload["end_ms"] > payload["start_ms"]
                words = payload["words"]
                assert isinstance(words, list) and words
                for word in words:
                    assert (
                        payload["start_ms"]
                        <= word["start_ms"]
                        < word["end_ms"]
                        <= payload["end_ms"]
                    )
                last_end = int(payload["end_ms"])

    def test_the_candidates_words_are_posted_verbatim(self) -> None:
        """What the candidate said is what the timeline gets, unedited."""
        _, timeline = run_conversation(["I built a payments migration"])

        candidate = [
            event["payload"]
            for batch in timeline.batches
            for event in batch
            if event["type"] == "transcript.segment.final"
            and isinstance(event["payload"], dict)
            and event["payload"]["speaker"] == "candidate"
        ]
        assert len(candidate) == 1
        assert candidate[0]["text"] == "I built a payments migration"

    def test_the_script_ends_the_interview_after_the_last_answer(self) -> None:
        """Two questions, two answers, then silence: no filler question."""
        tts, _ = run_conversation(["one", "two", "three"])

        assert tts.spoken == [
            "Welcome. Tell me about yourself.",
            "What did you build last year?",
            "What would you change?",
        ]


class TestSpokenSegments:
    """The interviewer's own words become an honest, approximate segment."""

    def test_words_spread_evenly_and_stay_inside_the_span(self) -> None:
        """Even spacing over the measured duration, never outside it."""
        segment = spoken_segment("Tell me about it", start_ms=1000, duration_ms=2000)

        assert segment.start_ms == 1000
        assert segment.end_ms == 3000
        assert [w.text for w in segment.words] == ["Tell", "me", "about", "it"]
        for word in segment.words:
            assert 1000 <= word.start_ms < word.end_ms <= 3000


class TestTheTimelineClient:
    """The request the platform's internal surface expects, exactly."""

    def test_posts_with_the_bearer_and_the_candidate_and_no_sequences(self) -> None:
        """Token, body shape, and the URL, observed by a real HTTP server."""
        received: dict[str, object] = {}

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_POST(self) -> None:
                length = int(self.headers.get("Content-Length", "0"))
                received["path"] = self.path
                received["authorization"] = self.headers.get("Authorization")
                received["body"] = json.loads(self.rfile.read(length))
                self.send_response(200)
                self.send_header("Content-Length", "2")
                self.end_headers()
                self.wfile.write(b"{}")

            def log_message(self, *args: object) -> None:
                """Quiet."""

        httpd = http.server.HTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=httpd.serve_forever, daemon=True).start()
        try:
            timeline = PlatformTimeline(
                TimelineTarget(
                    api_url=f"http://127.0.0.1:{httpd.server_port}",
                    service_token="agent-secret",
                    session_id="ses-1",
                    candidate_id="cand-1",
                )
            )
            asyncio.run(timeline.post([{"event_id": "e1", "type": "turn.boundary"}]))
        finally:
            httpd.shutdown()

        assert received["path"] == "/api/v1/internal/interviews/ses-1/events"
        assert received["authorization"] == "Bearer agent-secret"
        body = received["body"]
        assert isinstance(body, dict)
        assert body["candidate_id"] == "cand-1"
        assert body["mode"] == "practice"
        assert "sequence" not in body["events"][0]

    def test_a_refusal_is_raised_with_the_platforms_answer(self) -> None:
        """A 401 is not swallowed: the agent must know it is not writing."""

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_POST(self) -> None:
                body = b'{"error":{}}'
                self.send_response(401)
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *args: object) -> None:
                """Quiet."""

        httpd = http.server.HTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=httpd.serve_forever, daemon=True).start()
        try:
            timeline = PlatformTimeline(
                TimelineTarget(
                    api_url=f"http://127.0.0.1:{httpd.server_port}",
                    service_token="wrong",
                    session_id="ses-1",
                    candidate_id="cand-1",
                )
            )
            try:
                asyncio.run(timeline.post([]))
            except TimelineRefusedError as refused:
                assert refused.status == 401
            else:
                raise AssertionError("a 401 was swallowed")
        finally:
            httpd.shutdown()
