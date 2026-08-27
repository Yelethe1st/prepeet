"""The ten-dimension delivery profile: articulation-profile-v1 (ART-03).

Each dimension is a level with the turn sequences that produced it, from
the deterministic features and the words themselves, never from a model
and never collapsed into one number. Levels are strong, solid,
developing, or not_assessable; the rules are stated in code so a
candidate can be told exactly why, and a level that cannot be measured
at this floor says so rather than pretending.

Intelligibility is followability: could a listener follow the words as
transcribed? It is measured from transcript confidence and sentence
length only. There is no accent-conformity component and, by test, no
input that could carry one.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Any

from prepeet_ai.articulation.features import TurnFeatures, session_features

PROFILE_VERSION = "articulation-profile-v1"

DIMENSIONS = (
    "structure",
    "conciseness",
    "fluency",
    "pace",
    "pausing",
    "precision",
    "signposting",
    "intelligibility",
    "vocal_delivery",
    "responsiveness",
)

SIGNPOSTS = (
    "first",
    "second",
    "third",
    "finally",
    "next",
    "then",
    "because",
    "so that",
    "as a result",
    "for example",
    "in short",
    "to summarise",
    "to summarize",
    "the key point",
    "the outcome",
)
"""Words that tell a listener where they are. Counted, not judged."""

_SENTENCE = re.compile(r"[^.!?]+[.!?]?")
_NUMBER = re.compile(r"\d")
_TOKEN = re.compile(r"[a-z0-9']+")


@dataclass(frozen=True)
class Dimension:
    """One dimension's level and where it came from."""

    level: str
    evidence_sequences: tuple[int, ...]
    reason: str


@dataclass(frozen=True)
class Profile:
    """All ten dimensions. Deliberately no total: there is none."""

    profile_version: str
    dimensions: dict[str, Dimension] = field(default_factory=dict)


def _level(value: float, solid_at: float, strong_at: float, higher_is_better: bool = True) -> str:
    if not higher_is_better:
        value = -value
        solid_at, strong_at = -solid_at, -strong_at
    if value >= strong_at:
        return "strong"
    if value >= solid_at:
        return "solid"
    return "developing"


def _not_assessable(reason: str) -> Dimension:
    return Dimension(level="not_assessable", evidence_sequences=(), reason=reason)


