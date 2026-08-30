"""The graders: grounding, unsupported facts and schema conformance.

Every measure here is a string comparison against the sealed transcript,
never a judgement about it. That is the whole point of QUA-02's second
criterion: an unsupported fact is one whose quoted words are not in the
turn it names, which is decidable, rather than one a reviewer felt uneasy
about, which is not.

A grader that can only ever return zero measures nothing, so each of these
is exercised in the suite against a deliberately fabricated record as well
as against the fixtures.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any

EVIDENCE_KINDS = frozenset(
    {"supporting", "contradictory", "claim_unverified", "gap", "delivery_observation"}
)
"""The evidence types evaluation-system.md names. A kind outside this set is
a schema failure, not a new category somebody may introduce quietly."""

_DIGIT = re.compile(r"\d")


@dataclass(frozen=True)
class Assertion:
    """One thing the pipeline said, and whether the transcript supports it.

    `supported` is measured by exact string containment, so this record is
    reproducible from the same inputs by anyone who doubts it.
    """

    kind: str
    sequence: int | None
    text: str
    supported: bool
    reason: str


def _candidate_turns(turns: list[dict[str, Any]]) -> dict[int, str]:
    """The candidate's turns by sequence. Only these can support anything."""
    return {
        int(turn["sequence"]): str(turn.get("text", ""))
        for turn in turns
        if turn.get("speaker") == "candidate"
    }


def grounding_problems(turns: list[dict[str, Any]], span: dict[str, Any]) -> list[str]:
    """Everything wrong with one span's link back to the transcript.

    A span is grounded when it names a candidate turn that exists, its
    character range slices exactly the quote it carries, and its clock
    range sits inside that turn. This is the same reading Go's validator
    holds the extractor to; measuring it here catches the regression
    before the control plane has to refuse a whole batch.
    """
    problems: list[str] = []
    by_sequence = {int(turn["sequence"]): turn for turn in turns}
    sequence = int(span["segment_sequence"])
    turn = by_sequence.get(sequence)
    if turn is None:
        return [f"segment {sequence} is not in the transcript"]
    if turn.get("speaker") != "candidate":
        problems.append(f"segment {sequence} is not the candidate's turn")
    text = str(turn["text"])
    start, end = int(span["char_start"]), int(span["char_end"])
    if not 0 <= start < end <= len(text):
        problems.append(
            f"character range {start}:{end} is outside a turn of {len(text)} characters"
        )
    elif text[start:end] != span["quote"]:
        problems.append("the quote is not the text at its own character range")
    start_ms, end_ms = int(span["start_ms"]), int(span["end_ms"])
    if not int(turn["start_ms"]) <= start_ms <= end_ms <= int(turn["end_ms"]):
        problems.append(
            f"clock range {start_ms}:{end_ms} is outside the turn's "
            f"{turn['start_ms']}:{turn['end_ms']}"
        )
    return problems


def evidence_span_schema_problems(record: dict[str, Any]) -> list[str]:
    """Everything about one span record that is not the declared shape."""
    problems: list[str] = []
    for field in (
        "competency_id",
        "kind",
        "segment_sequence",
        "quote",
        "char_start",
        "char_end",
        "start_ms",
        "end_ms",
        "extraction_version",
    ):
        if field not in record:
            problems.append(f"missing {field}")
    if problems:
        return problems
    if not isinstance(record["competency_id"], str) or not record["competency_id"]:
        problems.append("competency_id is not a non-empty string")
    if record["kind"] not in EVIDENCE_KINDS:
        problems.append(f"kind {record['kind']!r} is not one of the declared evidence kinds")
    if not isinstance(record["quote"], str) or not record["quote"].strip():
        problems.append("quote is empty, which is an assertion about nothing")
    for field in ("segment_sequence", "char_start", "char_end", "start_ms", "end_ms"):
        if not isinstance(record[field], int) or isinstance(record[field], bool):
            problems.append(f"{field} is not an integer")
    if not isinstance(record["extraction_version"], str) or not record["extraction_version"]:
        problems.append("extraction_version is empty, so the reading cannot be reproduced")
    return problems


