"""QUA-03: the calibration harness, and the calibration it refuses to claim.

The ticket asks for thresholds derived from human agreement data. There are
no human ratings in this repository, so the honest deliverable is the
machinery plus a refusal that cannot be talked around, and these tests hold
both. The refusal is the important half: a calibrator that would accept
invented ratings is worse than no calibrator, because its output would carry
the word calibrated.

Not one rating in this file or in the fixtures it loads came from a person.
The exercise set's raters are declared rules, and the one test that drives
the accept branch says in its own docstring that its input is constructed.
"""

from __future__ import annotations

import dataclasses
import json
import pathlib
from typing import Any

import pytest

from prepeet_ai.evals import calibration, harness
from prepeet_ai.evals.agreement import Rating, UndefinedAgreementError

REPORT = harness.load_committed_report()
PLAN = calibration.load_plan()


def _rating_set(
    provenance: str = "human",
    raters: int = 3,
    items: int = 60,
    professions: tuple[str, ...] = calibration.REQUIRED_PROFESSIONS,
    labels: tuple[str, ...] = ("high", "medium", "low"),
    lawful_basis: str | None = "legitimate_interest, recorded in the collection record",
) -> dict[str, Any]:
    """A rating set document of the shape the plan describes.

    Constructed inside the test, never written to the repository, and not a
    benchmark set: nobody rated anything. It exists so the floors can be
    driven one at a time.
    """
    document: dict[str, Any] = {
        "set_id": "constructed-in-a-test",
        "set_version": "1.0.0",
        "rater_provenance": provenance,
        "provenance_note": "Constructed by a test to drive one branch. Not a rating exercise.",
        "collection_record": {
            "lawful_basis": lawful_basis,
            "procedure_reference": "evals/calibration/plan.json",
            "raters": [{"id": f"rater-{index}", "independent": True} for index in range(raters)],
        },
        "items": [],
    }
    if lawful_basis is None:
        del document["collection_record"]["lawful_basis"]
    for index in range(items):
        profession = professions[index % len(professions)] if professions else "engineering"
        document["items"].append(
            {
                "item_id": f"{profession}-case-{index}::competency-{index}",
                "profession": profession,
                "ratings": [
                    {"rater_id": f"rater-{rater}", "label": labels[(index + rater) % len(labels)]}
                    for rater in range(raters)
                ],
            }
        )
    return document


class TestTheRepositoryIsNotCalibrated:
    """The state of the world, asserted so it cannot change quietly."""

    def test_calibration_refuses_because_there_is_no_human_benchmark_data(self) -> None:
        """The first criterion is not met, and the code says which criterion."""
        outcome = calibration.calibrate()

        assert outcome.calibrated is False
        assert any(
            refusal.startswith(calibration.NO_HUMAN_BENCHMARK_SET) for refusal in outcome.refusals
        )

    def test_a_refused_calibration_recommends_no_thresholds_at_all(self) -> None:
        """No number escapes the refusal wearing the word calibrated."""
        outcome = calibration.calibrate()

        assert outcome.thresholds is None

    def test_numeric_confidence_stays_prohibited_while_it_refuses(self) -> None:
        """ADR-0015: no numeric confidence anywhere until QUA-03 calibrates.

        QUA-03 is this ticket and it did not calibrate, so the prohibition
        stands. This test is the mechanical statement of that, so relaxing
        the ADR without the data fails here.
        """
        assert calibration.numeric_confidence_permitted() is False

    def test_no_rating_set_in_the_repository_claims_a_human_rater(self) -> None:
        """A tripwire against fabricated benchmark data arriving quietly.

        If real ratings are ever collected, this test is updated by the
        person who collected them, along with the ticket and the plan. That
        is the point: it makes the claim a deliberate act.
        """
        claimed = [
            path.name
            for path in sorted(calibration.CALIBRATION_DIR.rglob("*.json"))
            if json.loads(path.read_text()).get("rater_provenance") == "human"
        ]

        assert claimed == []

    def test_the_plan_carries_an_owner_and_a_review_date(self) -> None:
        """The third criterion applies to thresholds; the plan carries its own."""
        assert PLAN.owner
        assert PLAN.review_date
        assert calibration.plan_problems(json.loads(calibration.PLAN_PATH.read_text())) == []

    def test_the_plan_quotes_its_agreement_floor_from_a_named_source(self) -> None:
        """A floor invented by the team that wants to pass it is not a floor."""
        assert PLAN.minimum_agreement == 0.8
        assert "Krippendorff" in PLAN.minimum_agreement_source


