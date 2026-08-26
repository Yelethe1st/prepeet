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
