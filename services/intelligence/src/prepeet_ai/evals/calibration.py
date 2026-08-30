"""QUA-03: the calibration harness, and the calibration it refuses to claim.

ADR-0015 makes confidence a qualitative per-competency label and forbids any
numeric confidence until QUA-03 calibrates the thresholds against human
benchmark ratings. This module is QUA-03's machinery. It is not QUA-03's
answer, because the answer needs human judgements and there are none in this
repository.

So the deliverable is deliberately shaped as two halves. The first is
everything that can be built without the data: the derivation rule the
product actually uses, a sweep over every candidate threshold that would
still be a valid rubric, the agreement estimators in `agreement.py`, and the
record a calibration would have to produce. The second is a refusal.
`calibrate` returns `calibrated=False` with a named reason, and
`numeric_confidence_permitted` stays False, so ADR-0015's prohibition is a
value this code computes rather than a sentence somebody remembers.

The refusal is the load-bearing part. A calibrator that would accept
invented ratings is worse than no calibrator at all, because its output
carries the word calibrated into a rubric version and from there onto a
surface a candidate reads. Two separate acts are therefore required before
any number here can be called calibrated: a rating set that declares human
provenance with a collection record, and the plan's owner naming that set in
`approved_benchmark_sets`. Neither has happened.

The synthetic exercise set is scaffolding and says so in its own file. Its
raters are declared rules rather than people, its labels are computed rather
than stored so that no invented judgement is ever committed as data, and
every figure derived from it is stamped `exercise_only`.

One duplication is deliberate and worth naming: `derive_confidence` mirrors
Go's `confidenceOf` in `internal/evaluation/rubric.go`. The sweep is only
worth running against the rule the product actually applies, and this
service cannot call into Go. Nothing enforces that the two stay in step, so
a cross-language check belongs with the cross-language harness QUA-02
already records as the honest next step.
"""

from __future__ import annotations

import itertools
import json
import pathlib
from collections.abc import Iterable, Mapping, Sequence
from dataclasses import dataclass
from typing import Any

from prepeet_ai.evals.agreement import (
    LabelAgreement,
    Rating,
    UndefinedAgreementError,
    krippendorff_alpha,
    label_agreement,
    observed_agreement,
)
from prepeet_ai.evals.dataset import EVALS_ROOT, SUPPORTED_PROFESSIONS
from prepeet_ai.evals.harness import RUBRIC_PATH

CALIBRATION_DIR = EVALS_ROOT / "calibration"
PLAN_PATH = CALIBRATION_DIR / "plan.json"
EXERCISE_PATH = CALIBRATION_DIR / "exercise.json"
BENCHMARK_DIR = CALIBRATION_DIR / "benchmarks"
"""Where a real human benchmark set would live. Empty, which is the point."""

LABELS = ("high", "medium", "low", "not_assessable")
"""ADR-0015's scale. Nominal, not ordered: `not_assessable` is a fact about
the session rather than the bottom rung, and treating it as one would assert
exactly the reading the ADR exists to prevent."""

REQUIRED_PROFESSIONS = SUPPORTED_PROFESSIONS

NO_HUMAN_BENCHMARK_SET = "NO_HUMAN_BENCHMARK_SET"
RATINGS_ARE_NOT_HUMAN = "RATINGS_ARE_NOT_HUMAN"
TOO_FEW_RATERS = "TOO_FEW_RATERS"
TOO_FEW_ITEMS = "TOO_FEW_ITEMS"
PROFESSIONS_NOT_COVERED = "PROFESSIONS_NOT_COVERED"
AGREEMENT_BELOW_FLOOR = "AGREEMENT_BELOW_FLOOR"
AGREEMENT_UNDEFINED = "AGREEMENT_UNDEFINED"
LAWFUL_BASIS_UNDECLARED = "LAWFUL_BASIS_UNDECLARED"
SET_NOT_APPROVED = "SET_NOT_APPROVED"
PLAN_INCOMPLETE = "PLAN_INCOMPLETE"
"""Refusal codes. Named constants rather than sentences, so a test can
assert which refusal fired instead of matching on prose that will be
reworded."""


