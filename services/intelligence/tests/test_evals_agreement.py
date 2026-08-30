"""QUA-03: the agreement arithmetic, checked against hand-computed answers.

Every expected value in this file is worked out in the test's own docstring
from the published definition of the statistic. That is deliberate. An
agreement figure quoted in a calibration record is only as trustworthy as
the function that produced it, and a function checked only against its own
output is checked against nothing.

Nothing here involves a human rating. These are arithmetic properties of
the estimators, exercised on labels chosen to make the arithmetic legible.
"""

from __future__ import annotations

import pytest

from prepeet_ai.evals.agreement import (
    Rating,
    UndefinedAgreementError,
    cohens_kappa,
    consensus_labels,
    fleiss_kappa,
    krippendorff_alpha,
    label_agreement,
    observed_agreement,
)


def _ratings(rows: list[tuple[str, str, str]]) -> tuple[Rating, ...]:
    return tuple(Rating(item_id=item, rater_id=rater, label=label) for item, rater, label in rows)


class TestObservedAgreement:
    """The raw proportion, which is the number chance correction argues with."""

    def test_total_agreement_is_one(self) -> None:
        """Two raters, two items, no disagreement anywhere."""
        ratings = _ratings(
            [("i1", "a", "high"), ("i1", "b", "high"), ("i2", "a", "low"), ("i2", "b", "low")]
        )

        assert observed_agreement(ratings) == 1.0

    def test_total_disagreement_is_zero(self) -> None:
        """Every rateable pair disagrees, so the proportion is zero."""
        ratings = _ratings(
            [("i1", "a", "high"), ("i1", "b", "low"), ("i2", "a", "low"), ("i2", "b", "high")]
        )

        assert observed_agreement(ratings) == 0.0

    def test_it_is_the_share_of_agreeing_pairs_not_of_items(self) -> None:
        """Three raters on one item, labels high, high, low.

        Three pairs: (a,b) agree, (a,c) and (b,c) do not. One in three.
        """
        ratings = _ratings([("i1", "a", "high"), ("i1", "b", "high"), ("i1", "c", "low")])

        assert observed_agreement(ratings) == pytest.approx(1 / 3)

    def test_an_item_only_one_rater_saw_contributes_no_pairs(self) -> None:
        """A single rating is not an agreement or a disagreement."""
        ratings = _ratings([("i1", "a", "high"), ("i1", "b", "high"), ("i2", "a", "low")])

        assert observed_agreement(ratings) == 1.0

    def test_nothing_to_compare_is_refused_rather_than_reported_as_perfect(self) -> None:
        """No pair means no agreement figure, and zero pairs is not 1.0."""
        with pytest.raises(UndefinedAgreementError):
            observed_agreement(_ratings([("i1", "a", "high")]))


class TestCohensKappa:
    """Two raters, chance corrected."""

    def test_the_worked_two_by_two_table(self) -> None:
        """Fifty items: 20 both high, 15 both low, 5 and 10 disagreeing.

        Observed agreement is (20 + 15) / 50 = 0.7. Rater a said high 25
        times and rater b said high 30 times, so chance agreement is
        (25 * 30 + 25 * 20) / 2500 = 0.5. Kappa is (0.7 - 0.5) / 0.5 = 0.4.
        """
        rows: list[tuple[str, str, str]] = []
        for index in range(20):
            rows += [(f"agree-high-{index}", "a", "high"), (f"agree-high-{index}", "b", "high")]
        for index in range(15):
            rows += [(f"agree-low-{index}", "a", "low"), (f"agree-low-{index}", "b", "low")]
        for index in range(5):
            rows += [(f"a-high-{index}", "a", "high"), (f"a-high-{index}", "b", "low")]
        for index in range(10):
            rows += [(f"b-high-{index}", "a", "low"), (f"b-high-{index}", "b", "high")]

        assert cohens_kappa(_ratings(rows), "a", "b") == pytest.approx(0.4)

    def test_perfect_agreement_over_two_labels_is_one(self) -> None:
        """Observed 1.0 against chance 0.5 is kappa 1.0."""
        ratings = _ratings(
            [("i1", "a", "high"), ("i1", "b", "high"), ("i2", "a", "low"), ("i2", "b", "low")]
        )

        assert cohens_kappa(ratings, "a", "b") == pytest.approx(1.0)

    def test_agreement_no_better_than_chance_is_zero(self) -> None:
        """Observed 0.5 with both raters splitting evenly is chance itself.

        Four items: both high, both low, and two crossed. Observed is 0.5,
        each rater says high twice, so chance is also 0.5 and kappa is 0.
        """
        ratings = _ratings(
            [
                ("i1", "a", "high"),
                ("i1", "b", "high"),
                ("i2", "a", "low"),
                ("i2", "b", "low"),
                ("i3", "a", "high"),
                ("i3", "b", "low"),
                ("i4", "a", "low"),
                ("i4", "b", "high"),
            ]
        )

        assert cohens_kappa(ratings, "a", "b") == pytest.approx(0.0)

    def test_only_the_items_both_raters_saw_are_counted(self) -> None:
        """An item one rater skipped cannot be agreement or disagreement."""
        ratings = _ratings(
            [
                ("i1", "a", "high"),
                ("i1", "b", "high"),
                ("i2", "a", "low"),
                ("i2", "b", "low"),
                ("i3", "a", "high"),
            ]
        )

        assert cohens_kappa(ratings, "a", "b") == pytest.approx(1.0)

    def test_one_label_used_by_everyone_is_undefined_rather_than_perfect(self) -> None:
        """Chance agreement of 1.0 makes the denominator zero.

        Two raters who only ever say high agree by construction. Reporting
        that as kappa 1.0 would claim reliability that the data cannot
        show, so it is refused instead.
        """
        ratings = _ratings(
            [("i1", "a", "high"), ("i1", "b", "high"), ("i2", "a", "high"), ("i2", "b", "high")]
        )

        with pytest.raises(UndefinedAgreementError):
            cohens_kappa(ratings, "a", "b")

    def test_a_rater_who_is_not_in_the_data_is_an_error(self) -> None:
        """Naming an absent rater is a mistake, not an agreement of zero."""
        ratings = _ratings([("i1", "a", "high"), ("i1", "b", "high")])

        with pytest.raises(UndefinedAgreementError):
            cohens_kappa(ratings, "a", "c")


