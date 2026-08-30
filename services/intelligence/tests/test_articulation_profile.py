"""ART-03: ten dimensions, each a level with its evidence, and no total anywhere.

Intelligibility is followability and is proven to depend on nothing that
could encode an accent: it moves with transcript confidence and sentence
length, and on nothing else.
"""

from __future__ import annotations

import ast
import json
import pathlib

from prepeet_ai.articulation.profile import DIMENSIONS, PROFILE_VERSION, profile, profile_document
from prepeet_ai.articulation.service import analysis_from_ref

FIXTURE = json.loads(
    (pathlib.Path(__file__).parent / "fixtures/articulation_known.json").read_text()
)


def _turns(
    confidence: float = 0.95,
    text: str | None = None,
    words_per_minute: float | None = None,
) -> list[dict[str, object]]:
    turns = json.loads(json.dumps(FIXTURE["turns"]))
    for turn in turns:
        if turn["speaker"] != "candidate":
            continue
        for word in turn.get("words", []):
            word["confidence"] = confidence
        if text is not None and turn["sequence"] == 3:
            turn["text"] = text
        if words_per_minute is not None:
            # Stretch or compress the turn to hit a rate, keeping the words.
            # The word timings move with it, because a turn whose duration
            # disagrees with its words is not a turn the calculator would see.
            spoken = len(str(turn.get("text", "")).split())
            duration = int(spoken / words_per_minute * 60_000) or 1
            start = int(turn["start_ms"])
            turn["end_ms"] = start + duration
            words = turn.get("words", [])
            for index, word in enumerate(words):
                step = duration // max(len(words), 1)
                word["start_ms"] = start + index * step
                word["end_ms"] = start + index * step + max(step - 1, 1)
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
            assert dim.level in (
                "strong",
                "solid",
                "developing",
                # A measurement with no direction. Pace is reported and
                # never graded, because the product's position is that
                # there is no correct speaking rate.
                "observed",
                "not_assessable",
            ), name
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

    def test_it_does_not_move_with_transcript_confidence(self) -> None:
        """Provider uncertainty is not a judgment about the candidate.

        This asserted the opposite until the review: confidence below 0.85
        could not be strong and below 0.70 was developing, so a turn that was
        assessable at 0.60 was graded down for the microphone. A provider is
        less certain about unfamiliar speech, unusual vocabulary and poor
        rooms, and none of those is a fact about how followable somebody was.
        Confidence gates assessability upstream and decides nothing here.
        """
        clear = profile(_turns(confidence=0.95)).dimensions["intelligibility"].level
        murky = profile(_turns(confidence=0.62)).dimensions["intelligibility"].level

        assert clear == murky, "transcription certainty changed the candidate's level"

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
        """The profile reads text, timings and confidence: no audio, no phonetics, no locale.

        Checked against the code with its comments and docstrings removed.
        Reading the whole file was the first version and it banned the word
        rather than the input: it fired on the comment explaining why the
        confidence grading had been taken out, which is the one place the word
        genuinely belongs. Prose is how the reasoning survives; what matters
        is that nothing named for any of these is ever read.
        """
        import prepeet_ai.articulation.profile as module

        source = pathlib.Path(module.__file__).read_text()
        tree = ast.parse(source)
        for node in ast.walk(tree):
            if isinstance(node, (ast.Module, ast.ClassDef, ast.FunctionDef)):
                node.body = [
                    statement
                    for statement in node.body
                    if not (
                        isinstance(statement, ast.Expr)
                        and isinstance(statement.value, ast.Constant)
                        and isinstance(statement.value.value, str)
                    )
                ]
        code = ast.unparse(tree).lower()

        for forbidden in ("accent", "phoneme", "pronunc", "dialect", "locale", "native"):
            assert forbidden not in code, forbidden


class TestPaceIsObservedNeverGraded:
    """Regression from the review: fixed WPM bands were a universal standard.

    The profile awarded strong, solid or developing from 130-170 and 110-190,
    so a candidate whose ordinary rate falls outside them was told their
    delivery was developing for speaking the way they speak, and the coaching
    told them to aim for a different rate. The product's position is that there
    is no correct speaking rate, and ART-07's personal baseline was displayed
    beside the grade rather than replacing it.
    """

    def test_a_slow_and_a_fast_speaker_get_the_same_level(self) -> None:
        """Because the level is not about the rate at all."""
        slow = profile(_turns(words_per_minute=90)).dimensions["pace"]
        fast = profile(_turns(words_per_minute=210)).dimensions["pace"]

        assert slow.level == fast.level == "observed"

    def test_the_number_is_still_reported(self) -> None:
        """Observed is not withheld: the finding is the number."""
        measured = profile(_turns(words_per_minute=90)).dimensions["pace"]

        assert "90" in measured.reason
        assert measured.evidence_sequences

    def test_no_level_ranks_a_speaking_rate(self) -> None:
        """None of the graded levels can ever be reached by pace."""
        for rate in (60, 90, 120, 150, 180, 240):
            level = profile(_turns(words_per_minute=rate)).dimensions["pace"].level
            assert level not in ("strong", "solid", "developing"), rate