@dataclass(frozen=True)
class ConfidenceThresholds:
    """One candidate set of confidence rules, in the rubric's own shape.

    `min_supporting` is the rubric's sufficiency floor and belongs here
    because it decides `not_assessable` before any confidence rule runs.
    """

    min_supporting: int
    high_min_supporting: int
    high_max_contradictory: int
    medium_min_supporting: int
    medium_max_contradictory: int

    def problems(self) -> list[str]:
        """Everything that would make this an unpublishable rubric.

        The same three orderings Go's rubric parser enforces. A sweep that
        recommended a candidate failing them would have spent a benchmark
        set arriving at something the registry refuses to publish.
        """
        problems: list[str] = []
        if self.medium_min_supporting < self.min_supporting:
            problems.append("medium confidence is easier to reach than sufficiency")
        if self.high_min_supporting < self.medium_min_supporting:
            problems.append("high confidence is easier to reach than medium")
        if self.high_max_contradictory > self.medium_max_contradictory:
            problems.append("high confidence tolerates more contradiction than medium")
        return problems

    def as_dict(self) -> dict[str, int]:
        """The rule as plain data, for a report or a rubric body."""
        return {
            "min_supporting": self.min_supporting,
            "high_min_supporting": self.high_min_supporting,
            "high_max_contradictory": self.high_max_contradictory,
            "medium_min_supporting": self.medium_min_supporting,
            "medium_max_contradictory": self.medium_max_contradictory,
        }


@dataclass(frozen=True)
class CalibrationPlan:
    """The declared terms a benchmark set has to meet to be usable.

    Every floor is read from `plan.json` rather than written here, so
    changing what would count as calibrated is a reviewable diff in a
    document with an owner, not a constant somebody adjusted.
    """

    owner: str
    review_date: str
    minimum_raters: int
    minimum_items: int
    required_professions: tuple[str, ...]
    agreement_metric: str
    minimum_agreement: float
    minimum_agreement_source: str
    approved_sets: tuple[str, ...]


@dataclass(frozen=True)
class SweepRow:
    """One candidate threshold and how well it matched the consensus."""

    thresholds: ConfidenceThresholds
    agreement: LabelAgreement


@dataclass(frozen=True)
class ThresholdRecord:
    """What a completed calibration would publish.

    All three of QUA-03's criteria are fields here rather than prose: the
    thresholds came from a sweep against consensus labels, the inter-rater
    agreement is carried beside them, and an owner and a review date are
    required to construct the record at all.
    """

    thresholds: ConfidenceThresholds
    benchmark_set_id: str
    inter_rater_agreement: Mapping[str, float]
    agreement_with_consensus: float
    items_compared: int
    sweep: tuple[SweepRow, ...]
    owner: str
    review_date: str


@dataclass(frozen=True)
class CalibrationOutcome:
    """Whether the thresholds are calibrated, and if not, exactly why not."""

    calibrated: bool
    refusals: tuple[str, ...]
    measurements: Mapping[str, Any]
    thresholds: ThresholdRecord | None


def _document(path: pathlib.Path) -> dict[str, Any]:
    loaded: dict[str, Any] = json.loads(path.read_text())
    return loaded


def plan_problems(document: Mapping[str, Any]) -> list[str]:
    """Everything missing from a calibration plan.

    A plan with no agreement floor would let any measured agreement pass,
    and a plan with no owner produces a threshold nobody has to defend.
    Both are checked here rather than trusted, because this file is the
    only thing standing between a sweep and a number somebody liked.
    """
    problems: list[str] = []
    owner = document.get("owner") or {}
    if not owner.get("team") and not owner.get("role"):
        problems.append("owner: the plan names nobody accountable for the calibration")
    review = document.get("review") or {}
    if not review.get("date"):
        problems.append("review: the plan carries no review date")
    if not review.get("owner"):
        problems.append("review: the plan carries no review owner")
    floors = document.get("acceptance_floors") or {}
    for field in (
        "minimum_independent_raters",
        "minimum_items",
        "professions_required",
        "agreement_metric",
        "minimum_agreement",
        "minimum_agreement_source",
    ):
        if field not in floors:
            problems.append(f"acceptance_floors: {field} is not declared")
    return problems


def load_plan(path: pathlib.Path | None = None) -> CalibrationPlan:
    """The calibration plan, refusing to load one that is incomplete."""
    document = _document(path or PLAN_PATH)
    problems = plan_problems(document)
    if problems:
        raise ValueError(f"{PLAN_INCOMPLETE}: {'; '.join(problems)}")
    floors = document["acceptance_floors"]
    owner = document["owner"]
    return CalibrationPlan(
        owner=owner.get("team") or owner["role"],
        review_date=document["review"]["date"],
        minimum_raters=int(floors["minimum_independent_raters"]),
        minimum_items=int(floors["minimum_items"]),
        required_professions=tuple(floors["professions_required"]),
        agreement_metric=str(floors["agreement_metric"]),
        minimum_agreement=float(floors["minimum_agreement"]),
        minimum_agreement_source=str(floors["minimum_agreement_source"]),
        approved_sets=tuple(document.get("approved_benchmark_sets", ())),
    )


