"""Evidence extraction: evidence-1, the deterministic floor.

Turns a sealed conversation into spans linked to competencies, with the
properties EVL-01 stakes everything on: every span is an exact substring of
a real turn, its clock range sits inside that turn (tightened to the quoted
sentence's own words when word timing exists), and absence of evidence is
absence - silence about a competency yields nothing, never a low-value
span. The rules are keyword-and-shape based so the reading is reproducible;
a model-backed extractor replaces this behind the same contract, and Go's
validator holds either one to the same honesty.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any

EXTRACTION_VERSION = "evidence-1"

_SENTENCE = re.compile(r"[^.!?]+[.!?]?")
_MEASURED = re.compile(r"\d")
_UNCERTAIN = re.compile(
    r"\b(not sure|don't know|do not know|never (?:done|had)|no experience)\b", re.IGNORECASE
)


@dataclass(frozen=True)
class EvidenceSpan:
    """One span of what was actually said, competency-linked."""

    competency_id: str
    kind: str
    segment_sequence: int
    quote: str
    char_start: int
    char_end: int
    start_ms: int
    end_ms: int
    extraction_version: str


def _tokens(name: str) -> list[str]:
    """The competency name's distinctive tokens, lowercased."""
    return [token for token in re.split(r"[^a-z0-9]+", name.lower()) if len(token) > 3]


def _sentence_clock(turn: dict[str, Any], char_start: int, char_end: int) -> tuple[int, int]:
    """The sentence's clock range, from word timing when it exists.

    Words carry no character offsets, so the mapping counts whitespace
    tokens before the sentence to find its first word. Approximate by
    construction and bounded by the turn either way.
    """
    start_ms, end_ms = int(turn["start_ms"]), int(turn["end_ms"])
    words = turn.get("words") or []
    if not words:
        return start_ms, end_ms

    text = turn["text"]
    first = len(text[:char_start].split())
    count = len(text[char_start:char_end].split())
    last = min(first + count - 1, len(words) - 1)
    if first >= len(words) or count == 0:
        return start_ms, end_ms
    return int(words[first]["start_ms"]), int(words[last]["end_ms"])


def extract_evidence(
    turns: list[dict[str, Any]], competencies: list[dict[str, Any]]
) -> list[EvidenceSpan]:
    """Read the turns into spans, deterministically."""
    spans: list[EvidenceSpan] = []

    for turn in turns:
        if turn.get("speaker") != "candidate":
            continue
        text: str = turn["text"]
        sequence = int(turn["sequence"])

        sentences: list[tuple[int, int, str]] = []
        for match in _SENTENCE.finditer(text):
            raw = match.group(0)
            stripped = raw.strip()
            if not stripped:
                continue
            begin = match.start() + (len(raw) - len(raw.lstrip()))
            sentences.append((begin, begin + len(stripped), stripped))

        for position, (char_start, char_end, stripped) in enumerate(sentences):
            lowered = stripped.lower()

            for competency in competencies:
                if not any(token in lowered for token in _tokens(competency["name"])):
                    continue

                quote_end = char_end
                if _UNCERTAIN.search(stripped):
                    kind = "gap"
                elif _MEASURED.search(stripped):
                    kind = "supporting"
                elif (
                    position + 1 < len(sentences)
                    and _MEASURED.search(sentences[position + 1][2])
                    and not _UNCERTAIN.search(sentences[position + 1][2])
                ):
                    # People state the act and then the number: "I rebuilt
                    # the pipeline. Latency dropped 40 percent." The outcome
                    # sentence attributes to the claim beside it, and the
                    # span honestly covers both.
                    kind = "supporting"
                    quote_end = sentences[position + 1][1]
                else:
                    kind = "claim_unverified"

                quote = text[char_start:quote_end]
                start_ms, end_ms = _sentence_clock(turn, char_start, quote_end)
                spans.append(
                    EvidenceSpan(
                        competency_id=competency["id"],
                        kind=kind,
                        segment_sequence=sequence,
                        quote=quote,
                        char_start=char_start,
                        char_end=quote_end,
                        start_ms=start_ms,
                        end_ms=end_ms,
                        extraction_version=EXTRACTION_VERSION,
                    )
                )

    spans.sort(
        key=lambda span: (span.segment_sequence, span.char_start, span.competency_id, span.kind)
    )
    return spans