class TestThePlanIsHeldToItsOwnShape:
    """A plan missing a floor would let a threshold be picked from noise."""

    def test_a_plan_with_no_owner_is_refused(self) -> None:
        """An unowned threshold is one nobody has to defend."""
        document = json.loads(calibration.PLAN_PATH.read_text())
        del document["owner"]

        assert any(problem.startswith("owner") for problem in calibration.plan_problems(document))

    def test_a_plan_with_no_review_date_is_refused(self) -> None:
        """QUA-03's third criterion, enforced rather than described."""
        document = json.loads(calibration.PLAN_PATH.read_text())
        del document["review"]["date"]

        assert any(problem.startswith("review") for problem in calibration.plan_problems(document))

    def test_a_plan_whose_review_has_no_owner_is_refused(self) -> None:
        """A review date nobody owns is a date that passes unnoticed."""
        document = json.loads(calibration.PLAN_PATH.read_text())
        del document["review"]["owner"]

        assert any(problem.startswith("review") for problem in calibration.plan_problems(document))

    def test_a_plan_with_no_agreement_floor_is_refused(self) -> None:
        """Without a declared floor, any measured agreement would pass."""
        document = json.loads(calibration.PLAN_PATH.read_text())
        del document["acceptance_floors"]["minimum_agreement"]

        assert any(
            problem.startswith("acceptance_floors")
            for problem in calibration.plan_problems(document)
        )


class TestTheSyntheticExerciseCannotCalibrate:
    """The machinery runs end to end on rules, and refuses to call it data."""

    def test_the_exercise_produces_ratings_from_declared_rules(self) -> None:
        """Three raters over every competency in the report, computed not stored."""
        ratings = calibration.exercise_ratings(REPORT)

        assert {rating.rater_id for rating in ratings} == {
            "rule-current-rubric",
            "rule-lenient",
            "rule-current-rubric-with-drift",
        }
        items = {rating.item_id for rating in ratings}
        assert len(items) == sum(len(case["sufficiency"]) for case in REPORT["cases"])

    def test_the_exercise_raters_actually_disagree(self) -> None:
        """Agreement measured where every rater applies one rule is one by construction."""
        measured = calibration.run_exercise(REPORT)

        assert measured["observed_agreement"] < 1.0
        assert -1.0 <= measured["krippendorff_alpha"] <= 1.0

    def test_every_number_the_exercise_produces_is_labelled_exercise_only(self) -> None:
        """So a figure lifted out of a report cannot be read as calibration."""
        measured = calibration.run_exercise(REPORT)

        assert measured["exercise_only"] is True
        assert measured["provenance"] == "synthetic_exercise"

    def test_a_set_that_is_not_human_is_refused_however_well_it_scores(self) -> None:
        """Provenance is a separate gate from the floors, and it fires alone.

        The constructed set below satisfies the rater count, the item count
        and the profession coverage. It is still refused, because the labels
        did not come from people, and the structural floors are recorded as
        met so nobody can mistake the refusal for a data-quality problem.
        """
        document = _rating_set(provenance="synthetic_exercise")

        problems = calibration.benchmark_problems(document, PLAN)

        assert any(problem.startswith(calibration.RATINGS_ARE_NOT_HUMAN) for problem in problems)
        assert not any(problem.startswith(calibration.TOO_FEW_RATERS) for problem in problems)
        assert not any(problem.startswith(calibration.TOO_FEW_ITEMS) for problem in problems)
        assert not any(
            problem.startswith(calibration.PROFESSIONS_NOT_COVERED) for problem in problems
        )


