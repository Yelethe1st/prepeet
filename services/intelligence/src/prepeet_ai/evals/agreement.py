"""Inter-rater agreement: the arithmetic QUA-03 needs before it has data.

A threshold "derived from human agreement data" is only as good as the
agreement figure behind it, so the estimators live here as ordinary
production code: linted, typed, documented and checked against
hand-computed answers rather than against their own output.

Four estimators, because they answer different questions and a calibration
record that quotes one without the others is easy to read too kindly.
Observed agreement is what a reader thinks agreement means and is inflated
whenever one label dominates, which for evidence sufficiency it will.
Cohen's kappa corrects for chance between exactly two raters. Fleiss'
kappa does the same for a fixed number of raters per item. Krippendorff's
alpha is the one that survives a rater who did not rate every item, which
every real exercise produces.

Where the arithmetic is undefined, these functions refuse. A kappa whose
denominator is zero is not 1.0, and an agreement rate over zero comparable
items is not 0.0; reporting either as a number is how a calibration record
comes to claim reliability that the data never showed.

Nothing here knows what a competency is. The estimators take labels, so
they can be pointed at any rating exercise, and the meaning lives in the
calibration module that calls them.
"""

from __future__ import annotations

import itertools
from collections import Counter
from collections.abc import Iterable, Mapping
from dataclasses import dataclass


class UndefinedAgreementError(Exception):
    """Raised when the data cannot support the statistic that was asked for.

    Deliberately an exception rather than a sentinel value. A caller that
    ignores it will crash a calibration run, which is the correct outcome:
    the alternative is a report quoting a number that means nothing.
    """


@dataclass(frozen=True)
class Rating:
    """One rater's label for one item.

    Item and rater are opaque identifiers. A rater is whoever produced the
    label, which is a person for a benchmark set and a declared rule for a
    synthetic exercise; the calibration module keeps that distinction, not
    this one.
    """

    item_id: str
    rater_id: str
    label: str


@dataclass(frozen=True)
class LabelAgreement:
    """How one label map compares with another, and what it could not compare.

    `unmatched` is the honest half: items present in the reference and
    absent from the comparison are named rather than silently dropped or
    counted as disagreements.
    """

    compared: int
    agreed: int
    rate: float
    unmatched: tuple[str, ...]
    confusion: Mapping[tuple[str, str], int]


def _by_item(ratings: Iterable[Rating]) -> dict[str, list[Rating]]:
    grouped: dict[str, list[Rating]] = {}
    for rating in ratings:
        grouped.setdefault(rating.item_id, []).append(rating)
    return grouped


def observed_agreement(ratings: Iterable[Rating]) -> float:
    """The share of rater pairs that chose the same label.

    Pairs rather than items, so an item three raters split two to one
    counts as one third rather than as a whole disagreement. Items with a
    single rating contribute no pairs, because one rating is neither an
    agreement nor a disagreement.
    """
    agreeing = 0
    pairs = 0
    for item_ratings in _by_item(ratings).values():
        for left, right in itertools.combinations(item_ratings, 2):
            pairs += 1
            if left.label == right.label:
                agreeing += 1
    if pairs == 0:
        raise UndefinedAgreementError(
            "no item carries two ratings, so there is no agreement to observe"
        )
    return agreeing / pairs


def cohens_kappa(ratings: Iterable[Rating], rater_a: str, rater_b: str) -> float:
    """Chance-corrected agreement between two named raters.

    Only items both raters labelled are counted. A rater who skipped an
    item has expressed no opinion about it, and scoring the gap either way
    would be inventing one.
    """
    grouped = _by_item(ratings)
    paired: list[tuple[str, str]] = []
    for item_ratings in grouped.values():
        labels = {rating.rater_id: rating.label for rating in item_ratings}
        if rater_a in labels and rater_b in labels:
            paired.append((labels[rater_a], labels[rater_b]))
    if not paired:
        raise UndefinedAgreementError(
            f"{rater_a!r} and {rater_b!r} share no rated item, so kappa has nothing to correct"
        )

    total = len(paired)
    observed = sum(1 for left, right in paired if left == right) / total
    left_counts = Counter(left for left, _ in paired)
    right_counts = Counter(right for _, right in paired)
    expected = sum(
        (left_counts[label] / total) * (right_counts[label] / total)
        for label in set(left_counts) | set(right_counts)
    )
    if expected >= 1.0:
        raise UndefinedAgreementError(
            "both raters used a single label, so chance agreement is total and kappa is undefined"
        )
    return (observed - expected) / (1 - expected)


