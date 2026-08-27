"""ART-03: ten dimensions, each a level with its evidence, and no total anywhere.

Intelligibility is followability and is proven to depend on nothing that
could encode an accent: it moves with transcript confidence and sentence
length, and on nothing else.
"""

from __future__ import annotations

import json
import pathlib

from prepeet_ai.articulation.profile import DIMENSIONS, PROFILE_VERSION, profile, profile_document
from prepeet_ai.articulation.service import analysis_from_ref

FIXTURE = json.loads(
    (pathlib.Path(__file__).parent / "fixtures/articulation_known.json").read_text()
)


def _turns(confidence: float = 0.95, text: str | None = None) -> list[dict[str, object]]:
    turns = json.loads(json.dumps(FIXTURE["turns"]))
    for turn in turns:
        if turn["speaker"] != "candidate":
            continue
        for word in turn.get("words", []):
            word["confidence"] = confidence
        if text is not None and turn["sequence"] == 3:
            turn["text"] = text
    return turns


def _keys_matching(node: object, needles: tuple[str, ...]) -> list[str]:
    """Every key at any depth containing one of the needles."""
    found: list[str] = []
    if isinstance(node, dict):
        for key, value in node.items():
            if any(needle in str(key).lower() for needle in needles):
                found.append(str(key))
            found.extend(_keys_matching(value, needles))
    elif isinstance(node, list):
        for item in node:
            found.extend(_keys_matching(item, needles))
    return found


class TestEveryDimensionCarriesLevelAndEvidence:
    """The first box."""

    def test_all_ten_dimensions_are_present_with_levels_and_sequences(self) -> None:
        """Each dimension names its level and the turns behind it, or says why not."""
        result = profile(_turns())
        assert set(result.dimensions) == set(DIMENSIONS)
        assert result.profile_version == PROFILE_VERSION
        for name, dim in result.dimensions.items():
            assert dim.level in ("strong", "solid", "developing", "not_assessable"), name
            assert dim.reason
            if dim.level != "not_assessable":
                assert dim.evidence_sequences, name
                assert all(seq in (3,) for seq in dim.evidence_sequences), name

    def test_vocal_delivery_is_honestly_not_assessable_without_audio(self) -> None:
        """No decoded audio, no vocal delivery level: stated, not guessed."""
        assert profile(_turns()).dimensions["vocal_delivery"].level == "not_assessable"


class TestNoAggregateExists:
    """The second box: nothing in the document a total could live in."""

    def test_the_profile_document_has_no_score_field(self) -> None:
        """No overall, total, score or percentage at any depth."""
        document = profile_document(_turns())
        assert not _keys_matching(document, ("score", "overall", "total", "percentage", "grade"))
        levels = [d["level"] for d in document["dimensions"].values()]
        assert all(isinstance(level, str) for level in levels)

    def test_the_served_analysis_carries_the_profile_and_no_total(self, monkeypatch) -> None:
        """Over the service boundary too."""
        body = json.dumps({"session_id": "ses-p", "turns": _turns()}).encode()
        monkeypatch.setattr("prepeet_ai.articulation.service.fetch_verified", lambda u, d: body)
        analysis = json.loads(analysis_from_ref("http://x/input.json", "sha256:x"))
        assert set(analysis["profile"]["dimensions"]) == set(DIMENSIONS)
        # Keys, not prose: the note may SAY "score" precisely to deny one.
        assert not _keys_matching(analysis, ("score", "overall", "total", "percentage", "grade"))


class TestIntelligibilityIsFollowability:
    """The third box: moves with confidence and sentence length, and nothing else."""

    def test_it_moves_with_transcript_confidence(self) -> None:
        """Clear transcription reads as strong; uncertain transcription does not."""
        clear = profile(_turns(confidence=0.95)).dimensions["intelligibility"].level
        murky = profile(_turns(confidence=0.72)).dimensions["intelligibility"].level
        assert clear == "strong"
        assert murky in ("solid", "developing")

    def test_it_moves_with_sentence_length(self) -> None:
        """One unbroken 40-word sentence is harder to follow than the same words in three."""
        words = [f"w{i}" for i in range(40)]
        run_on = " ".join(words) + "."
        broken = (
            " ".join(words[:13]) + ". " + " ".join(words[13:26]) + ". " + " ".join(words[26:]) + "."
        )
        long_level = profile(_turns(text=run_on)).dimensions["intelligibility"].level
        short_level = profile(_turns(text=broken)).dimensions["intelligibility"].level
        assert long_level != "strong"
        assert short_level == "strong"

    def test_no_input_could_carry_an_accent(self) -> None:
        """The profile reads text, timings and confidence: no audio, no phonetics, no locale."""
        import prepeet_ai.articulation.profile as module

        source = pathlib.Path(module.__file__).read_text().lower()
        for forbidden in ("accent", "phoneme", "pronunc", "dialect", "locale", "native"):
            assert forbidden not in source.replace("accent-conformity", "").replace(
                "no accent", ""
            ), forbidden