class TestFleissKappa:
    """More than two raters, where the raters need not be the same people."""

    def test_unanimous_and_split_categories_is_one(self) -> None:
        """Two items, three raters, all high then all low.

        Each item's agreement term is (3^2 - 3) / (3 * 2) = 1, so mean
        observed agreement is 1. Each label was used half the time, so
        chance is 0.5^2 + 0.5^2 = 0.5, and kappa is (1 - 0.5) / 0.5 = 1.
        """
        ratings = _ratings(
            [
                ("i1", "a", "high"),
                ("i1", "b", "high"),
                ("i1", "c", "high"),
                ("i2", "a", "low"),
                ("i2", "b", "low"),
                ("i2", "c", "low"),
            ]
        )

        assert fleiss_kappa(ratings) == pytest.approx(1.0)

    def test_systematic_two_one_splits_go_below_zero(self) -> None:
        """Two items rated two-one and one-two.

        Each item's term is (2^2 + 1^2 - 3) / 6 = 1/3, so observed is 1/3.
        Both labels are used half the time, so chance is 0.5, and kappa is
        (1/3 - 1/2) / (1 - 1/2), which is -1/3. Worse than chance is a real
        finding and it is reported rather than clamped to zero.
        """
        ratings = _ratings(
            [
                ("i1", "a", "high"),
                ("i1", "b", "high"),
                ("i1", "c", "low"),
                ("i2", "a", "low"),
                ("i2", "b", "low"),
                ("i2", "c", "high"),
            ]
        )

        assert fleiss_kappa(ratings) == pytest.approx(-1 / 3)

    def test_an_unequal_number_of_raters_per_item_is_refused(self) -> None:
        """Fleiss assumes a fixed rating count per item, so say so loudly."""
        ratings = _ratings(
            [
                ("i1", "a", "high"),
                ("i1", "b", "high"),
                ("i1", "c", "high"),
                ("i2", "a", "low"),
                ("i2", "b", "low"),
            ]
        )

        with pytest.raises(UndefinedAgreementError):
            fleiss_kappa(ratings)

    def test_a_single_label_across_the_whole_set_is_undefined(self) -> None:
        """Chance agreement of one leaves nothing to correct against."""
        ratings = _ratings([("i1", "a", "high"), ("i1", "b", "high"), ("i1", "c", "high")])

        with pytest.raises(UndefinedAgreementError):
            fleiss_kappa(ratings)