def rubric_thresholds(path: pathlib.Path | None = None) -> ConfidenceThresholds:
    """The thresholds currently in force, read from the pinned rubric.

    Read rather than restated, so this module cannot drift from the
    artifact the product is actually evaluating against.
    """
    body = _document(path or RUBRIC_PATH)["body"]
    return ConfidenceThresholds(
        min_supporting=int(body["sufficiency"]["min_supporting"]),
        high_min_supporting=int(body["confidence"]["high"]["min_supporting"]),
        high_max_contradictory=int(body["confidence"]["high"]["max_contradictory"]),
        medium_min_supporting=int(body["confidence"]["medium"]["min_supporting"]),
        medium_max_contradictory=int(body["confidence"]["medium"]["max_contradictory"]),
    )


def derive_confidence(supporting: int, contradictory: int, thresholds: ConfidenceThresholds) -> str:
    """The label a competency's evidence counts earn under these thresholds.

    Mirrors Go's aggregation exactly, including its order: sufficiency
    decides `not_assessable` before any confidence rule is consulted, so a
    competency with one supporting span is unassessable rather than low.
    """
    if supporting < thresholds.min_supporting:
        return "not_assessable"
    if (
        supporting >= thresholds.high_min_supporting
        and contradictory <= thresholds.high_max_contradictory
    ):
        return "high"
    if (
        supporting >= thresholds.medium_min_supporting
        and contradictory <= thresholds.medium_max_contradictory
    ):
        return "medium"
    return "low"


def evidence_counts(case: Mapping[str, Any]) -> dict[str, tuple[int, int]]:
    """Supporting and contradictory span counts per competency, from one case.

    A span naming a competency the case does not carry is ignored rather
    than counted, because counting it would file evidence under a
    competency nobody was rated on.
    """
    counts: dict[str, tuple[int, int]] = dict.fromkeys(case["sufficiency"], (0, 0))
    for span in case["evidence_spans"]:
        competency = span["competency_id"]
        if competency not in counts:
            continue
        supporting, contradictory = counts[competency]
        if span["kind"] == "supporting":
            supporting += 1
        elif span["kind"] == "contradictory":
            contradictory += 1
        counts[competency] = (supporting, contradictory)
    return counts


def machine_labels(report: Mapping[str, Any], thresholds: ConfidenceThresholds) -> dict[str, str]:
    """The confidence label the pipeline would give every rated item.

    Keyed `case_id::competency_id`, which is the unit the plan says a
    human would rate. Derived from the committed report rather than from a
    fresh run, so a sweep and the report it is quoted beside describe the
    same evidence.
    """
    labels: dict[str, str] = {}
    for case in report["cases"]:
        for competency, (supporting, contradictory) in evidence_counts(case).items():
            item = f"{case['case_id']}::{competency}"
            labels[item] = derive_confidence(supporting, contradictory, thresholds)
    return labels


def candidate_thresholds(min_supporting: int, ceiling: int = 6) -> tuple[ConfidenceThresholds, ...]:
    """Every threshold a sweep should consider, at one sufficiency floor.

    Only candidates that would still be a valid rubric are generated. The
    ceiling is deliberately low: the fixtures produce single-figure
    evidence counts, and sweeping to a bound no case can reach would pad
    the sweep with candidates that are all equally wrong.
    """
    candidates: list[ConfidenceThresholds] = []
    for high_min, medium_min in itertools.product(range(min_supporting, ceiling + 1), repeat=2):
        for high_max, medium_max in itertools.product(range(0, 3), repeat=2):
            candidate = ConfidenceThresholds(
                min_supporting=min_supporting,
                high_min_supporting=high_min,
                high_max_contradictory=high_max,
                medium_min_supporting=medium_min,
                medium_max_contradictory=medium_max,
            )
            if not candidate.problems():
                candidates.append(candidate)
    return tuple(candidates)


def sweep(
    report: Mapping[str, Any],
    consensus: Mapping[str, str],
    candidates: Sequence[ConfidenceThresholds],
) -> tuple[SweepRow, ...]:
    """Score every candidate threshold against the consensus labels.

    Every candidate is returned, ranked, not only the winner. A record
    that shows one number hides how close the runners-up were, and two
    thresholds a hair apart on 78 items are not distinguishable evidence.
    """
    rows = [
        SweepRow(
            thresholds=candidate,
            agreement=label_agreement(consensus, machine_labels(report, candidate)),
        )
        for candidate in candidates
    ]
    return tuple(
        sorted(
            rows,
            key=lambda row: (-row.agreement.rate, row.thresholds.as_dict()["high_min_supporting"]),
        )
    )