class TestTheBenchmarkFloors:
    """Each floor refuses on its own, driven by a constructed document."""

    def test_too_few_independent_raters(self) -> None:
        """Two raters cannot separate a shared standard from a shared mistake."""
        problems = calibration.benchmark_problems(_rating_set(raters=2), PLAN)

        assert any(problem.startswith(calibration.TOO_FEW_RATERS) for problem in problems)

    def test_too_few_items(self) -> None:
        """A threshold resting on a convenient subset is not calibrated."""
        problems = calibration.benchmark_problems(_rating_set(items=10), PLAN)

        assert any(problem.startswith(calibration.TOO_FEW_ITEMS) for problem in problems)

    def test_a_profession_the_benchmark_never_covered(self) -> None:
        """Calibrating on software interviews and applying it to nursing."""
        problems = calibration.benchmark_problems(
            _rating_set(professions=("engineering", "product")), PLAN
        )

        assert any(problem.startswith(calibration.PROFESSIONS_NOT_COVERED) for problem in problems)

    def test_a_set_whose_agreement_is_below_the_declared_floor(self) -> None:
        """Rotating labels give near-chance agreement, which fails the floor."""
        problems = calibration.benchmark_problems(_rating_set(), PLAN)

        assert any(problem.startswith(calibration.AGREEMENT_BELOW_FLOOR) for problem in problems)

    def test_a_set_where_agreement_cannot_be_computed_at_all(self) -> None:
        """One label everywhere makes alpha undefined, which is not agreement."""
        problems = calibration.benchmark_problems(_rating_set(labels=("high",)), PLAN)

        assert any(problem.startswith(calibration.AGREEMENT_UNDEFINED) for problem in problems)

    def test_a_set_with_no_lawful_basis_for_the_ratings(self) -> None:
        """Ratings are judgements by identified people about recorded speech."""
        problems = calibration.benchmark_problems(_rating_set(lawful_basis=None), PLAN)

        assert any(problem.startswith(calibration.LAWFUL_BASIS_UNDECLARED) for problem in problems)

    def test_a_set_the_plan_owner_never_admitted(self) -> None:
        """Dropping a file into benchmarks/ is one act; admitting it is the second."""
        document = _rating_set()
        document["set_id"] = "never-approved"

        problems = calibration.benchmark_problems(document, PLAN)

        assert any(problem.startswith(calibration.SET_NOT_APPROVED) for problem in problems)

    def test_the_gate_can_admit_as_well_as_refuse(self) -> None:
        """The accept branch, driven by a document constructed in this test.

        The input is not a benchmark set and no calibration is derived from
        it anywhere: it exists because a gate never shown to admit anything
        may be refusing for a reason nobody has noticed, and that would only
        be discovered on the day real ratings arrive. Every rating in it is
        generated by the loop above, and the repository contains no file
        like it, which the tripwire test above enforces.
        """
        document = _rating_set()
        # Raters who mostly agree, with one dissent every tenth item so that
        # both labels are in play and alpha is defined rather than perfect.
        for index, item in enumerate(document["items"]):
            settled = "high" if index % 2 else "low"
            for position, rating in enumerate(item["ratings"]):
                dissent = index % 10 == 0 and position == 0
                rating["label"] = "medium" if dissent else settled

        plan = calibration.CalibrationPlan(
            owner=PLAN.owner,
            review_date=PLAN.review_date,
            minimum_raters=PLAN.minimum_raters,
            minimum_items=PLAN.minimum_items,
            required_professions=PLAN.required_professions,
            agreement_metric=PLAN.agreement_metric,
            minimum_agreement=PLAN.minimum_agreement,
            minimum_agreement_source=PLAN.minimum_agreement_source,
            approved_sets=("constructed-in-a-test",),
        )

        assert calibration.benchmark_problems(document, plan) == []


class TestConfidenceDerivationMirrorsThePublishedRule:
    """The sweep is only meaningful if it sweeps the rule the product uses."""

    def test_the_pinned_rubric_supplies_the_current_thresholds(self) -> None:
        """Read from the artifact, never restated as a constant here."""
        thresholds = calibration.rubric_thresholds()

        assert thresholds.min_supporting == 2
        assert thresholds.high_min_supporting == 4
        assert thresholds.high_max_contradictory == 0
        assert thresholds.medium_min_supporting == 2
        assert thresholds.medium_max_contradictory == 1

    @pytest.mark.parametrize(
        ("supporting", "contradictory", "expected"),
        [
            (4, 0, "high"),
            (5, 0, "high"),
            (4, 1, "medium"),
            (3, 0, "medium"),
            (2, 1, "medium"),
            (2, 2, "low"),
            (1, 0, "not_assessable"),
            (0, 3, "not_assessable"),
        ],
    )
    def test_each_branch_of_the_rule(
        self, supporting: int, contradictory: int, expected: str
    ) -> None:
        """Sufficiency first, then high, then medium, then low."""
        thresholds = calibration.rubric_thresholds()

        assert calibration.derive_confidence(supporting, contradictory, thresholds) == expected

    def test_a_candidate_that_would_not_be_a_valid_rubric_is_refused(self) -> None:
        """High cannot be easier than medium, which is the registry's own rule."""
        invalid = calibration.ConfidenceThresholds(
            min_supporting=2,
            high_min_supporting=1,
            high_max_contradictory=0,
            medium_min_supporting=2,
            medium_max_contradictory=1,
        )

        assert invalid.problems()

    def test_medium_may_not_be_easier_to_reach_than_sufficiency(self) -> None:
        """A competency below the sufficiency floor has no confidence at all."""
        invalid = calibration.ConfidenceThresholds(
            min_supporting=2,
            high_min_supporting=4,
            high_max_contradictory=0,
            medium_min_supporting=1,
            medium_max_contradictory=1,
        )

        assert "medium confidence is easier to reach than sufficiency" in invalid.problems()

    def test_high_may_not_be_easier_to_reach_than_medium(self) -> None:
        """The second ordering rule, which the registry also enforces."""
        invalid = calibration.ConfidenceThresholds(
            min_supporting=2,
            high_min_supporting=2,
            high_max_contradictory=0,
            medium_min_supporting=3,
            medium_max_contradictory=1,
        )

        assert "high confidence is easier to reach than medium" in invalid.problems()

    def test_high_may_not_tolerate_more_contradiction_than_medium(self) -> None:
        """The third: a label meaning more evidence cannot mean more conflict."""
        invalid = calibration.ConfidenceThresholds(
            min_supporting=2,
            high_min_supporting=4,
            high_max_contradictory=2,
            medium_min_supporting=2,
            medium_max_contradictory=1,
        )

        assert "high confidence tolerates more contradiction than medium" in invalid.problems()

    def test_every_generated_candidate_would_be_a_valid_rubric(self) -> None:
        """A sweep that recommends an unpublishable rubric has wasted its data."""
        candidates = calibration.candidate_thresholds(min_supporting=2)

        assert candidates
        assert all(candidate.problems() == [] for candidate in candidates)


