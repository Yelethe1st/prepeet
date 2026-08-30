"""QUA-04: the publication gate, proved by the publications it refuses.

Every refusal in this file is driven by a deliberately broken record rather
than described in a comment. A gate that has only ever been shown a valid
publication is a formatter: the way to tell the difference is to hand it a
missing report, a stale one, a report about a different artifact, an
approver who is the author, and a rollback plan naming a version that was
never published, and to watch each one be named.

The artifact trees these tests publish into are built in tmp_path. Nothing
here writes to services/intelligence/artifacts, and no publication record in
this repository names a person, because no person has approved anything
through this gate yet.
"""

from __future__ import annotations

import copy
import dataclasses
import hashlib
import json
import pathlib
from typing import Any

import pytest

from prepeet_ai.evals import harness, publication

REPORT = harness.load_committed_report()
POLICY = publication.load_policy()
TODAY = "2026-08-31"


def _digest(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _rubric_body(min_supporting: int = 2) -> dict[str, Any]:
    return {
        "type": "rubric",
        "reference": "rubric/synthetic-test",
        "version": "1.1.0",
        "schema_version": "1.0",
        "body": {
            "sufficiency": {"min_supporting": min_supporting},
            "bands": [{"id": "developing", "min_ratio": 0.0}],
            "confidence": {
                "high": {"min_supporting": 4, "max_contradictory": 0},
                "medium": {"min_supporting": 2, "max_contradictory": 1},
            },
        },
    }


@pytest.fixture
def tree(tmp_path: pathlib.Path) -> pathlib.Path:
    """A synthetic artifact tree with two versions of one rubric.

    Two versions on purpose: a rollback plan can only name a version that
    was actually published, so a tree with one version cannot demonstrate
    the thing QUA-04 asks to see demonstrated.
    """
    root = tmp_path / "artifacts"
    (root / "rubric").mkdir(parents=True)
    previous = _rubric_body(min_supporting=2)
    previous["version"] = "1.0.0"
    (root / "rubric" / "synthetic-test@1.0.0.json").write_text(
        json.dumps(previous, indent=2) + "\n"
    )
    (root / "rubric" / "synthetic-test@1.1.0.json").write_text(
        json.dumps(_rubric_body(min_supporting=3), indent=2) + "\n"
    )
    return root


@pytest.fixture
def record(tree: pathlib.Path) -> dict[str, Any]:
    """A publication record that passes, so each refusal can break one thing."""
    published = tree / "rubric" / "synthetic-test@1.1.0.json"
    return {
        "record_version": "1.0.0",
        "artifact": {
            "file": "rubric/synthetic-test@1.1.0.json",
            "type": "rubric",
            "reference": "rubric/synthetic-test",
            "version": "1.1.0",
            "sha256": _digest(published),
        },
        "change": {
            "material": True,
            "summary": "Constructed by a test. No such rubric is published anywhere.",
        },
        "author": {"id": "person:author", "name": "The author", "kind": "person"},
        "approver": {"id": "person:approver", "name": "The approver", "kind": "person"},
        "approved_on": TODAY,
        "evaluation_report": {
            "path": "evals/reports/latest.json",
            "results_digest": REPORT["results_digest"],
            "governed_digest": REPORT["governed_inputs"]["digest"],
        },
        "rollback": {
            "kind": "previous_version",
            "to_version": "1.0.0",
            "to_sha256": _digest(tree / "rubric" / "synthetic-test@1.0.0.json"),
        },
    }


def _covering_report(tree: pathlib.Path) -> dict[str, Any]:
    """The committed report, rewritten to have run against the synthetic rubric.

    A test double rather than a fixture of its own: the coverage rule is
    that a report vouches only for the bytes it ran against, so a record
    about a rubric this repository does not contain needs a report that
    says it ran against that rubric. The unmodified report is used in the
    test that proves the coverage check bites.
    """
    covering = copy.deepcopy(REPORT)
    covering["governed_inputs"]["rubric"]["sha256"] = _digest(
        tree / "rubric" / "synthetic-test@1.1.0.json"
    )
    return covering


def _refusals(
    record: dict[str, Any],
    tree: pathlib.Path,
    report: dict[str, Any] | None = None,
    today: str = TODAY,
    current_governed_digest: str | None = None,
) -> list[str]:
    document = _covering_report(tree) if report is None else report
    return publication.publication_refusals(
        record,
        artifacts_root=tree,
        report=document,
        policy=POLICY,
        today=today,
        current_governed_digest=(
            current_governed_digest
            if current_governed_digest is not None
            else document["governed_inputs"]["digest"]
        ),
    )


class TestAValidPublicationIsAdmitted:
    """A gate that refuses everything is a gate nobody can use."""

    def test_a_complete_record_passes(self, record: dict[str, Any], tree: pathlib.Path) -> None:
        """Report, approver, rollback plan and matching digests."""
        assert _refusals(record, tree) == []


class TestPublicationIsBlockedWithoutAnEvaluationReport:
    """QUA-04's first criterion, in five separate ways of being wrong."""

    def test_no_report_at_all(self, record: dict[str, Any], tree: pathlib.Path) -> None:
        """The plain case: a material change with nothing attached."""
        del record["evaluation_report"]

        assert any(r.startswith(publication.REPORT_MISSING) for r in _refusals(record, tree))

    def test_a_report_that_is_not_the_committed_one(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """A record may not quote results that no report in the tree produced."""
        record["evaluation_report"]["results_digest"] = "0" * 64

        assert any(
            r.startswith(publication.REPORT_DIGEST_MISMATCH) for r in _refusals(record, tree)
        )

    def test_a_stale_report_produced_before_the_governed_inputs_moved(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """The rubric, the extractor or the dataset changed after the run.

        Modelled by moving the governed digest the gate compares against,
        which is what editing a rubric or bumping the extractor does.
        """
        refusals = _refusals(record, tree, current_governed_digest="1" * 64)

        assert any(r.startswith(publication.REPORT_STALE) for r in refusals)

    def test_a_report_older_than_the_policy_allows(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """An approval cannot be reused indefinitely on one old run."""
        refusals = _refusals(record, tree, today="2026-12-01")

        assert any(r.startswith(publication.REPORT_EXPIRED) for r in refusals)

    def test_a_report_with_no_generation_date(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """An undated report cannot be shown to be current, so it is refused."""
        undated = _covering_report(tree)
        del undated["generated"]

        assert any(
            r.startswith(publication.REPORT_UNDATED)
            for r in _refusals(record, tree, report=undated)
        )

    def test_a_report_that_is_below_a_hard_floor(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """The floors are the harness's own, so this gate cannot loosen them."""
        broken = _covering_report(tree)
        broken["totals"]["unsupported_facts"]["unsupported"] = 3

        assert any(
            r.startswith(publication.REPORT_BELOW_THRESHOLD)
            for r in _refusals(record, tree, report=broken)
        )

    def test_a_report_that_never_ran_against_this_artifact(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """A green report about another rubric is evidence about that rubric."""
        refusals = _refusals(record, tree, report=copy.deepcopy(REPORT))

        assert any(r.startswith(publication.REPORT_DOES_NOT_COVER_THE_ARTIFACT) for r in refusals)

    def test_the_committed_report_covers_the_pinned_rubric_and_policy(self) -> None:
        """The coverage check is not vacuous: it matches the real artifacts."""
        rubric = harness.RUBRIC_PATH
        policy = harness.POLICY_PATH

        assert publication.report_covers("rubric", _digest(rubric), REPORT)
        assert publication.report_covers("policy", _digest(policy), REPORT)
        assert not publication.report_covers("plan", _digest(rubric), REPORT)

    def test_a_consent_artifact_needs_no_report_but_still_needs_an_approver(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """An evaluation report says nothing about consent wording.

        Exempting it is the honest reading. Requiring a report that cannot
        be evidence would teach people that the report is a formality.
        """
        consent = tree / "consent"
        consent.mkdir()
        body = {"type": "consent_text", "reference": "consent/x", "version": "1.0.0"}
        path = consent / "x@1.0.0.json"
        path.write_text(json.dumps(body) + "\n")
        record["artifact"] = {
            "file": "consent/x@1.0.0.json",
            "type": "consent",
            "reference": "consent/x",
            "version": "1.0.0",
            "sha256": _digest(path),
        }
        record["rollback"] = {"kind": "withdraw", "why": "First version of this reference."}
        del record["evaluation_report"]

        assert _refusals(record, tree) == []

        record["approver"] = record["author"]
        assert any(
            r.startswith(publication.APPROVER_IS_THE_AUTHOR) for r in _refusals(record, tree)
        )


class TestTheApproverIsANamedPersonWhoIsNotTheAuthor:
    """QUA-04's second criterion, and ADR-0011's stricter version of it."""

    def test_the_author_cannot_approve_their_own_change(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """ADR-0011 makes this structural, for every artifact and not only material ones."""
        record["approver"] = dict(record["author"])

        assert any(
            r.startswith(publication.APPROVER_IS_THE_AUTHOR) for r in _refusals(record, tree)
        )

    def test_an_unnamed_approver_is_no_approver(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """A record with an empty name records nobody."""
        record["approver"] = {"id": "person:approver", "name": "", "kind": "person"}

        assert any(r.startswith(publication.APPROVER_MISSING) for r in _refusals(record, tree))

    def test_a_service_principal_cannot_approve_a_material_change(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """The contentctl publisher account satisfies the registry, not the criterion.

        This is the gap the publication record exists to close: the two
        principals the loader uses are two accounts, and neither of them is
        a person who read the change.
        """
        record["approver"] = {
            "id": "service:contentctl-publisher",
            "name": "contentctl publisher",
            "kind": "service",
        }

        assert any(
            r.startswith(publication.APPROVER_IS_A_SERVICE_PRINCIPAL)
            for r in _refusals(record, tree)
        )


class TestRollbackIsDemonstratedRatherThanAssumed:
    """QUA-04's third criterion. The plan resolves, and the resolution is run."""

    def test_a_material_change_with_no_rollback_plan_is_refused(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """A change to how candidates are evaluated with no way back."""
        del record["rollback"]

        assert any(r.startswith(publication.ROLLBACK_PLAN_MISSING) for r in _refusals(record, tree))

    def test_a_plan_naming_a_version_that_was_never_published(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """Rollback is a pointer move, so the target has to exist."""
        record["rollback"]["to_version"] = "0.9.0"

        assert any(
            r.startswith(publication.ROLLBACK_TARGET_UNRESOLVABLE) for r in _refusals(record, tree)
        )

    def test_a_plan_whose_target_has_been_edited_since(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """An edited previous version is not the version that was published."""
        target = tree / "rubric" / "synthetic-test@1.0.0.json"
        body = json.loads(target.read_text())
        body["body"]["sufficiency"]["min_supporting"] = 5
        target.write_text(json.dumps(body, indent=2) + "\n")

        assert any(
            r.startswith(publication.ROLLBACK_TARGET_UNRESOLVABLE) for r in _refusals(record, tree)
        )

    def test_withdraw_is_only_available_to_a_reference_with_one_version(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """A second version cannot roll back by pretending it is the first."""
        record["rollback"] = {"kind": "withdraw", "why": "Claimed to be the first version."}

        assert any(
            r.startswith(publication.ROLLBACK_WITHDRAW_UNAVAILABLE) for r in _refusals(record, tree)
        )

    def test_the_rollback_runs_and_lands_on_byte_identical_content(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """Demonstrated: the plan is executed and the bytes are compared.

        This is the half of ADR-0011's rollback that lives in the authoring
        tree. The pointer move, the deprecation and the immutability
        trigger are the registry's and are tested there.
        """
        before = (tree / "rubric" / "synthetic-test@1.0.0.json").read_bytes()

        outcome = publication.roll_back(record, artifacts_root=tree)

        assert outcome.to_version == "1.0.0"
        assert outcome.content == before
        assert outcome.digest == hashlib.sha256(before).hexdigest()
        assert outcome.problems == ()

    def test_a_rollback_that_cannot_land_says_so_instead_of_pretending(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """The demonstration has to be able to fail, or it demonstrates nothing."""
        (tree / "rubric" / "synthetic-test@1.0.0.json").unlink()

        outcome = publication.roll_back(record, artifacts_root=tree)

        assert outcome.problems
        assert outcome.content is None


class TestTheArtifactTreeIsHeldToTheGate:
    """Where the gate actually bites in this repository today."""

    def test_every_artifact_in_the_tree_is_recorded_or_declared_pre_gate(self) -> None:
        """A new or edited artifact with no publication record fails here.

        This is what makes the policy's pre-gate list a floor rather than
        an amnesty: the entries are digests, so editing one of the listed
        files is as much a failure as adding an unlisted one.
        """
        assert publication.unrecorded_artifacts() == []

    def test_the_record_must_match_the_bytes_it_claims_to_publish(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """A record wearing an old digest is a record about a different file."""
        record["artifact"]["sha256"] = "2" * 64

        assert any(
            r.startswith(publication.ARTIFACT_DIGEST_MISMATCH) for r in _refusals(record, tree)
        )

    def test_a_record_for_a_file_that_is_not_there(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """Publishing a file nobody committed."""
        report = _covering_report(tree)
        (tree / "rubric" / "synthetic-test@1.1.0.json").unlink()

        refusals = _refusals(record, tree, report=report)

        assert any(r.startswith(publication.ARTIFACT_MISSING) for r in refusals)

    def test_the_policy_carries_an_owner_and_a_review_date(self) -> None:
        """The age limit and the material list are choices somebody owns."""
        assert POLICY.owner
        assert POLICY.review_date
        assert POLICY.maximum_report_age_days == 30

    def test_the_command_refuses_a_bad_record_with_a_non_zero_exit(
        self, record: dict[str, Any], tree: pathlib.Path, tmp_path: pathlib.Path
    ) -> None:
        """The gate is runnable, and a refusal is an exit code."""
        del record["evaluation_report"]
        path = tmp_path / "record.json"
        path.write_text(json.dumps(record, indent=2) + "\n")
        report_path = tmp_path / "report.json"
        report_path.write_text(json.dumps(_covering_report(tree)) + "\n")

        arguments = [
            str(path),
            "--artifacts-root",
            str(tree),
            "--report",
            str(report_path),
            "--today",
            TODAY,
        ]
        assert publication.main(arguments) == 1

    def test_the_command_admits_a_good_record(
        self, record: dict[str, Any], tree: pathlib.Path, tmp_path: pathlib.Path
    ) -> None:
        """And exits zero, or nobody could ever publish."""
        path = tmp_path / "record.json"
        path.write_text(json.dumps(record, indent=2) + "\n")
        report_path = tmp_path / "report.json"
        report_path.write_text(json.dumps(_covering_report(tree)) + "\n")

        arguments = [
            str(path),
            "--artifacts-root",
            str(tree),
            "--report",
            str(report_path),
            "--today",
            TODAY,
        ]
        assert publication.main(arguments) == 0


class TestTheRemainingRefusalPaths:
    """The branches a valid record never reaches, driven one at a time."""

    def test_a_record_quoting_a_governed_digest_the_report_does_not_carry(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """The record's own claim about the run has to match the run."""
        record["evaluation_report"]["governed_digest"] = "3" * 64

        assert any(r.startswith(publication.REPORT_STALE) for r in _refusals(record, tree))

    def test_a_rollback_plan_that_names_no_version(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """A plan of kind previous_version with no previous version named."""
        record["rollback"] = {"kind": "previous_version"}

        assert any(r.startswith(publication.ROLLBACK_PLAN_MISSING) for r in _refusals(record, tree))

    def test_running_a_withdraw_plan_lands_on_nothing_by_design(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """Withdrawing a first version removes the pointer; there are no bytes."""
        record["rollback"] = {"kind": "withdraw", "why": "First version."}

        outcome = publication.roll_back(record, artifacts_root=tree)

        assert outcome.to_version is None
        assert outcome.content is None
        assert outcome.problems == ()

    def test_running_a_plan_with_no_target_version_reports_the_gap(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """The rollback runner refuses the same plan the gate refuses."""
        record["rollback"] = {"kind": "previous_version"}

        outcome = publication.roll_back(record, artifacts_root=tree)

        assert outcome.problems
        assert outcome.content is None

    def test_running_a_plan_whose_target_has_been_edited(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """The rollback lands, and the runner says these are not the bytes published."""
        target = tree / "rubric" / "synthetic-test@1.0.0.json"
        target.write_text(target.read_text() + "\n")

        outcome = publication.roll_back(record, artifacts_root=tree)

        assert outcome.content is not None
        assert any(
            problem.startswith(publication.ROLLBACK_TARGET_UNRESOLVABLE)
            for problem in outcome.problems
        )

    def test_no_records_directory_is_no_records(self, tmp_path: pathlib.Path) -> None:
        """Absent is empty rather than an error, because today it is absent."""
        assert publication.load_records(tmp_path / "nothing") == ()

    def test_an_artifact_with_no_record_and_no_pre_gate_entry_is_named(
        self, tree: pathlib.Path
    ) -> None:
        """The tree-level gate: a new artifact nobody recorded fails."""
        empty_policy = dataclasses.replace(POLICY, pre_gate={})

        problems = publication.unrecorded_artifacts(
            policy=empty_policy,
            artifacts_root=tree,
            records=(),
            report=_covering_report(tree),
            today=TODAY,
        )

        assert len(problems) == 2
        assert all(problem.startswith(publication.NO_PUBLICATION_RECORD) for problem in problems)

    def test_a_record_that_is_itself_refused_does_not_admit_the_artifact(
        self, record: dict[str, Any], tree: pathlib.Path
    ) -> None:
        """A record with no report is not a record, so the artifact stays unpublishable."""
        del record["evaluation_report"]
        policy = dataclasses.replace(
            POLICY,
            pre_gate={
                "rubric/synthetic-test@1.0.0.json": _digest(
                    tree / "rubric" / "synthetic-test@1.0.0.json"
                )
            },
        )

        problems = publication.unrecorded_artifacts(
            policy=policy,
            artifacts_root=tree,
            records=(record,),
            report=_covering_report(tree),
            today=TODAY,
        )

        assert any(publication.REPORT_MISSING in problem for problem in problems)
