"""Delivery coaching: articulation-coaching-v1 (ART-04).

One or two priorities per session, each stating the listener impact and
one action, chosen from the profile's weakest measurable dimensions; a
selected drill from the spec's list; and a suggested shape assembled ONLY
from the candidate's own sentences, with bracketed questions where a
slot has nothing to fill it. The vocabulary cannot invent a fact because
it has none: every quote part is a sentence the candidate said, verified
by preserve() before the document is served, and a violation refuses the
whole coaching rather than one line of it.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any

from prepeet_ai.articulation.profile import Profile, profile

COACHING_VERSION = "articulation-coaching-v1"

_SENTENCE = re.compile(r"[^.!?]+[.!?]?")
_NUMBER = re.compile(r"\d")

LEVEL_ORDER = {"developing": 0, "solid": 1, "strong": 2}
"""Lower first: the priorities are the dimensions with most room."""

# Listener impact and one action per dimension. Impact is what the
# listener experiences; the action is one thing to do next time. Neither
# states anything about the candidate's ability.
PRIORITY_COPY: dict[str, tuple[str, str, str]] = {
    "structure": (
        "The listener hears context before they know what the answer is about.",
        "Say the decision or result in your first sentence, then explain.",
        "headline_first",
    ),
    "conciseness": (
        "Repeated phrases make the listener wait for new information.",
        "Say each point once; if you feel the urge to restate, stop and move to the outcome.",
        "sixty_second_compression",
    ),
    "fluency": (
        "Fillers and restarts pull the listener's attention from the content.",
        "Pause silently where you would say a filler; the pause reads as thought.",
        "deliberate_pause",
    ),
    "pace": (
        "A rate far from conversational makes the listener work to keep up or wait.",
        "Aim for a pace that lets you finish each sentence cleanly; "
        "the number is guidance, not a rule.",
        "playback_and_redo",
    ),
    "pausing": (
        "Long hesitations leave the listener unsure whether the answer is over.",
        "Decide your first sentence before you start speaking, then the pauses shorten.",
        "headline_first",
    ),
    "precision": (
        "Without a figure, the listener cannot tell how big the result was.",
        "Attach one number to the outcome: a percentage, a count, a time saved.",
        "concrete_language",
    ),
    "signposting": (
        "Without markers, the listener cannot tell where the answer is going.",
        "Use one marker per move: first, then, as a result.",
        "signposting",
    ),
    "intelligibility": (
        "Long unbroken sentences are hard to follow in speech.",
        "End a sentence every fifteen to twenty words.",
        "star_compression",
    ),
    "responsiveness": (
        "An answer that does not echo the question leaves the listener matching it up themselves.",
        "Repeat the question's key term in your first sentence.",
        "headline_first",
    ),
}

ASK_HEADLINE = "[What is the one-sentence answer? Lead with it.]"
ASK_CONTEXT = "[What was the situation, in one sentence?]"
ASK_REASONING = "[Why did you choose that, over what alternative?]"
ASK_RESULT = "[What changed as a result? Give the number or the outcome.]"


@dataclass(frozen=True)
class Priority:
    """One thing to work on: impact, action, evidence, drill."""

    dimension: str
    level: str
    listener_impact: str
    action: str
    evidence_sequences: tuple[int, ...]
    drill: str


@dataclass(frozen=True)
class ShapePart:
    """One slot of the suggested shape: the candidate's words, or a question."""

    slot: str
    kind: str  # quote | placeholder
    text: str
    sequence: int | None = None


@dataclass(frozen=True)
class DeliveryCoaching:
    """The session's delivery coaching."""

    coaching_version: str
    priorities: tuple[Priority, ...]
    suggested_shape: tuple[ShapePart, ...]


def _sentences(text: str) -> list[str]:
    return [s.strip() for s in _SENTENCE.findall(text) if s.strip()]