def load_exercise(path: pathlib.Path | None = None) -> dict[str, Any]:
    """The synthetic exercise specification. Rules, never stored labels."""
    return _document(path or EXERCISE_PATH)


def exercise_ratings(
    report: Mapping[str, Any], exercise: Mapping[str, Any] | None = None
) -> tuple[Rating, ...]:
    """Materialise the exercise's ratings by running its declared rules.

    Nothing is read from a file of labels, because a file of labels is
    indistinguishable on inspection from a file of human ratings. Running
    the rule keeps the fabrication impossible to mistake for data.
    """
    specification = exercise if exercise is not None else load_exercise()
    ratings: list[Rating] = []
    for rater in specification["raters"]:
        thresholds = ConfidenceThresholds(**rater["rule"])
        labels = machine_labels(report, thresholds)
        drift = rater.get("drift")
        for index, item in enumerate(sorted(labels)):
            label = labels[item]
            if drift and (index + 1) % int(drift["every_nth_item"]) == 0:
                label = drift["shift_map"].get(label, label)
            ratings.append(Rating(item_id=item, rater_id=rater["id"], label=label))
    return tuple(ratings)


def run_exercise(report: Mapping[str, Any]) -> dict[str, Any]:
    """Run the agreement arithmetic over the exercise set, stamped as an exercise.

    Every figure here is labelled `exercise_only` so that a number lifted
    out of a report cannot be read as a calibration. The exercise proves
    the machinery runs; it says nothing whatever about people.
    """
    specification = load_exercise()
    ratings = exercise_ratings(report, specification)
    return {
        "exercise_only": True,
        "provenance": specification["rater_provenance"],
        "set_id": specification["set_id"],
        "note": specification["provenance_note"],
        "raters": len(specification["raters"]),
        "items": len({rating.item_id for rating in ratings}),
        "observed_agreement": observed_agreement(ratings),
        "krippendorff_alpha": krippendorff_alpha(ratings),
    }


def _ratings_of(document: Mapping[str, Any]) -> tuple[Rating, ...]:
    return tuple(
        Rating(item_id=item["item_id"], rater_id=rating["rater_id"], label=rating["label"])
        for item in document["items"]
        for rating in item["ratings"]
    )


def benchmark_problems(document: Mapping[str, Any], plan: CalibrationPlan) -> list[str]:
    """Every reason this rating set cannot be calibrated against.

    Provenance is checked alongside the floors rather than instead of
    them, so a refusal says whether the set was rejected for what it is or
    for what it contains. A synthetic set that satisfies every structural
    floor is still refused, and the refusal names provenance.
    """
    problems: list[str] = []
    if document.get("rater_provenance") != "human":
        problems.append(
            f"{RATINGS_ARE_NOT_HUMAN}: provenance is "
            f"{document.get('rater_provenance')!r}, and a threshold derived from labels "
            "no person produced would be calibrated against nothing"
        )

    collection = document.get("collection_record") or {}
    if not collection.get("lawful_basis"):
        problems.append(
            f"{LAWFUL_BASIS_UNDECLARED}: ratings are judgements by identified people and "
            "the set declares no lawful basis for holding them"
        )

    raters = {
        rating["rater_id"] for item in document.get("items", ()) for rating in item["ratings"]
    }
    if len(raters) < plan.minimum_raters:
        problems.append(
            f"{TOO_FEW_RATERS}: {len(raters)} raters against the plan's {plan.minimum_raters}"
        )

    items = document.get("items", ())
    if len(items) < plan.minimum_items:
        problems.append(
            f"{TOO_FEW_ITEMS}: {len(items)} items against the plan's {plan.minimum_items}"
        )

    covered = {item.get("profession") for item in items}
    missing = sorted(set(plan.required_professions) - covered)
    if missing:
        problems.append(
            f"{PROFESSIONS_NOT_COVERED}: nothing rated for {missing}, so a threshold from "
            "this set would be applied to professions it was never measured on"
        )

    if document.get("set_id") not in plan.approved_sets:
        problems.append(
            f"{SET_NOT_APPROVED}: {document.get('set_id')!r} is not named in the plan's "
            "approved_benchmark_sets, and admitting a set is the plan owner's act"
        )

    try:
        measured = krippendorff_alpha(_ratings_of(document))
    except UndefinedAgreementError as undefined:
        problems.append(f"{AGREEMENT_UNDEFINED}: {undefined}")
    else:
        if measured < plan.minimum_agreement:
            problems.append(
                f"{AGREEMENT_BELOW_FLOOR}: {plan.agreement_metric} is {measured:.4f} against "
                f"the plan's floor of {plan.minimum_agreement}"
            )
    return problems