def contradiction_schema_problems(record: dict[str, Any]) -> list[str]:
    """Everything about one contradiction record that is not the declared shape."""
    problems: list[str] = []
    topic = record.get("topic")
    if not isinstance(topic, list) or not topic:
        problems.append("topic is empty, so nobody can see why the pair was made")
    elif not all(isinstance(token, str) and token for token in topic):
        problems.append("topic contains something that is not a word")
    for side in ("side_a", "side_b"):
        body = record.get(side)
        if not isinstance(body, dict):
            problems.append(f"{side} is missing")
            continue
        for field in ("segment_sequence", "quote", "char_start", "char_end", "start_ms", "end_ms"):
            if field not in body:
                problems.append(f"{side} is missing {field}")
    if not record.get("extraction_version"):
        problems.append("extraction_version is empty, so the reading cannot be reproduced")
    return problems


def _quote_assertion(
    kind: str,
    candidate: dict[int, str],
    sequence: int,
    quote: str,
    exact_range: tuple[int, int] | None,
) -> Assertion:
    """One quoted assertion, checked against the words that were said."""
    text = candidate.get(sequence)
    if text is None:
        return Assertion(
            kind, sequence, quote, False, f"segment {sequence} is not a candidate turn"
        )
    if exact_range is not None:
        start, end = exact_range
        if text[start:end] != quote:
            return Assertion(
                kind, sequence, quote, False, "the quote is not the text at its own character range"
            )
        return Assertion(kind, sequence, quote, True, "")
    if quote not in text:
        return Assertion(kind, sequence, quote, False, "the quote is not in the turn it names")
    return Assertion(kind, sequence, quote, True, "")


def assertions(
    turns: list[dict[str, Any]],
    spans: list[dict[str, Any]],
    contradictions: list[dict[str, Any]],
    coaching: dict[str, Any] | None,
) -> list[Assertion]:
    """Every factual assertion the pipeline made about this transcript.

    The set is deliberately wide: a span, either side of a contradiction,
    and every part of the suggested shape. Coaching is where invented prose
    would appear if it ever appeared, so a rate computed without it would
    be measuring the safest surface and calling it the whole product.
    """
    candidate = _candidate_turns(turns)
    found: list[Assertion] = []

    for span in spans:
        found.append(
            _quote_assertion(
                "evidence_quote",
                candidate,
                int(span["segment_sequence"]),
                str(span["quote"]),
                (int(span["char_start"]), int(span["char_end"])),
            )
        )

    for pair in contradictions:
        for side in ("side_a", "side_b"):
            body = pair[side]
            found.append(
                _quote_assertion(
                    "contradiction_quote",
                    candidate,
                    int(body["segment_sequence"]),
                    str(body["quote"]),
                    (int(body["char_start"]), int(body["char_end"])),
                )
            )

    for part in (coaching or {}).get("suggested_shape", []):
        text = str(part.get("text", ""))
        if part.get("kind") == "quote":
            sequence = part.get("sequence")
            found.append(
                _quote_assertion(
                    "coaching_quote",
                    candidate,
                    int(sequence) if sequence is not None else -1,
                    text,
                    None,
                )
            )
        else:
            # A placeholder is allowed to be the only prose the pipeline
            # writes, so it is held to the narrow rule the coaching gate
            # uses: a bracketed question, and no figure inside the
            # brackets. A number in a placeholder is a fact wearing them.
            bracketed = text.startswith("[") and text.endswith("]") and "?" in text
            if not bracketed:
                found.append(
                    Assertion("coaching_placeholder", None, text, False, "not a bracketed question")
                )
            elif _DIGIT.search(text):
                found.append(
                    Assertion(
                        "coaching_placeholder", None, text, False, "carries a figure in brackets"
                    )
                )
            else:
                found.append(Assertion("coaching_placeholder", None, text, True, ""))

    return found