def suggested_shape(turns: list[dict[str, Any]], sequence: int | None) -> tuple[ShapePart, ...]:
    """Headline, context, reasoning, result: from one answer's own sentences.

    The result slot takes the first sentence with a figure; the headline
    the first sentence; context the second; reasoning the first sentence
    containing 'because' or 'so'. A slot with no sentence to fill it is a
    question, never an invented line.
    """
    turn = next((t for t in turns if int(t["sequence"]) == sequence), None)
    sentences = _sentences(str(turn.get("text", ""))) if turn else []

    def quote(text: str) -> ShapePart:
        return ShapePart(slot="", kind="quote", text=text, sequence=sequence)

    used: set[str] = set()
    parts: list[ShapePart] = []
    result = next((s for s in sentences if _NUMBER.search(s)), None)
    headline = sentences[0] if sentences else None
    context = next((s for s in sentences[1:] if s != result), None)
    reasoning = next(
        (
            s
            for s in sentences
            if s not in (headline, context, result) and re.search(r"\b(because|so)\b", s.lower())
        ),
        None,
    )
    for slot, chosen, ask in (
        ("headline", headline, ASK_HEADLINE),
        ("context", context, ASK_CONTEXT),
        ("reasoning", reasoning, ASK_REASONING),
        ("result", result, ASK_RESULT),
    ):
        if chosen and chosen not in used:
            used.add(chosen)
            parts.append(ShapePart(slot=slot, kind="quote", text=chosen, sequence=sequence))
        else:
            parts.append(ShapePart(slot=slot, kind="placeholder", text=ask))
    return tuple(parts)


def coach(turns: list[dict[str, Any]], measured: Profile | None = None) -> DeliveryCoaching:
    """Derive the coaching from the profile and the turns. Empty when nothing is measurable."""
    result = measured or profile(turns)
    measurable = [
        (name, dim)
        for name, dim in result.dimensions.items()
        if dim.level in LEVEL_ORDER and name in PRIORITY_COPY
    ]
    measurable.sort(key=lambda item: (LEVEL_ORDER[item[1].level], item[0]))
    priorities: list[Priority] = []
    for name, dim in measurable:
        if dim.level == "strong" or len(priorities) == 2:
            break
        impact, action, drill = PRIORITY_COPY[name]
        priorities.append(
            Priority(
                dimension=name,
                level=dim.level,
                listener_impact=impact,
                action=action,
                evidence_sequences=dim.evidence_sequences,
                drill=drill,
            )
        )
    anchor = (
        priorities[0].evidence_sequences[0]
        if priorities and priorities[0].evidence_sequences
        else None
    )
    shape = suggested_shape(turns, anchor) if anchor is not None else ()
    return DeliveryCoaching(
        coaching_version=COACHING_VERSION,
        priorities=tuple(priorities),
        suggested_shape=shape,
    )


class UnpreservingError(ValueError):
    """The coaching contains words the candidate did not say, or a fact in brackets."""


def preserve(turns: list[dict[str, Any]], coaching: DeliveryCoaching) -> DeliveryCoaching:
    """The fact-preservation gate. Returns the coaching or refuses it whole.

    Every quote part must be an exact substring of its own candidate turn;
    every placeholder must be a bracketed question with no digit in it (a
    number in brackets is a fact wearing them); every priority's evidence
    must name candidate turns that exist.
    """
    candidate = {
        int(t["sequence"]): str(t.get("text", "")) for t in turns if t.get("speaker") == "candidate"
    }
    for part in coaching.suggested_shape:
        if part.kind == "quote":
            if part.sequence not in candidate or part.text not in candidate[part.sequence]:
                raise UnpreservingError(
                    f"the {part.slot} slot quotes words the candidate did not say"
                )
        elif part.kind == "placeholder":
            if not (part.text.startswith("[") and part.text.endswith("]") and "?" in part.text):
                raise UnpreservingError(f"the {part.slot} placeholder is not a bracketed question")
            if _NUMBER.search(part.text):
                raise UnpreservingError(f"the {part.slot} placeholder carries a figure")
        else:
            raise UnpreservingError(f"unknown shape part kind {part.kind!r}")
    for priority in coaching.priorities:
        if any(seq not in candidate for seq in priority.evidence_sequences):
            raise UnpreservingError(
                f"{priority.dimension} cites a turn that is not the candidate's"
            )
    return coaching


def coaching_document(
    turns: list[dict[str, Any]], measured: Profile | None = None
) -> dict[str, Any]:
    """The coaching as the analysis document carries it, gated first."""
    coaching = preserve(turns, coach(turns, measured))
    return {
        "coaching_version": coaching.coaching_version,
        "priorities": [
            {
                "dimension": p.dimension,
                "level": p.level,
                "listener_impact": p.listener_impact,
                "action": p.action,
                "evidence_sequences": list(p.evidence_sequences),
                "drill": p.drill,
            }
            for p in coaching.priorities
        ],
        "suggested_shape": [
            {"slot": s.slot, "kind": s.kind, "text": s.text, "sequence": s.sequence}
            for s in coaching.suggested_shape
        ],
    }
