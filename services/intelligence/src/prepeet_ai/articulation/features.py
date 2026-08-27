"""Deterministic delivery features: articulation-features-v1 (ART-01).

Everything here is arithmetic over the sealed transcript's word timings
and confidences. No model is consulted and none could be: words per
minute, pauses, fillers, restarts and repeated phrases are counted, not
judged. Each result carries its calculator version and the sequence of
the turn it came from, so a value is reproducible from the same inputs
forever and a coaching statement can point at exactly where it came from.

Audio-derived quality (clipping, noise, volume) is not computed at this
floor: the recording is not decoded here yet, and inventing a number for
it would be the exact fabrication the spec forbids. The result says so by
status and warning, never by a made-up value.
"""

from __future__ import annotations

import re
from collections.abc import Sequence
from dataclasses import dataclass, field
from itertools import pairwise
from typing import Any

CALCULATION_VERSION = "articulation-features-v1"

LONG_PAUSE_MS = 700
"""A gap between words a listener notices as a pause, not a breath."""

MIN_WORDS_FOR_ASSESSMENT = 20
"""Fewer words than this and rates are noise: not_assessable, never low."""

CONFIDENCE_FLOOR = 0.6
"""Mean word confidence below this makes the transcript itself unreliable."""

FILLERS = frozenset({"um", "uh", "er", "erm", "ah", "hmm", "mm", "uhm", "umm"})
"""Unambiguous vocal fillers. Words like 'like' and 'so' are not counted:
they are fillers sometimes and content the rest of the time, and a count
that guesses is worse than one that abstains."""

_TOKEN = re.compile(r"[a-z0-9']+")

CLIPPING_FLOOR = 0.02
"""Share of samples at full scale above which the recording is clipped:
one in fifty samples pinned to the rail is distortion a listener hears,
and pace or pauses measured through it are not the candidate's."""

SILENCE_FLOOR = 0.9
"""Share of near-silent samples above which there is no speech to measure."""

NOT_A_LOW_RESULT = (
    "Delivery was not assessable for this session. That is a statement about the "
    "recording or the transcript, not about you: it is not a low result, and it has "
    "not affected any score."
)
"""Ships with every not-assessable result, so no surface can drop it."""


@dataclass(frozen=True)
class AudioQuality:
    """What the recording's samples say about themselves."""

    calculation_version: str
    clipping_ratio: float
    silence_ratio: float
    status: str
    warnings: tuple[str, ...]


def audio_quality(samples: Sequence[float], full_scale: float = 0.99) -> AudioQuality:
    """Measure clipping and silence over normalised samples in [-1, 1].

    Deterministic and model-free like everything here. Samples arrive
    decoded by whoever holds the recording; this function never decodes,
    so it can be proven on synthetic fixtures before any decoder exists.
    """
    total = len(samples)
    if total == 0:
        return AudioQuality(CALCULATION_VERSION, 0.0, 1.0, "not_assessable", ("NO_AUDIO",))
    clipped = sum(1 for sample in samples if abs(sample) >= full_scale)
    silent = sum(1 for sample in samples if abs(sample) < 0.01)
    clipping_ratio = round(clipped / total, 4)
    silence_ratio = round(silent / total, 4)
    warnings: list[str] = []
    status = "assessable"
    if clipping_ratio > CLIPPING_FLOOR:
        status = "not_assessable"
        warnings.append("AUDIO_CLIPPED")
    if silence_ratio > SILENCE_FLOOR:
        status = "not_assessable"
        warnings.append("AUDIO_SILENT")
    return AudioQuality(CALCULATION_VERSION, clipping_ratio, silence_ratio, status, tuple(warnings))


@dataclass(frozen=True)
class TurnFeatures:
    """One candidate turn's measurements, with what produced them."""

    sequence: int
    calculation_version: str
    words: int
    duration_ms: int
    words_per_minute: float
    pause_count: int
    average_pause_ms: float
    max_pause_ms: int
    long_pause_count: int
    filler_count: int
    fillers_per_100_words: float
    restart_count: int
    repeated_phrase_count: int
    transcript_confidence: float
    status: str
    warnings: tuple[str, ...] = field(default_factory=tuple)


@dataclass(frozen=True)
class SessionFeatures:
    """The session's turns and the honest aggregate."""

    calculation_version: str
    status: str
    turns: tuple[TurnFeatures, ...]
    words: int
    words_per_minute: float
    fillers_per_100_words: float
    long_pause_count: int
    transcript_confidence: float
    warnings: tuple[str, ...]


def _tokens(text: str) -> list[str]:
    return _TOKEN.findall(text.lower())


def _pauses(words: list[dict[str, Any]]) -> list[int]:
    gaps: list[int] = []
    for earlier, later in pairwise(words):
        gap = int(later["start_ms"]) - int(earlier["end_ms"])
        if gap > 0:
            gaps.append(gap)
    return gaps


