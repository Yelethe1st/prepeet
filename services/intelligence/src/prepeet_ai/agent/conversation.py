"""The conversation: speak, listen, write the timeline, in that order.

Pure orchestration over the ports. The agent is the transcript's source of
truth (ADR-0019), so everything said in the room - its own utterances and
the candidate's - becomes a final segment with words on the room clock,
posted through the timeline as it happens. Turn boundaries are the agent's
own decision, recorded as events, so evaluation never infers them.

The interviewer's own words carry word timings spread evenly across the
utterance's measured duration: a deterministic derivation from what was
said and how long it took, honest about being approximate, and enough for
the contract's requirement that final segments carry words.
"""

from __future__ import annotations

import uuid
from datetime import UTC, datetime

from prepeet_ai.agent.clock import RoomClock
from prepeet_ai.agent.ports import (
    Interviewer,
    SpeechToText,
    TextToSpeech,
    Timeline,
    TranscriptSegment,
    TranscriptWord,
)


def _event(kind: str, payload: dict[str, object] | None = None) -> dict[str, object]:
    event: dict[str, object] = {
        "event_id": str(uuid.uuid4()),
        "type": kind,
        "occurred_at": datetime.now(UTC).isoformat().replace("+00:00", "Z"),
    }
    if payload is not None:
        event["payload"] = payload
    return event


def segment_payload(segment: TranscriptSegment) -> dict[str, object]:
    """The transcript.segment.final payload the timeline validates."""
    return {
        "speaker": segment.speaker,
        "text": segment.text,
        "start_ms": segment.start_ms,
        "end_ms": segment.end_ms,
        "confidence": segment.confidence,
        "words": [
            {
                "w": word.text,
                "start_ms": word.start_ms,
                "end_ms": word.end_ms,
                "confidence": word.confidence,
            }
            for word in segment.words
        ],
    }


def spoken_segment(text: str, start_ms: int, duration_ms: int) -> TranscriptSegment:
    """The interviewer's own utterance as a segment, words spread evenly."""
    words = text.split()
    duration = max(duration_ms, len(words) or 1)
    per_word = duration / max(len(words), 1)
    timed: list[TranscriptWord] = []
    for index, word in enumerate(words):
        word_start = start_ms + int(index * per_word)
        word_end = start_ms + int((index + 1) * per_word)
        timed.append(
            TranscriptWord(
                text=word,
                start_ms=word_start,
                end_ms=max(word_end, word_start + 1),
                confidence=1.0,
            )
        )
    end_ms = start_ms + duration
    if timed and timed[-1].end_ms > end_ms:
        end_ms = timed[-1].end_ms
    return TranscriptSegment(
        speaker="interviewer",
        text=text,
        start_ms=start_ms,
        end_ms=end_ms,
        confidence=1.0,
        words=tuple(timed),
    )


class Conversation:
    """One interview's turn loop."""

    def __init__(
        self,
        interviewer: Interviewer,
        stt: SpeechToText,
        tts: TextToSpeech,
        timeline: Timeline,
        clock: RoomClock,
    ) -> None:
        """Wire the ports for one room."""
        self._interviewer = interviewer
        self._stt = stt
        self._tts = tts
        self._timeline = timeline
        self._clock = clock

    async def _say(self, text: str) -> None:
        start_ms = self._clock.now_ms()
        duration_ms = await self._tts.speak(text)
        segment = spoken_segment(text, start_ms, duration_ms)
        await self._timeline.post(
            [
                _event("transcript.segment.final", segment_payload(segment)),
                _event("turn.boundary"),
            ]
        )

    async def run(self) -> None:
        """Speak the opening, then alternate: hear an answer, ask the next question."""
        await self._say(self._interviewer.opening())

        async for heard in self._stt.segments():
            await self._timeline.post(
                [
                    _event("transcript.segment.final", segment_payload(heard)),
                    _event("turn.boundary"),
                ]
            )
            question = self._interviewer.next_question(heard.text)
            if question is None:
                return
            await self._say(question)