class TestKrippendorffAlpha:
    """The estimator that survives a rater who did not rate everything."""

    def test_the_worked_four_unit_example(self) -> None:
        """Two coders on four units: AA, AB, BB, BB.

        Coincidences over ordered pairs: o(A,A) = 2, o(A,B) = o(B,A) = 1,
        o(B,B) = 4, so n(A) = 3, n(B) = 5 and n = 8. Observed disagreement
        is 2 / 8 = 0.25. Expected is (3 * 5 + 5 * 3) / (8 * 7) = 30 / 56.
        Alpha is 1 - 0.25 / (30 / 56), which is 0.5333 recurring.
        """
        ratings = _ratings(
            [
                ("u1", "a", "A"),
                ("u1", "b", "A"),
                ("u2", "a", "A"),
                ("u2", "b", "B"),
                ("u3", "a", "B"),
                ("u3", "b", "B"),
                ("u4", "a", "B"),
                ("u4", "b", "B"),
            ]
        )

        assert krippendorff_alpha(ratings) == pytest.approx(1 - 0.25 / (30 / 56))

    def test_a_unit_only_one_coder_reached_is_dropped_not_guessed(self) -> None:
        """The same four-unit answer, with a fifth unit one coder skipped.

        This is why alpha is here rather than only kappa: a real rating
        exercise loses ratings, and an estimator that cannot take a gap
        invites somebody to fill it in.
        """
        rows = [
            ("u1", "a", "A"),
            ("u1", "b", "A"),
            ("u2", "a", "A"),
            ("u2", "b", "B"),
            ("u3", "a", "B"),
            ("u3", "b", "B"),
            ("u4", "a", "B"),
            ("u4", "b", "B"),
            ("u5", "a", "A"),
        ]

        assert krippendorff_alpha(_ratings(rows)) == pytest.approx(1 - 0.25 / (30 / 56))

    def test_perfect_agreement_across_two_labels_is_one(self) -> None:
        """No observed disagreement at all, with both labels in play."""
        ratings = _ratings([("u1", "a", "A"), ("u1", "b", "A"), ("u2", "a", "B"), ("u2", "b", "B")])

        assert krippendorff_alpha(ratings) == pytest.approx(1.0)

    def test_one_label_everywhere_is_undefined(self) -> None:
        """Expected disagreement of zero is a denominator, not a result."""
        ratings = _ratings([("u1", "a", "A"), ("u1", "b", "A"), ("u2", "a", "A"), ("u2", "b", "A")])

        with pytest.raises(UndefinedAgreementError):
            krippendorff_alpha(ratings)


class TestConsensusAndPerLabelAgreement:
    """What a threshold sweep compares the machine against."""

    def test_a_majority_label_is_the_consensus(self) -> None:
        """Two of three raters carry the item."""
        ratings = _ratings([("i1", "a", "high"), ("i1", "b", "high"), ("i1", "c", "low")])

        assert consensus_labels(ratings) == {"i1": "high"}

    def test_a_tie_has_no_consensus_rather_than_the_first_label(self) -> None:
        """An item the raters split evenly is excluded from comparison.

        Breaking the tie by sort order or by rater order would invent an
        answer the raters did not give, and a threshold fitted against
        invented answers is fitted against nothing.
        """
        ratings = _ratings([("i1", "a", "high"), ("i1", "b", "low")])

        assert consensus_labels(ratings) == {}

    def test_agreement_against_a_label_map_counts_only_shared_items(self) -> None:
        """Items the machine did not label are reported, never assumed wrong."""
        consensus = {"i1": "high", "i2": "low", "i3": "medium"}
        machine = {"i1": "high", "i2": "high"}

        measured = label_agreement(consensus, machine)

        assert measured.compared == 2
        assert measured.agreed == 1
        assert measured.rate == pytest.approx(0.5)
        assert measured.unmatched == ("i3",)
        assert measured.confusion[("low", "high")] == 1

    def test_nothing_in_common_is_refused_rather_than_scored_zero(self) -> None:
        """Zero out of zero is not an agreement of zero."""
        with pytest.raises(UndefinedAgreementError):
            label_agreement({"i1": "high"}, {"i2": "low"})


class TestTheEstimatorsRefuseRatherThanGuess:
    """The undefined cases, each reached by data that really is undefined."""

    def test_fleiss_on_nothing_at_all(self) -> None:
        """No ratings is not perfect agreement."""
        with pytest.raises(UndefinedAgreementError):
            fleiss_kappa(())

    def test_fleiss_where_every_item_carries_one_rating(self) -> None:
        """One opinion per item is a survey, not an agreement study."""
        ratings = _ratings([("i1", "a", "high"), ("i2", "b", "low")])

        with pytest.raises(UndefinedAgreementError):
            fleiss_kappa(ratings)

    def test_alpha_where_no_unit_was_rated_twice(self) -> None:
        """Alpha counts coincidences, and there are none to count."""
        ratings = _ratings([("u1", "a", "A"), ("u2", "b", "B")])

        with pytest.raises(UndefinedAgreementError):
            krippendorff_alpha(ratings)
