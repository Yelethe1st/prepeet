"""ART-08: hedging counted from the transcript, and never made a target.

The counting is here; whether a hedge is worth mentioning is decided in the
profile, where it is known whether the claim was backed. Both halves are
proven, because the distinction is the whole point of the feature: an answer
that hedges nothing claims a certainty the candidate may not have.
"""

from __future__ import annotations

import json
import pathlib

from prepeet_ai.articulation.features import (
    HEDGE_PHRASES,
    HEDGES_VERSION,
    _hedges,
    turn_features,
)
from prepeet_ai.articulation.profile import profile

FIXTURE = json.loads(
    (pathlib.Path(__file__).parent / "fixtures/articulation_known.json").read_text()
)


def _tokens(text: str) -> list[str]:
    return text.lower().split()


def _candidate_turn(text: str) -> dict[str, object]:
    """A turn long enough to be assessable, saying exactly what is given."""
    words = text.split()
    return {
        "sequence": 3,
        "speaker": "candidate",
        "text": text,
        "start_ms": 0,
        "end_ms": len(words) * 400,
        "words": [
            {"word": word, "start_ms": index * 400, "end_ms": index * 400 + 300, "confidence": 0.95}
            for index, word in enumerate(words)
        ],
    }


class TestCounting:
    """Counting is arithmetic over tokens: no model, no audio, no locale."""

    def test_counts_a_softening_phrase(self) -> None:
        """The simplest case, so a regression here is unmistakable."""
        count, phrases = _hedges(_tokens("i think we shipped it on friday"))

        assert count == 1
        assert phrases == ("i think",)

    def test_counts_each_occurrence_and_names_each_phrase_once(self) -> None:
        """Four repeats of the same hedge tell the candidate nothing new."""
        count, phrases = _hedges(_tokens("i think maybe i think we were probably fine"))

        assert count == 4
        assert phrases == ("i think", "maybe", "probably")

    def test_matches_the_longest_phrase_rather_than_the_one_inside_it(self) -> None:
        """'a little bit' is one hedge, not also the 'a bit' within it."""
        count, phrases = _hedges(_tokens("it was a little bit slower"))

        assert count == 1
        assert phrases == ("a little bit",)

    def test_finds_nothing_in_a_plainly_stated_answer(self) -> None:
        """A plainly stated answer counts zero, and reports no phrases."""
        count, phrases = _hedges(_tokens("we cut the p99 to eighty milliseconds"))

        assert count == 0
        assert phrases == ()

    def test_leaves_out_the_words_that_are_not_softeners(self) -> None:
        """'just', 'like' and 'really' are excluded, and the data says why."""
        count, _ = _hedges(_tokens("we just shipped it really fast like that"))

        assert count == 0

    def test_reaches_a_turn_as_a_measurement(self) -> None:
        """The count arrives on the turn, beside the fillers it resembles."""
        text = (
            "i think the queue was probably the cause and we saw it recover "
            "after we drained it and restarted the workers on the second node"
        )
        turn = turn_features(_candidate_turn(text))

        assert turn.hedge_count == 2
        assert turn.hedge_phrases == ("i think", "probably")


class TestTheListIsData:
    """ART-08's second box: adding a phrase is a data change and a test."""

    def test_the_phrases_come_from_the_data_file(self) -> None:
        """The module reads the file rather than restating it."""
        data = json.loads(
            (
                pathlib.Path(__file__).parents[1] / "src/prepeet_ai/articulation/hedges.json"
            ).read_text()
        )

        assert data["version"] == HEDGES_VERSION
        assert len(HEDGE_PHRASES) == len(data["phrases"])

    def test_the_phrases_are_ordered_longest_first(self) -> None:
        """Or a phrase is never matched, because a shorter one inside it wins."""
        lengths = [len(phrase) for phrase in HEDGE_PHRASES]

        assert lengths == sorted(lengths, reverse=True)


LONG_HEDGED_FIGURE = (
    "i think we cut it to 80 milliseconds across the whole estate and the change "
    + "held through the next release without a single rollback anywhere"
)
LONG_PLAIN_FIGURE = (
    "we cut it to 80 milliseconds across the whole estate and the change held "
    + "through the next release without a single rollback anywhere"
)
LONG_HONEST = (
    "i think it was slower afterwards but honestly i could not tell you by how "
    + "much because nobody was measuring that path at the time and we never went back"
)


class TestOnlyAHedgeInFrontOfEvidenceIsWorthMentioning:
    """ART-08's distinction, which is the reason the feature is safe to have."""

    def _profile(self, text: str) -> object:
        turns = json.loads(json.dumps(FIXTURE["turns"]))
        for turn in turns:
            if turn["speaker"] != "candidate":
                continue
            for word in turn.get("words", []):
                word["confidence"] = 0.95
            if turn["sequence"] == 3:
                turn["text"] = text
        return profile(turns).dimensions

    def test_the_replacement_text_is_actually_assessable(self) -> None:
        """Or every assertion below passes on 'no assessable candidate speech'.

        Which is exactly what happened while writing them: the replacement was
        eleven words, under the twenty-word floor, and the two tests asserting
        an absence passed without measuring anything.
        """
        got = self._profile(LONG_HEDGED_FIGURE)

        assert got["precision"].level != "not_assessable"

    def test_a_hedge_in_front_of_a_figure_is_named(self) -> None:
        """The candidate had the number and softened it anyway."""
        got = self._profile(LONG_HEDGED_FIGURE)

        assert "softened it with" in got["precision"].reason
        assert '"i think"' in got["precision"].reason

    def test_a_hedge_marking_an_honest_estimate_is_left_alone(self) -> None:
        """No figure behind it, so it is somebody being careful, not vague."""
        got = self._profile(LONG_HONEST)

        assert "softened it with" not in got["precision"].reason

    def test_a_plain_figure_is_not_reported_as_softened(self) -> None:
        """Nothing to soften, so nothing is said about softening."""
        got = self._profile(LONG_PLAIN_FIGURE)

        assert "softened it with" not in got["precision"].reason


class TestHedgingIsNeverATarget:
    """ART-07's rule, and the reason zero would be the wrong goal."""

    def _reason_and_level(self, text: str) -> tuple[str, str]:
        turns = json.loads(json.dumps(FIXTURE["turns"]))
        for turn in turns:
            if turn["speaker"] != "candidate":
                continue
            for word in turn.get("words", []):
                word["confidence"] = 0.95
            if turn["sequence"] == 3:
                turn["text"] = text
        got = profile(turns).dimensions["precision"]
        return got.reason, got.level

    def test_hedging_does_not_move_the_level(self) -> None:
        """The same answer, softened and plain, is equally precise."""
        plain = self._reason_and_level(LONG_PLAIN_FIGURE)
        softened = self._reason_and_level(LONG_HEDGED_FIGURE)

        assert softened[1] == plain[1]

    def test_no_target_is_stated_anywhere_in_the_reason(self) -> None:
        """The reason states what was said. It never asks for less of it."""
        reason, _ = self._reason_and_level(LONG_HEDGED_FIGURE)
        forbidden = ("should", "aim", "target", "too many", "avoid", "reduce", "stop")

        assert not [word for word in forbidden if word in reason.lower()]