def profile(turns: list[dict[str, Any]]) -> Profile:
    """Derive the profile from the turns. Every level names its turns."""
    features = session_features(turns)
    assessable = [t for t in features.turns if t.status == "assessable"]
    by_sequence = {int(t["sequence"]): t for t in turns}
    questions = [t for t in turns if t.get("speaker") == "interviewer"]
    dims: dict[str, Dimension] = {}

    if not assessable:
        for name in DIMENSIONS:
            dims[name] = _not_assessable("no assessable candidate speech")
        return Profile(profile_version=PROFILE_VERSION, dimensions=dims)

    sequences = tuple(t.sequence for t in assessable)
    total_words = sum(t.words for t in assessable)

    # Pace: 120 to 180 words per minute reads as conversational; outside
    # that in either direction is developing, without saying which way is
    # "correct" - the reason does.
    wpm = features.words_per_minute
    pace_level = "strong" if 130 <= wpm <= 170 else ("solid" if 110 <= wpm <= 190 else "developing")
    dims["pace"] = Dimension(pace_level, sequences, f"{wpm} words per minute over assessable turns")

    # Pausing: long pauses per minute of speech.
    minutes = sum(t.duration_ms for t in assessable) / 60_000 or 1
    long_per_minute = features.long_pause_count / minutes
    dims["pausing"] = Dimension(
        _level(long_per_minute, solid_at=2.0, strong_at=1.0, higher_is_better=False),
        sequences,
        f"{features.long_pause_count} pauses of 700 ms or more in {minutes:.1f} minutes",
    )

    # Fluency: fillers per hundred words plus restarts per hundred words.
    disfluency = features.fillers_per_100_words + 100 * sum(
        t.restart_count for t in assessable
    ) / max(total_words, 1)
    dims["fluency"] = Dimension(
        _level(disfluency, solid_at=6.0, strong_at=3.0, higher_is_better=False),
        sequences,
        f"{features.fillers_per_100_words} fillers per hundred words; "
        f"fillers and restarts combined {disfluency:.1f}",
    )

    # Conciseness: repeated phrases per hundred words, and answer length.
    repeats = sum(t.repeated_phrase_count for t in assessable)
    repeat_rate = 100 * repeats / max(total_words, 1)
    dims["conciseness"] = Dimension(
        _level(repeat_rate, solid_at=1.5, strong_at=0.5, higher_is_better=False),
        sequences,
        f"{repeats} repeated phrases across {total_words} words",
    )

    # Signposting and structure from the candidate's own words.
    texts = {t.sequence: str(by_sequence[t.sequence].get("text", "")).lower() for t in assessable}
    signposted = tuple(seq for seq, text in texts.items() if any(s in text for s in SIGNPOSTS))
    share = len(signposted) / len(assessable)
    dims["signposting"] = Dimension(
        _level(share, solid_at=0.34, strong_at=0.67),
        signposted or sequences,
        f"{len(signposted)} of {len(assessable)} answers used a signposting phrase",
    )
    # Structure: an answer with an outcome sentence (a number) after a claim.
    structured = tuple(
        seq
        for seq, text in texts.items()
        if len([s for s in _SENTENCE.findall(text) if s.strip()]) >= 2 and _NUMBER.search(text)
    )
    dims["structure"] = Dimension(
        _level(len(structured) / len(assessable), solid_at=0.34, strong_at=0.67),
        structured or sequences,
        f"{len(structured)} of {len(assessable)} answers moved from claim to a stated outcome",
    )
    # Precision: answers carrying a concrete figure.
    precise = tuple(seq for seq, text in texts.items() if _NUMBER.search(text))
    dims["precision"] = Dimension(
        _level(len(precise) / len(assessable), solid_at=0.34, strong_at=0.67),
        precise or sequences,
        f"{len(precise)} of {len(assessable)} answers carried a concrete figure",
    )

    # Responsiveness: overlap between the question's content words and the
    # answer's, per adjacent pair.
    responsive: list[int] = []
    for turn in assessable:
        earlier = [q for q in questions if int(q["sequence"]) < turn.sequence]
        if not earlier:
            continue
        question_tokens = {
            w for w in _TOKEN.findall(str(earlier[-1].get("text", "")).lower()) if len(w) > 3
        }
        answer_tokens = set(_TOKEN.findall(texts[turn.sequence]))
        if question_tokens and question_tokens & answer_tokens:
            responsive.append(turn.sequence)
    if questions:
        dims["responsiveness"] = Dimension(
            _level(len(responsive) / len(assessable), solid_at=0.5, strong_at=0.8),
            tuple(responsive) or sequences,
            f"{len(responsive)} of {len(assessable)} answers echoed the question's own terms",
        )
    else:
        dims["responsiveness"] = _not_assessable("no interviewer questions to answer")

    # Intelligibility as followability: confidence and sentence length,
    # nothing about how the words sounded.
    confidence = features.transcript_confidence
    long_sentences = sum(
        1
        for text in texts.values()
        for sentence in _SENTENCE.findall(text)
        if len(_TOKEN.findall(sentence)) > 35
    )
    if confidence >= 0.85 and long_sentences == 0:
        level = "strong"
    elif confidence >= 0.7 and long_sentences <= 2:
        level = "solid"
    else:
        level = "developing"
    dims["intelligibility"] = Dimension(
        level,
        sequences,
        f"transcript confidence {confidence} with {long_sentences} sentences over 35 words",
    )

    # Vocal delivery needs the audio itself; until it is decoded here the
    # dimension says so.
    dims["vocal_delivery"] = _not_assessable("audio not decoded at this floor")

    return Profile(profile_version=PROFILE_VERSION, dimensions=dims)


def profile_document(turns: list[dict[str, Any]]) -> dict[str, Any]:
    """The profile as the analysis document carries it."""
    result = profile(turns)
    return {
        "profile_version": result.profile_version,
        "dimensions": {
            name: {
                "level": dim.level,
                "evidence_sequences": list(dim.evidence_sequences),
                "reason": dim.reason,
            }
            for name, dim in result.dimensions.items()
        },
    }


__all__ = [
    "DIMENSIONS",
    "PROFILE_VERSION",
    "Dimension",
    "Profile",
    "TurnFeatures",
    "profile",
    "profile_document",
]
