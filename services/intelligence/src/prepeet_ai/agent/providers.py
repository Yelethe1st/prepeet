"""Provider adapters (ADR-0019), constructed only from configuration.

Deepgram hears, Cartesia speaks, both through LiveKit's plugin interfaces
so the audio plumbing is the SDK's and the mapping onto our ports is the
only code here. Keys are deployment secrets; nothing in this module runs
under test, which is why the conversation logic lives elsewhere.
"""

from __future__ import annotations

import os
from collections.abc import AsyncIterator
from dataclasses import dataclass

from prepeet_ai.agent.ports import TranscriptSegment, TranscriptWord


@dataclass(frozen=True)
class ProviderConfig:
    """What the deployment supplies. Missing keys mean the adapter is absent."""

    deepgram_api_key: str
    cartesia_api_key: str
    cartesia_voice: str

    @classmethod
    def from_env(cls) -> ProviderConfig:
        """Read the keys the deployment set."""
        return cls(
            deepgram_api_key=os.environ.get("PREPEET_DEEPGRAM_API_KEY", ""),
            cartesia_api_key=os.environ.get("PREPEET_CARTESIA_API_KEY", ""),
            cartesia_voice=os.environ.get("PREPEET_CARTESIA_VOICE", ""),
        )

    @property
    def complete(self) -> bool:
        """Whether every provider the loop needs is configured."""
        return bool(self.deepgram_api_key and self.cartesia_api_key)


def segment_from_deepgram(  # pragma: no cover - mapping over a live plugin event
    text: str,
    words: list[tuple[str, float, float, float]],
    offset_ms: int,
    confidence: float,
) -> TranscriptSegment:
    """Map one final Deepgram alternative onto a segment on the room clock.

    Deepgram times words from the start of the audio stream; offset_ms is
    where that stream began on the room clock, so the mapping is addition,
    never reconstruction.
    """
    timed = tuple(
        TranscriptWord(
            text=word,
            start_ms=offset_ms + int(start * 1000),
            end_ms=offset_ms + int(end * 1000),
            confidence=word_confidence,
        )
        for word, start, end, word_confidence in words
    )
    start_ms = timed[0].start_ms if timed else offset_ms
    end_ms = timed[-1].end_ms if timed else offset_ms + 1
    return TranscriptSegment(
        speaker="candidate",
        text=text,
        start_ms=start_ms,
        end_ms=max(end_ms, start_ms + 1),
        confidence=confidence,
        words=timed,
    )


class NoSpeech:
    """The absent hearing port.

    Yields nothing, so a misconfigured deployment runs a silent interview
    rather than a crashing one; the worker logs why.
    """

    async def segments(self) -> AsyncIterator[TranscriptSegment]:
        """Yield nothing."""
        nothing: tuple[TranscriptSegment, ...] = ()
        for segment in nothing:  # pragma: no cover - the empty generator idiom
            yield segment