def fleiss_kappa(ratings: Iterable[Rating]) -> float:
    """Chance-corrected agreement across a fixed number of ratings per item.

    Fleiss assumes every item carries the same number of ratings, and the
    raters need not be the same people from item to item. An uneven set is
    refused rather than padded, because padding it would change the answer
    without saying so; Krippendorff's alpha is the estimator for that case.
    """
    grouped = _by_item(ratings)
    if not grouped:
        raise UndefinedAgreementError("no ratings, so there is no agreement to correct")
    sizes = {len(item_ratings) for item_ratings in grouped.values()}
    if len(sizes) != 1:
        raise UndefinedAgreementError(
            f"items carry {sorted(sizes)} ratings; Fleiss' kappa needs one fixed count. "
            "Use Krippendorff's alpha for an uneven set"
        )
    per_item = sizes.pop()
    if per_item < 2:
        raise UndefinedAgreementError("one rating per item is not agreement")

    labels = sorted({rating.label for rating in itertools.chain.from_iterable(grouped.values())})
    items = len(grouped)
    item_terms: list[float] = []
    label_totals: Counter[str] = Counter()
    for item_ratings in grouped.values():
        counts = Counter(rating.label for rating in item_ratings)
        label_totals.update(counts)
        squares = sum(counts[label] ** 2 for label in labels)
        item_terms.append((squares - per_item) / (per_item * (per_item - 1)))

    observed = sum(item_terms) / items
    expected = sum((label_totals[label] / (items * per_item)) ** 2 for label in labels)
    if expected >= 1.0:
        raise UndefinedAgreementError(
            "every rating is the same label, so chance agreement is total and kappa is undefined"
        )
    return (observed - expected) / (1 - expected)


def krippendorff_alpha(ratings: Iterable[Rating]) -> float:
    """Nominal alpha: chance-corrected agreement that tolerates missing ratings.

    Units rated once are dropped, which is alpha's own rule rather than a
    convenience: a single rating carries no information about agreement.
    The nominal difference function is used because the label set is not
    an interval scale and `not_assessable` is not one end of it. Treating
    the labels as ordered would quietly assert that a not-assessable
    competency is a low-confidence one, which ADR-0015 says it is not.
    """
    coincidences: dict[tuple[str, str], float] = {}
    for item_ratings in _by_item(ratings).values():
        size = len(item_ratings)
        if size < 2:
            continue
        for left, right in itertools.permutations(item_ratings, 2):
            pair = (left.label, right.label)
            coincidences[pair] = coincidences.get(pair, 0.0) + 1 / (size - 1)

    total = sum(coincidences.values())
    if total == 0:
        raise UndefinedAgreementError(
            "no unit carries two ratings, so alpha has no coincidences to count"
        )

    marginals: dict[str, float] = {}
    for (left_label, _), count in coincidences.items():
        marginals[left_label] = marginals.get(left_label, 0.0) + count

    observed = sum(count for (left, right), count in coincidences.items() if left != right) / total
    expected = sum(
        marginals[left] * marginals[right] for left, right in itertools.permutations(marginals, 2)
    ) / (total * (total - 1))
    if expected == 0:
        raise UndefinedAgreementError(
            "every rating is the same label, so expected disagreement is zero "
            "and alpha is undefined"
        )
    return 1 - observed / expected


def consensus_labels(ratings: Iterable[Rating]) -> dict[str, str]:
    """The majority label per item, where there is one.

    A tie has no consensus and the item is left out. Breaking it by sort
    order or by whichever rater happens to be first would manufacture an
    answer the raters did not give, and a threshold fitted to manufactured
    answers is fitted to nothing.
    """
    consensus: dict[str, str] = {}
    for item_id, item_ratings in _by_item(ratings).items():
        counts = Counter(rating.label for rating in item_ratings)
        ranked = counts.most_common()
        if len(ranked) > 1 and ranked[0][1] == ranked[1][1]:
            continue
        consensus[item_id] = ranked[0][0]
    return consensus


def label_agreement(reference: Mapping[str, str], comparison: Mapping[str, str]) -> LabelAgreement:
    """How often a comparison label map matches a reference one.

    Used to score a candidate threshold against a human consensus. Items
    the comparison has no label for are named in `unmatched` rather than
    counted as disagreements, because a threshold cannot be blamed for an
    item the pipeline never produced a result for.
    """
    shared = [item for item in reference if item in comparison]
    if not shared:
        raise UndefinedAgreementError(
            "the two label maps share no item, so there is no agreement to measure"
        )
    confusion: Counter[tuple[str, str]] = Counter()
    agreed = 0
    for item in shared:
        confusion[(reference[item], comparison[item])] += 1
        if reference[item] == comparison[item]:
            agreed += 1
    return LabelAgreement(
        compared=len(shared),
        agreed=agreed,
        rate=agreed / len(shared),
        unmatched=tuple(sorted(item for item in reference if item not in comparison)),
        confusion=dict(confusion),
    )
