"""ART-04's fact-preservation suite, run in CI with everything else.

The suggested shape is the candidate's own sentences and bracketed
questions, nothing else; a coaching that says otherwise is refused whole;
and the priorities state listener impact and one action from copy that
never describes the person.
"""

from __future__ import annotations

import dataclasses
import json
import pathlib

import pytest

from prepeet_ai.articulation.coaching import (
    COACHING_VERSION,
    PRIORITY_COPY,
    DeliveryCoaching,
    ShapePart,
    UnpreservingError,
    coach,
    coaching_document,
    preserve,
)

FIXTURE = json.loads(
    (pathlib.Path(__file__).parent / "fixtures/articulation_known.json").read_text()
)
TURNS = FIXTURE["turns"]


def _candidate_text(sequence: int) -> str:
    return next(t["text"] for t in TURNS if t["sequence"] == sequence)


class TestTheShapeIsTheCandidatesOwnWords:
    """The first two boxes, on the honest output."""

    def test_every_quote_is_a_sentence_the_candidate_said(self) -> None:
        """Word for word, from the turn the shape names."""
        coaching = coach(TURNS)
        assert coaching.suggested_shape, "an assessable session earns a shape"
        for part in coaching.suggested_shape:
            if part.kind == "quote":
                assert part.sequence is not None
                assert part.text in _candidate_text(part.sequence)

    def test_missing_slots_are_questions_never_details(self) -> None:
        """A slot with nothing to fill it asks; it never states."""
        thin = [
            {
                "sequence": 2,
                "speaker": "interviewer",
                "text": "Tell me.",
                "start_ms": 0,
                "end_ms": 500,
            },
            {
                "sequence": 3,
                "speaker": "candidate",
                "text": " ".join(f"word{i}" for i in range(24)) + ".",
                "start_ms": 1000,
                "end_ms": 13000,
                "words": [
                    {
                        "w": f"word{i}",
                        "start_ms": 1000 + i * 450,
                        "end_ms": 1300 + i * 450,
                        "confidence": 0.95,
                    }
                    for i in range(24)
                ],
            },
        ]
        coaching = coach(thin)
        placeholders = [p for p in coaching.suggested_shape if p.kind == "placeholder"]
        assert placeholders, "one sentence cannot fill four slots"
        for part in placeholders:
            assert part.text.startswith("[") and part.text.endswith("]") and "?" in part.text
            assert not any(ch.isdigit() for ch in part.text)

    def test_priorities_state_impact_and_one_action_from_neutral_copy(self) -> None:
        """One or two priorities, each with a listener impact, an action and a drill."""
        coaching = coach(TURNS)
        assert 1 <= len(coaching.priorities) <= 2
        for priority in coaching.priorities:
            assert priority.listener_impact and priority.action and priority.drill
            assert priority.evidence_sequences
        joined = " ".join(f"{i} {a}" for i, a, _ in PRIORITY_COPY.values()).lower()
        for forbidden in ("you are bad", "poor", "weak", "fail", "incompeten", "accent"):
            assert forbidden not in joined


class TestTheGateRefusesInvention:
    """The gate is what outlives the floor when a model writes the shape."""

    def test_the_honest_coaching_passes(self) -> None:
        """What the floor produces is admitted."""
        preserve(TURNS, coach(TURNS))

    @pytest.mark.parametrize(
        "tamper",
        [
            ShapePart(
                slot="result", kind="quote", text="We saved the company two million.", sequence=3
            ),
            ShapePart(slot="result", kind="placeholder", text="[Latency fell 40 percent]"),
            ShapePart(slot="result", kind="placeholder", text="State the outcome."),
            ShapePart(
                slot="headline", kind="quote", text=_candidate_text(3).split(". ")[0], sequence=2
            ),
        ],
        ids=[
            "invented sentence",
            "fact in brackets",
            "unbracketed statement",
            "interviewer quoted",
        ],
    )
    def test_each_kind_of_invention_refuses_the_whole_coaching(self, tamper: ShapePart) -> None:
        """One lie anywhere and nothing is served."""
        honest = coach(TURNS)
        tampered = dataclasses.replace(
            honest, suggested_shape=(*honest.suggested_shape[:-1], tamper)
        )
        with pytest.raises(UnpreservingError):
            preserve(TURNS, tampered)

    def test_a_priority_citing_a_foreign_turn_is_refused(self) -> None:
        """Evidence must be the candidate's turns."""
        honest = coach(TURNS)
        bad = dataclasses.replace(honest.priorities[0], evidence_sequences=(2,))
        with pytest.raises(UnpreservingError):
            preserve(TURNS, dataclasses.replace(honest, priorities=(bad, *honest.priorities[1:])))

    def test_the_document_is_gated_before_it_is_served(self, monkeypatch) -> None:
        """A refused coaching becomes a stated absence, not a served invention."""
        import prepeet_ai.articulation.service as service

        def lying(turns, measured=None):  # type: ignore[no-untyped-def]
            raise UnpreservingError("test lie")

        monkeypatch.setattr(service, "coaching_document", lying)
        document = json.dumps({"session_id": "s", "turns": TURNS}).encode()
        monkeypatch.setattr(service, "fetch_verified", lambda u, d: document)
        analysis = json.loads(service.analysis_from_ref("http://x", "sha256:x"))
        assert analysis["coaching"]["available"] is False
        assert "withheld" in analysis["coaching"]["note"]

    def test_the_served_document_names_its_version(self) -> None:
        """Provenance on the coaching, like everything else."""
        assert coaching_document(TURNS)["coaching_version"] == COACHING_VERSION
        assert isinstance(coach(TURNS), DeliveryCoaching)