def _restarts(tokens: list[str]) -> int:
    """Immediate repeats of a word or a two-word phrase: 'I, I think', 'we had we had'."""
    count = 0
    index = 0
    while index < len(tokens) - 1:
        if tokens[index] == tokens[index + 1] and tokens[index] not in FILLERS:
            count += 1
            index += 2
            continue
        if index < len(tokens) - 3 and tokens[index : index + 2] == tokens[index + 2 : index + 4]:
            count += 1
            index += 4
            continue
        index += 1
    return count


def _repeated_phrases(tokens: list[str]) -> int:
    """Distinct three-word phrases said more than once, non-adjacently."""
    seen: dict[tuple[str, ...], int] = {}
    for index in range(len(tokens) - 2):
        phrase = tuple(tokens[index : index + 3])
        if len(set(phrase)) < 2:
            continue
        seen[phrase] = seen.get(phrase, 0) + 1
    return sum(1 for phrase, count in seen.items() if count > 1)


def turn_features(turn: dict[str, Any]) -> TurnFeatures:
    """Measure one turn. Never raises on thin input; it reports it."""
    sequence = int(turn["sequence"])
    text: str = turn.get("text", "")
    tokens = _tokens(text)
    words: list[dict[str, Any]] = list(turn.get("words") or [])
    start_ms, end_ms = int(turn["start_ms"]), int(turn["end_ms"])
    duration_ms = max(0, end_ms - start_ms)
    minutes = duration_ms / 60_000

    warnings: list[str] = []
    word_count = len(tokens)
    wpm = round(word_count / minutes, 1) if minutes > 0 else 0.0

    gaps = _pauses(words)
    long_pauses = [gap for gap in gaps if gap >= LONG_PAUSE_MS]
    filler_count = sum(1 for token in tokens if token in FILLERS)
    fillers_per_100 = round(100 * filler_count / word_count, 1) if word_count else 0.0
    confidence = (
        round(sum(float(w.get("confidence", 0.0)) for w in words) / len(words), 3) if words else 0.0
    )

    status = "assessable"
    if word_count < MIN_WORDS_FOR_ASSESSMENT:
        status = "not_assessable"
        warnings.append("INSUFFICIENT_SPEECH")
    if not words:
        status = "not_assessable"
        warnings.append("NO_WORD_TIMING")
    elif confidence < CONFIDENCE_FLOOR:
        status = "not_assessable"
        warnings.append("TRANSCRIPT_CONFIDENCE_LOW")
    warnings.append("AUDIO_QUALITY_NOT_COMPUTED")

    return TurnFeatures(
        sequence=sequence,
        calculation_version=CALCULATION_VERSION,
        words=word_count,
        duration_ms=duration_ms,
        words_per_minute=wpm,
        pause_count=len(gaps),
        average_pause_ms=round(sum(gaps) / len(gaps), 1) if gaps else 0.0,
        max_pause_ms=max(gaps) if gaps else 0,
        long_pause_count=len(long_pauses),
        filler_count=filler_count,
        fillers_per_100_words=fillers_per_100,
        restart_count=_restarts(tokens),
        repeated_phrase_count=_repeated_phrases(tokens),
        transcript_confidence=confidence,
        status=status,
        warnings=tuple(warnings),
    )


def session_features(turns: list[dict[str, Any]]) -> SessionFeatures:
    """Measure every candidate turn and aggregate only what is assessable.

    The aggregate rate is computed over assessable turns' totals, never
    averaged across turns, so a long clear answer is not outvoted by a
    two-word one. If nothing is assessable the session is not assessable;
    it is never a low result.
    """
    measured = tuple(turn_features(turn) for turn in turns if turn.get("speaker") == "candidate")
    assessable = [t for t in measured if t.status == "assessable"]
    warnings: list[str] = []
    if not measured:
        warnings.append("NO_CANDIDATE_SPEECH")
    elif not assessable:
        warnings.append("NO_ASSESSABLE_TURNS")
    warnings.append("AUDIO_QUALITY_NOT_COMPUTED")

    total_words = sum(t.words for t in assessable)
    total_ms = sum(t.duration_ms for t in assessable)
    total_fillers = sum(t.filler_count for t in assessable)
    status = "assessable" if assessable else "not_assessable"
    if assessable and len(assessable) < len(measured):
        status = "partially_assessable"
    return SessionFeatures(
        calculation_version=CALCULATION_VERSION,
        status=status,
        turns=measured,
        words=total_words,
        words_per_minute=round(total_words / (total_ms / 60_000), 1) if total_ms else 0.0,
        fillers_per_100_words=round(100 * total_fillers / total_words, 1) if total_words else 0.0,
        long_pause_count=sum(t.long_pause_count for t in assessable),
        transcript_confidence=(
            round(sum(t.transcript_confidence for t in assessable) / len(assessable), 3)
            if assessable
            else 0.0
        ),
        warnings=tuple(warnings),
    )