class TestTheThresholdSweep:
    """What a real benchmark set would be swept against."""

    def test_labels_are_derived_from_the_committed_report(self) -> None:
        """One label per competency per case, keyed case_id::competency_id."""
        labels = calibration.machine_labels(REPORT, calibration.rubric_thresholds())

        expected_items = sum(len(case["sufficiency"]) for case in REPORT["cases"])
        assert len(labels) == expected_items
        assert set(labels.values()) <= set(calibration.LABELS)
        first = REPORT["cases"][0]
        competency = sorted(first["sufficiency"])[0]
        assert f"{first['case_id']}::{competency}" in labels

    def test_moving_the_threshold_moves_the_labels(self) -> None:
        """A sweep over an inert function would rank every candidate equally."""
        strict = calibration.machine_labels(REPORT, calibration.rubric_thresholds())
        lenient = calibration.machine_labels(
            REPORT,
            calibration.ConfidenceThresholds(
                min_supporting=1,
                high_min_supporting=2,
                high_max_contradictory=0,
                medium_min_supporting=1,
                medium_max_contradictory=2,
            ),
        )

        assert strict != lenient

    def test_the_sweep_reports_every_candidate_not_only_the_winner(self) -> None:
        """A winner with no runners-up hides how arbitrary the choice was."""
        consensus = calibration.machine_labels(REPORT, calibration.rubric_thresholds())

        rows = calibration.sweep(REPORT, consensus, calibration.candidate_thresholds(2))

        assert len(rows) == len(calibration.candidate_thresholds(2))
        assert rows[0].agreement.rate >= rows[-1].agreement.rate
        assert rows[0].agreement.rate == pytest.approx(1.0), "the identity rule agrees with itself"

    def test_a_sweep_with_nothing_in_common_refuses_rather_than_scoring_zero(self) -> None:
        """An empty comparison is not a disagreement."""
        with pytest.raises(UndefinedAgreementError):
            calibration.sweep(
                REPORT, {"nothing::in-common": "high"}, calibration.candidate_thresholds(2)
            )


class TestTheCalibrationRecordItWouldProduce:
    """What arrives on the day the data does, so the shape is not improvised."""

    def test_a_recommendation_carries_its_agreement_owner_and_review_date(
        self, tmp_path: pathlib.Path
    ) -> None:
        """QUA-03's three criteria are three fields, and all three are required.

        Driven by the constructed document again: this proves the record's
        shape, not any threshold. The thresholds it names are the ones the
        constructed labels happen to favour and mean nothing about people.
        """
        del tmp_path
        consensus = calibration.machine_labels(REPORT, calibration.rubric_thresholds())
        ratings = [
            Rating(item_id=item, rater_id=f"rater-{index}", label=label)
            for index in range(3)
            for item, label in consensus.items()
        ]

        record = calibration.threshold_record(
            report=REPORT,
            ratings=ratings,
            consensus=consensus,
            plan=PLAN,
            benchmark_set_id="constructed-in-a-test",
        )

        assert record.owner == PLAN.owner
        assert record.review_date == PLAN.review_date
        assert record.benchmark_set_id == "constructed-in-a-test"
        assert record.inter_rater_agreement["krippendorff_alpha"] == pytest.approx(1.0)
        assert len(record.sweep) == len(calibration.candidate_thresholds(2))
        assert record.thresholds.problems() == []