def load_benchmark_sets(directory: pathlib.Path | None = None) -> tuple[dict[str, Any], ...]:
    """Every rating set on disk that claims to carry human judgements."""
    root = directory or BENCHMARK_DIR
    if not root.exists():
        return ()
    return tuple(_document(path) for path in sorted(root.glob("*.json")))


def threshold_record(
    report: Mapping[str, Any],
    ratings: Iterable[Rating],
    consensus: Mapping[str, str],
    plan: CalibrationPlan,
    benchmark_set_id: str,
) -> ThresholdRecord:
    """Build the record a completed calibration publishes.

    The winner of the sweep, the agreement it was chosen against, the
    inter-rater agreement of the set it was chosen from, and the owner and
    review date it inherits from the plan. Constructing it without all of
    those is impossible, which is QUA-03's third criterion expressed as a
    type rather than as a reminder.
    """
    materialised = tuple(ratings)
    candidates = candidate_thresholds(rubric_thresholds().min_supporting)
    rows = sweep(report, consensus, candidates)
    best = rows[0]
    return ThresholdRecord(
        thresholds=best.thresholds,
        benchmark_set_id=benchmark_set_id,
        inter_rater_agreement={
            "observed_agreement": observed_agreement(materialised),
            "krippendorff_alpha": krippendorff_alpha(materialised),
        },
        agreement_with_consensus=best.agreement.rate,
        items_compared=best.agreement.compared,
        sweep=rows,
        owner=plan.owner,
        review_date=plan.review_date,
    )


def calibrate(
    report: Mapping[str, Any] | None = None,
    plan: CalibrationPlan | None = None,
    sets: Sequence[Mapping[str, Any]] | None = None,
) -> CalibrationOutcome:
    """Calibrate the confidence thresholds, or say precisely why it cannot.

    Today it cannot: there is no human benchmark set in this repository,
    so the outcome is a refusal with the reason attached and no thresholds
    at all. The measurements it does return are the ones that can be made
    honestly without human data, and the exercise figures inside them are
    stamped as an exercise.
    """
    from prepeet_ai.evals.harness import load_committed_report

    document = report if report is not None else load_committed_report()
    terms = plan if plan is not None else load_plan()
    candidates = sets if sets is not None else load_benchmark_sets()

    measurements: dict[str, Any] = {
        "current_thresholds": rubric_thresholds().as_dict(),
        "current_labels": _label_totals(machine_labels(document, rubric_thresholds())),
        "exercise": run_exercise(document),
        "benchmark_sets_found": len(candidates),
    }

    human = [candidate for candidate in candidates if candidate.get("rater_provenance") == "human"]
    if not human:
        return CalibrationOutcome(
            calibrated=False,
            refusals=(
                f"{NO_HUMAN_BENCHMARK_SET}: no rating set in {BENCHMARK_DIR.name}/ carries "
                "human judgements, so there is nothing to calibrate against. The thresholds "
                "in the pinned rubric remain the crude initial rules ADR-0015 describes.",
            ),
            measurements=measurements,
            thresholds=None,
        )

    refusals: list[str] = []
    for candidate in human:
        refusals.extend(
            f"{candidate.get('set_id')}: {problem}"
            for problem in benchmark_problems(candidate, terms)
        )
    if refusals:
        return CalibrationOutcome(
            calibrated=False,
            refusals=tuple(refusals),
            measurements=measurements,
            thresholds=None,
        )

    from prepeet_ai.evals.agreement import consensus_labels

    admitted = human[0]
    ratings = _ratings_of(admitted)
    record = threshold_record(
        report=document,
        ratings=ratings,
        consensus=consensus_labels(ratings),
        plan=terms,
        benchmark_set_id=str(admitted["set_id"]),
    )
    return CalibrationOutcome(
        calibrated=True, refusals=(), measurements=measurements, thresholds=record
    )


def _label_totals(labels: Mapping[str, str]) -> dict[str, int]:
    totals = dict.fromkeys(LABELS, 0)
    for label in labels.values():
        totals[label] += 1
    return totals


def numeric_confidence_permitted() -> bool:
    """Whether ADR-0015's prohibition on numeric confidence has been lifted.

    False until a calibration actually succeeds. Expressed as a computed
    value rather than as a sentence in a document, so a surface that wants
    to show a percentage has something to ask, and so the day the answer
    changes is the day the data arrived rather than the day somebody
    decided the labels looked ready.
    """
    return calibrate().calibrated