@dataclass(frozen=True)
class ContradictionSide:
    """One of the two statements, quoted exactly on the room clock."""

    segment_sequence: int
    quote: str
    char_start: int
    char_end: int
    start_ms: int
    end_ms: int


@dataclass(frozen=True)
class Contradiction:
    """Two candidate statements about one subject whose numbers conflict.

    The vocabulary is deliberately descriptive: a contradiction is a
    prompt for clarification, and nothing here infers anything about the
    person. Topic tokens name what the statements share, so a reviewer
    can see WHY the pair was made.
    """

    topic: tuple[str, ...]
    side_a: ContradictionSide
    side_b: ContradictionSide
    extraction_version: str


_NUMBER = re.compile(r"\d+(?:\.\d+)?")

_MIN_SHARED_TOKENS = 2
"""Two shared subject tokens before numbers are comparable at all: one
shared word pairs unrelated measurements and turns coincidence into a
question the candidate never earned."""


def _candidate_sentences(turns: list[dict[str, Any]]) -> list[tuple[dict[str, Any], int, int, str]]:
    """Every candidate sentence with its turn and character range."""
    sentences: list[tuple[dict[str, Any], int, int, str]] = []
    for turn in turns:
        if turn.get("speaker") != "candidate":
            continue
        text: str = turn["text"]
        for match in _SENTENCE.finditer(text):
            raw = match.group(0)
            stripped = raw.strip()
            if not stripped:
                continue
            begin = match.start() + (len(raw) - len(raw.lstrip()))
            sentences.append((turn, begin, begin + len(stripped), stripped))
    return sentences


def extract_contradictions(turns: list[dict[str, Any]]) -> list[Contradiction]:
    """Pair candidate statements whose numbers disagree, deterministically.

    The rule is the floor's: two sentences must share at least two
    significant subject tokens, both must state a number, and their
    numbers must have nothing in common. A restated number is
    consistency; unrelated measurements never meet the shared-token bar.
    """
    sentences = _candidate_sentences(turns)
    pairs: list[Contradiction] = []

    for i, (turn_a, start_a, end_a, text_a) in enumerate(sentences):
        numbers_a = set(_NUMBER.findall(text_a))
        if not numbers_a:
            continue
        tokens_a = set(_tokens(text_a))
        for turn_b, start_b, end_b, text_b in sentences[i + 1 :]:
            numbers_b = set(_NUMBER.findall(text_b))
            if not numbers_b or numbers_a & numbers_b:
                continue
            shared = tokens_a & set(_tokens(text_b))
            if len(shared) < _MIN_SHARED_TOKENS:
                continue
            side_a = ContradictionSide(
                segment_sequence=int(turn_a["sequence"]),
                quote=turn_a["text"][start_a:end_a],
                char_start=start_a,
                char_end=end_a,
                start_ms=_sentence_clock(turn_a, start_a, end_a)[0],
                end_ms=_sentence_clock(turn_a, start_a, end_a)[1],
            )
            side_b = ContradictionSide(
                segment_sequence=int(turn_b["sequence"]),
                quote=turn_b["text"][start_b:end_b],
                char_start=start_b,
                char_end=end_b,
                start_ms=_sentence_clock(turn_b, start_b, end_b)[0],
                end_ms=_sentence_clock(turn_b, start_b, end_b)[1],
            )
            pairs.append(
                Contradiction(
                    topic=tuple(sorted(shared)),
                    side_a=side_a,
                    side_b=side_b,
                    extraction_version=EXTRACTION_VERSION,
                )
            )

    pairs.sort(
        key=lambda pair: (
            pair.side_a.segment_sequence,
            pair.side_a.char_start,
            pair.side_b.segment_sequence,
            pair.side_b.char_start,
        )
    )
    return pairs