def _constructed_human_set(approved: bool = True) -> dict[str, Any]:
    """A rating set over the report's own items, shaped like a human one.

    Constructed in this file and never written to the repository. Nobody
    rated anything: the labels are the current rubric's own, so the set
    agrees with itself perfectly and the branches past the floors can be
    reached. It exists only to prove the calibrator can admit, and the
    tripwire test above enforces that no file like it exists on disk.
    """
    labels = calibration.machine_labels(REPORT, calibration.rubric_thresholds())
    professions = {case["case_id"]: case["profession"] for case in REPORT["cases"]}
    return {
        "set_id": "constructed-in-a-test" if approved else "never-approved",
        "set_version": "1.0.0",
        "rater_provenance": "human",
        "provenance_note": "Constructed by a test. No person rated anything.",
        "collection_record": {
            "lawful_basis": "constructed for a test, and held nowhere",
            "raters": [{"id": f"rater-{index}", "independent": True} for index in range(3)],
        },
        "items": [
            {
                "item_id": item,
                "profession": professions[item.split("::")[0]],
                "ratings": [{"rater_id": f"rater-{index}", "label": label} for index in range(3)],
            }
            for item, label in labels.items()
        ],
    }


class TestCalibrateItselfCanAdmitAndCanRefuse:
    """The whole path, so the refusal is known to be about the data.

    A calibrator nobody has watched admit anything might be refusing for a
    reason nobody has noticed, and that would only surface on the day real
    ratings arrive. These drive both directions with a constructed set,
    which is not a benchmark and produces no calibration anywhere.
    """

    def test_a_human_set_the_plan_never_admitted_is_still_refused(self) -> None:
        """The second of the two acts: the plan owner has to name the set."""
        outcome = calibration.calibrate(
            report=REPORT, plan=PLAN, sets=[_constructed_human_set(approved=False)]
        )

        assert outcome.calibrated is False
        assert any(calibration.SET_NOT_APPROVED in refusal for refusal in outcome.refusals)
        assert outcome.thresholds is None

    def test_both_acts_together_produce_a_threshold_record(self) -> None:
        """Provenance and admission, and only then does a record exist."""
        plan = dataclasses.replace(PLAN, approved_sets=("constructed-in-a-test",))

        outcome = calibration.calibrate(report=REPORT, plan=plan, sets=[_constructed_human_set()])

        assert outcome.calibrated is True
        assert outcome.refusals == ()
        assert outcome.thresholds is not None
        assert outcome.thresholds.owner == PLAN.owner
        assert outcome.thresholds.review_date == PLAN.review_date
        assert outcome.thresholds.agreement_with_consensus == pytest.approx(1.0)

    def test_the_measurements_are_returned_even_when_it_refuses(self) -> None:
        """A refusal that reports nothing teaches nobody what is missing."""
        outcome = calibration.calibrate()

        assert outcome.measurements["benchmark_sets_found"] == 0
        assert outcome.measurements["exercise"]["exercise_only"] is True
        assert sum(outcome.measurements["current_labels"].values()) == sum(
            len(case["sufficiency"]) for case in REPORT["cases"]
        )


class TestTheLoadersRefuseWhatTheyCannotTrust:
    """Loading is a gate too, and each of its refusals is reachable."""

    def test_an_incomplete_plan_cannot_be_loaded_at_all(self, tmp_path: pathlib.Path) -> None:
        """A plan missing a floor would let any measured agreement pass."""
        document = json.loads(calibration.PLAN_PATH.read_text())
        del document["acceptance_floors"]["minimum_agreement"]
        path = tmp_path / "plan.json"
        path.write_text(json.dumps(document))

        with pytest.raises(ValueError, match=calibration.PLAN_INCOMPLETE):
            calibration.load_plan(path)

    def test_an_absent_benchmark_directory_is_no_benchmark_sets(
        self, tmp_path: pathlib.Path
    ) -> None:
        """Absent is empty rather than an error, because today it is absent."""
        assert calibration.load_benchmark_sets(tmp_path / "nothing") == ()

    def test_a_span_for_a_competency_the_case_does_not_carry_is_ignored(self) -> None:
        """Counting a stray span would put evidence under a competency nobody rated."""
        case = {
            "case_id": "constructed",
            "sufficiency": {"comp-1": "sufficient"},
            "evidence_spans": [
                {"competency_id": "comp-1", "kind": "supporting"},
                {"competency_id": "comp-1", "kind": "contradictory"},
                {"competency_id": "comp-elsewhere", "kind": "supporting"},
            ],
        }

        assert calibration.evidence_counts(case) == {"comp-1": (1, 1)}
