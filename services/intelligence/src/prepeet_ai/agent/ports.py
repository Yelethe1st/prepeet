"""The agent's ports: what the conversation needs, declared by the consumer.

Providers (ADR-0019) sit behind these. The contract is the platform's, not
the provider's: a transcript segment is words on the room clock with
confidence, exactly what the durable timeline stores, so an adapter is a
mapping and never a reconstruction.
"""

from __future__ import annotations

from collections.abc import AsyncIterator
from dataclasses import dataclass, field
from typing import Protocol


@dataclass(frozen=True)
class TranscriptWord:
    """One word on the room clock."""

    text: str
    start_ms: int
    end_ms: int
    confidence: float


@dataclass(frozen=True)
class TranscriptSegment:
    """One final utterance: exact text, its span, its words."""

    speaker: str
    text: str
    start_ms: int
    end_ms: int
    confidence: float
    words: tuple[TranscriptWord, ...] = field(default_factory=tuple)


class SpeechToText(Protocol):
    """Final candidate segments, as they are recognised."""

    def segments(self) -> AsyncIterator[TranscriptSegment]:
        """Yield final segments on the room clock until the candidate leaves."""
        ...


class TextToSpeech(Protocol):
    """Speak one utterance, answering how long it took on the room clock."""

    async def speak(self, text: str) -> int:
        """Synthesize and play text; return the utterance's duration in ms."""
        ...


@dataclass(frozen=True)
class Turn:
    """What the interviewer decided to say, and how long deciding took.

    latency_ms is measured, never asserted (ADR-0012's budget is checked
    against these), and rides the turn boundary into the durable timeline.
    """

    text: str
    latency_ms: int
    # Which provider and model decided this turn, or "scripted".
    model_version: str = "scripted"


class Interviewer(Protocol):
    """Decides what to say next. Scripted, or a model behind ADR-0019."""

    async def opening(self) -> Turn:
        """The first thing said in the room."""
        ...

    async def next_question(self, candidate_said: str) -> Turn | None:
        """The next question, or None when the interview is over."""
        ...


class Timeline(Protocol):
    """The durable timeline the agent is the source of truth for."""

    async def post(self, events: list[dict[str, object]]) -> None:
        """Land a batch of control events; the server assigns sequences."""
        ...
