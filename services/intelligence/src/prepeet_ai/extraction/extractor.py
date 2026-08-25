"""CV extraction: structured facts with the span that produced each one.

extract-1 is deliberate about what it is: a deterministic, rule-based reading
of plain text. It finds date ranges, role headings near them, and skill lists,
and it records everything it could NOT parse as unparsed spans rather than
dropping it - presenting a partial reading as a complete one is the failure
PRO-03's second criterion names. Confidences are honest about the method:
rule matches score what the rule deserves, never what a model would claim.

PDF and Word documents are refused as unsupported rather than half-read: the
caller surfaces that state and the profile continues manually, which is the
degradation the journey is designed around. Text extraction from rich
formats arrives as a later extractor version with its own dependencies.

Every fact's span is a half-open byte range into the exact text fetched, so
the inspection screen can highlight precisely what produced each fact.
"""

from __future__ import annotations

import re
from dataclasses import dataclass

EXTRACTOR_VERSION = "extract-1"
"""Recorded on every fact, so a reading can be reproduced or superseded."""

SUPPORTED_MEDIA_TYPES = frozenset({"text/plain"})
"""What extract-1 can honestly read. Anything else is unsupported, stated."""


@dataclass(frozen=True, slots=True)
class Fact:
    """One extracted fact, span-linked to its source."""

    kind: str
    value: dict[str, str]
    span_start: int
    span_end: int
    confidence: float


# A year range like "2019 - 2023", "2019 to present", dash variants included.
_DATE_RANGE = re.compile(
    r"(?P<start>(?:[A-Z][a-z]{2,8}\s+)?(?:19|20)\d{2})\s*(?:-|\u2013|\u2014|to)\s*"
    r"(?P<end>(?:[A-Z][a-z]{2,8}\s+)?(?:19|20)\d{2}|[Pp]resent|[Nn]ow)"
)

# A skills heading, then the list that follows on its line or the next.
_SKILLS_HEADING = re.compile(r"^(?:key\s+)?(?:technical\s+)?skills?\s*[:\-]?\s*$", re.I)

_SKILL_SPLIT = re.compile(r"[,;•|/]| and ")

# An achievement line: leads with a strong verb, carries a number.
_ACHIEVEMENT = re.compile(
    r"^(?:[-*•]\s*)?(?P<line>(?:led|built|delivered|reduced|increased|launched|migrated|grew|saved|shipped)\b[^\n]*\d[^\n]*)$",
    re.I | re.M,
)


def extract(text: str) -> list[Fact]:
    """Read the text into facts, recording what resisted reading.

    Deterministic: the same text yields the same facts in the same order,
    which is what lets a retried extraction converge instead of duplicating.
    """
    facts: list[Fact] = []
    claimed: list[tuple[int, int]] = []

    def claim(start: int, end: int) -> None:
        claimed.append((start, end))

    # Date ranges, and the line above each as a role heading candidate.
    for match in _DATE_RANGE.finditer(text):
        facts.append(
            Fact(
                kind="date_range",
                value={"start": match.group("start"), "end": match.group("end")},
                span_start=match.start(),
                span_end=match.end(),
                confidence=0.9,
            )
        )
        claim(match.start(), match.end())

        # The role heading: the tail of the date's own line if the date sits
        # inline ("Engineer, 2019-2023"), else the whole previous line.
        line_start = text.rfind("\n", 0, match.start()) + 1
        heading_end = match.start()
        heading = text[line_start:heading_end].strip(" \t-\u2013\u2014|,")
        if not heading and line_start >= 2:
            heading_end = line_start - 1
            line_start = text.rfind("\n", 0, heading_end) + 1
            heading = text[line_start:heading_end].strip(" \t-\u2013\u2014|,")
        if heading and len(heading) <= 120:
            facts.append(
                Fact(
                    kind="role",
                    value={"title": heading},
                    span_start=line_start,
                    span_end=line_start + len(text[line_start:heading_end].rstrip()),
                    confidence=0.6,
                )
            )
            claim(line_start, heading_end)

    # Skills: the line after a skills heading, split on list punctuation.
    lines = text.split("\n")
    offset = 0
    for index, line in enumerate(lines):
        if _SKILLS_HEADING.match(line.strip()) and index + 1 < len(lines):
            skills_line = lines[index + 1]
            skills_offset = offset + len(line) + 1
            cursor = 0
            for token in _SKILL_SPLIT.split(skills_line):
                position = skills_line.find(token, cursor)
                cursor = position + len(token)
                cleaned = token.strip(" \t.")
                if cleaned and len(cleaned) <= 60:
                    facts.append(
                        Fact(
                            kind="skill",
                            value={"name": cleaned},
                            span_start=skills_offset + position + token.index(cleaned[0]),
                            span_end=skills_offset
                            + position
                            + token.index(cleaned[0])
                            + len(cleaned),
                            confidence=0.8,
                        )
                    )
            claim(skills_offset, skills_offset + len(skills_line))
        offset += len(line) + 1

    for match in _ACHIEVEMENT.finditer(text):
        facts.append(
            Fact(
                kind="achievement",
                value={"text": match.group("line").strip()},
                span_start=match.start("line"),
                span_end=match.end("line"),
                confidence=0.5,
            )
        )
        claim(match.start(), match.end())

    facts.extend(_unparsed(text, claimed))
    facts.sort(key=lambda fact: (fact.span_start, fact.span_end, fact.kind))
    return facts


def _unparsed(text: str, claimed: list[tuple[int, int]]) -> list[Fact]:
    """The honesty pass: substantial spans nothing claimed become facts too.

    A span is worth surfacing when it carries real words - short connective
    lines and blank runs are not a failure to parse anything.
    """
    claimed.sort()
    merged: list[tuple[int, int]] = []
    for start, end in claimed:
        if merged and start <= merged[-1][1]:
            merged[-1] = (merged[-1][0], max(end, merged[-1][1]))
        else:
            merged.append((start, end))

    unparsed: list[Fact] = []
    cursor = 0
    for start, end in [*merged, (len(text), len(text))]:
        gap = text[cursor:start]
        stripped = gap.strip()
        if len(stripped) >= 40:
            lead = cursor + gap.index(stripped[0])
            unparsed.append(
                Fact(
                    kind="unparsed",
                    value={"text": stripped[:500]},
                    span_start=lead,
                    span_end=lead + len(stripped),
                    confidence=1.0,
                )
            )
        cursor = max(cursor, end)
    return unparsed
