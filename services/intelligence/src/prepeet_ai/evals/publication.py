"""QUA-04: no artifact reaches the registry without a report that vouches for it.

ADR-0011 gives the git-authored artifacts in `services/intelligence/artifacts`
a review step (the pull request) and a validating step (CI). This module is
the part of that validating step which asks the question QUA-04 asks: is
there an evaluation report, does it actually cover these bytes, is it still
current, does it meet the floors, is there a named approver who is not the
author, and is there a rollback plan that resolves to something that was
really published.

Six ways a publication is refused, and each one is driven by a deliberately
broken record in the suite rather than described in a comment. A gate that
has only ever seen a valid publication is a formatter.

Two design choices worth the words.

The floors are not restated here. `harness.gate_violations` owns them, so
this policy cannot quietly loosen an absolute the specification already
requires, and a floor that moves moves in one place.

The pre-gate list is digests rather than filenames. Every artifact in the
tree predates this gate and none of them carries a publication record,
because writing one now would mean naming an approver for an approval that
never happened. Listing them by digest means an edit to one of them is as
much a failure as a new file with no record, which is what makes the list a
floor rather than an amnesty.

What this does not do is as important. Nothing in Go's `contentctl` calls
this gate, so the registry would still accept a publication that never
passed here; CI refuses the change first, which blocks the deployment rather
than the insert. Wiring the gate into the publishing tool is Go work. The
registry's own half of ADR-0011, the immutability trigger, the pointer move
and the two-person publish, is enforced and tested in the control plane, and
this module deliberately does not reimplement any of it.
"""

from __future__ import annotations

import argparse
import datetime
import hashlib
import json
import pathlib
from collections.abc import Mapping, Sequence
from dataclasses import dataclass
from typing import Any

from prepeet_ai.evals.dataset import EVALS_ROOT
from prepeet_ai.evals.harness import (
    ARTIFACT_ROOT,
    gate_violations,
    governed_digest,
    load_committed_report,
)

POLICY_PATH = EVALS_ROOT / "publication-policy.json"
RECORDS_DIR = EVALS_ROOT / "publications"
"""Where a publication record lives once a person has approved a change.
Empty today, and the policy's pre-gate list says why."""

ARTIFACT_MISSING = "ARTIFACT_MISSING"
ARTIFACT_DIGEST_MISMATCH = "ARTIFACT_DIGEST_MISMATCH"
REPORT_MISSING = "REPORT_MISSING"
REPORT_DIGEST_MISMATCH = "REPORT_DIGEST_MISMATCH"
REPORT_STALE = "REPORT_STALE"
REPORT_UNDATED = "REPORT_UNDATED"
REPORT_EXPIRED = "REPORT_EXPIRED"
REPORT_BELOW_THRESHOLD = "REPORT_BELOW_THRESHOLD"
REPORT_DOES_NOT_COVER_THE_ARTIFACT = "REPORT_DOES_NOT_COVER_THE_ARTIFACT"
APPROVER_MISSING = "APPROVER_MISSING"
APPROVER_IS_THE_AUTHOR = "APPROVER_IS_THE_AUTHOR"
APPROVER_IS_A_SERVICE_PRINCIPAL = "APPROVER_IS_A_SERVICE_PRINCIPAL"
ROLLBACK_PLAN_MISSING = "ROLLBACK_PLAN_MISSING"
ROLLBACK_TARGET_UNRESOLVABLE = "ROLLBACK_TARGET_UNRESOLVABLE"
ROLLBACK_WITHDRAW_UNAVAILABLE = "ROLLBACK_WITHDRAW_UNAVAILABLE"
NO_PUBLICATION_RECORD = "NO_PUBLICATION_RECORD"
"""Refusal codes, so a test asserts which refusal fired rather than matching
prose that will be reworded."""


@dataclass(frozen=True)
class PublicationPolicy:
    """The declared terms a publication has to meet.

    Read from `publication-policy.json` rather than written here, so the
    age limit and the material-change list are a reviewable diff in a
    document with an owner rather than constants somebody adjusted.
    """

    owner: str
    review_date: str
    maximum_report_age_days: int
    material_change_types: tuple[str, ...]
    report_exempt_types: tuple[str, ...]
    service_principals: tuple[str, ...]
    pre_gate: Mapping[str, str]


@dataclass(frozen=True)
class RollbackOutcome:
    """The result of actually running a rollback plan, not of trusting it."""

    kind: str
    reference: str
    from_version: str
    to_version: str | None
    digest: str | None
    content: bytes | None
    problems: tuple[str, ...]


def _document(path: pathlib.Path) -> dict[str, Any]:
    loaded: dict[str, Any] = json.loads(path.read_text())
    return loaded


def load_policy(path: pathlib.Path | None = None) -> PublicationPolicy:
    """The publication policy, with its owner and review date."""
    document = _document(path or POLICY_PATH)
    gates = document["gates"]
    owner = document["owner"]
    return PublicationPolicy(
        owner=owner.get("team") or owner["role"],
        review_date=document["review"]["date"],
        maximum_report_age_days=int(gates["maximum_report_age_days"]),
        material_change_types=tuple(document["material_change_types"]),
        report_exempt_types=tuple(document["report_exempt_types"]),
        service_principals=tuple(gates["service_principals"]),
        pre_gate={entry["file"]: entry["sha256"] for entry in document["pre_gate_artifacts"]},
    )


def report_covers(artifact_type: str, digest: str, report: Mapping[str, Any]) -> bool:
    """Whether this report was produced against these exact bytes.

    The harness records the pinned rubric and policy by digest in its
    governed inputs, so a report can vouch for those two and nothing else.
    A plan or a catalogue change has no evidence in this report, and saying
    so is the honest answer rather than waving it through.
    """
    inputs = report.get("governed_inputs", {})
    recorded = {
        "rubric": inputs.get("rubric", {}).get("sha256"),
        "policy": inputs.get("policy", {}).get("sha256"),
    }
    return recorded.get(artifact_type) == digest


def _artifact_path(root: pathlib.Path, record: Mapping[str, Any]) -> pathlib.Path:
    return root / str(record["artifact"]["file"])


def _sibling_versions(path: pathlib.Path) -> list[pathlib.Path]:
    """Every version of the same reference sitting beside this one."""
    stem = path.name.split("@")[0]
    return sorted(path.parent.glob(f"{stem}@*.json"))


def _version_path(path: pathlib.Path, version: str) -> pathlib.Path:
    return path.parent / f"{path.name.split('@')[0]}@{version}.json"


def _digest_of(path: pathlib.Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _report_refusals(
    record: Mapping[str, Any],
    report: Mapping[str, Any],
    policy: PublicationPolicy,
    today: str,
    current_governed_digest: str,
) -> list[str]:
    """Everything wrong with the evaluation report this record attached."""
    artifact = record["artifact"]
    artifact_type = str(artifact["type"])
    attached = record.get("evaluation_report")
    if artifact_type in policy.report_exempt_types:
        return []
    if not attached:
        return [
            f"{REPORT_MISSING}: a {artifact_type} change is material and carries no "
            "evaluation report, so nothing shows it evaluates candidates any better"
        ]

    refusals: list[str] = []
    if attached.get("results_digest") != report.get("results_digest"):
        refusals.append(
            f"{REPORT_DIGEST_MISMATCH}: the record quotes results digest "
            f"{attached.get('results_digest')} and the report carries "
            f"{report.get('results_digest')}"
        )
    # Staleness is measured against the governed inputs as they stand now,
    # not against what the record claims. The report's digest covers the
    # extractor, calculator, profile and coaching versions, the model
    # policy, the pinned rubric and policy bodies and the dataset
    # manifest, so a report produced before any of those moved is evidence
    # about a pipeline that no longer exists.
    ran_under = report.get("governed_inputs", {}).get("digest")
    if ran_under != current_governed_digest:
        refusals.append(
            f"{REPORT_STALE}: the report ran under governed digest {ran_under} and the "
            f"inputs now digest to {current_governed_digest}"
        )
    if attached.get("governed_digest") != ran_under:
        refusals.append(
            f"{REPORT_STALE}: the record quotes governed digest "
            f"{attached.get('governed_digest')} against the report's {ran_under}"
        )
    violations = gate_violations(dict(report))
    if violations:
        refusals.append(f"{REPORT_BELOW_THRESHOLD}: {'; '.join(violations)}")
    if not report_covers(artifact_type, str(artifact["sha256"]), report):
        refusals.append(
            f"{REPORT_DOES_NOT_COVER_THE_ARTIFACT}: the report's governed inputs do not "
            f"record a {artifact_type} with digest {artifact['sha256']}, so it is evidence "
            "about some other artifact"
        )

    generated = report.get("generated")
    if not generated:
        refusals.append(
            f"{REPORT_UNDATED}: the report carries no generation date, so its age cannot be checked"
        )
    else:
        age = (
            datetime.date.fromisoformat(today) - datetime.date.fromisoformat(str(generated))
        ).days
        if age > policy.maximum_report_age_days:
            refusals.append(
                f"{REPORT_EXPIRED}: the report is {age} days old against the policy's "
                f"{policy.maximum_report_age_days}"
            )
    return refusals


def _approver_refusals(record: Mapping[str, Any], policy: PublicationPolicy) -> list[str]:
    """Everything wrong with who approved this, per ADR-0011 and QUA-04."""
    refusals: list[str] = []
    author = record.get("author") or {}
    approver = record.get("approver") or {}
    if not approver.get("name") or not approver.get("id"):
        return [
            f"{APPROVER_MISSING}: the record names nobody who approved the change, and an "
            "approval nobody signed is a review that may not have happened"
        ]
    if approver.get("id") == author.get("id"):
        refusals.append(
            f"{APPROVER_IS_THE_AUTHOR}: {approver['id']} drafted and approved the same "
            "change, which ADR-0011 refuses structurally for every artifact"
        )
    material = str(record["artifact"]["type"]) in policy.material_change_types
    if material and approver.get("id") in policy.service_principals:
        refusals.append(
            f"{APPROVER_IS_A_SERVICE_PRINCIPAL}: {approver['id']} is an account rather than "
            "a person, and a material change needs somebody who read it"
        )
    return refusals


def _rollback_refusals(record: Mapping[str, Any], artifacts_root: pathlib.Path) -> list[str]:
    """Everything wrong with the way back from this publication."""
    plan = record.get("rollback")
    if not plan:
        return [
            f"{ROLLBACK_PLAN_MISSING}: nothing says how this publication is undone, and "
            "ADR-0011 makes rollback a pointer move to a version that was published"
        ]
    path = _artifact_path(artifacts_root, record)
    if plan.get("kind") == "withdraw":
        siblings = _sibling_versions(path)
        if len(siblings) > 1:
            return [
                f"{ROLLBACK_WITHDRAW_UNAVAILABLE}: withdrawing the pointer is only a plan "
                f"for a reference's first version, and "
                f"{[sibling.name for sibling in siblings]} already exist"
            ]
        return []

    target_version = plan.get("to_version")
    if not target_version:
        return [f"{ROLLBACK_PLAN_MISSING}: the plan names no version to roll back to"]
    target = _version_path(path, str(target_version))
    if not target.exists():
        return [
            f"{ROLLBACK_TARGET_UNRESOLVABLE}: {target.name} is not in the tree, so the plan "
            "names a version that was never published"
        ]
    recorded = plan.get("to_sha256")
    actual = _digest_of(target)
    if recorded and recorded != actual:
        return [
            f"{ROLLBACK_TARGET_UNRESOLVABLE}: {target.name} now digests to {actual} and the "
            f"plan records {recorded}, so the version rolled back to is not the version "
            "that was published"
        ]
    return []


def publication_refusals(
    record: Mapping[str, Any],
    *,
    artifacts_root: pathlib.Path | None = None,
    report: Mapping[str, Any] | None = None,
    policy: PublicationPolicy | None = None,
    today: str | None = None,
    current_governed_digest: str | None = None,
) -> list[str]:
    """Every reason this publication must not proceed.

    A list rather than a boolean, because the useful failure message is
    which of the six things is wrong. An empty list is the only thing that
    admits a publication.
    """
    root = artifacts_root or ARTIFACT_ROOT
    terms = policy or load_policy()
    document = report if report is not None else load_committed_report()
    when = today or datetime.date.today().isoformat()
    governed = current_governed_digest or governed_digest()

    refusals: list[str] = []
    path = _artifact_path(root, record)
    if not path.exists():
        refusals.append(
            f"{ARTIFACT_MISSING}: {record['artifact']['file']} is not in the artifact tree"
        )
    elif _digest_of(path) != record["artifact"]["sha256"]:
        refusals.append(
            f"{ARTIFACT_DIGEST_MISMATCH}: {record['artifact']['file']} digests to "
            f"{_digest_of(path)} and the record claims {record['artifact']['sha256']}"
        )

    refusals.extend(_report_refusals(record, document, terms, when, governed))
    refusals.extend(_approver_refusals(record, terms))
    refusals.extend(_rollback_refusals(record, root))
    return refusals


def roll_back(
    record: Mapping[str, Any], *, artifacts_root: pathlib.Path | None = None
) -> RollbackOutcome:
    """Run the record's rollback plan and answer what it landed on.

    Demonstrated rather than assumed: the previous version is read from
    the tree and its bytes and digest are returned, so a test can compare
    them with what was published rather than trust that a version number
    resolves. This is the authoring tree's half of ADR-0011's rollback.
    The pointer move, the deprecation and the immutability trigger belong
    to the registry and are enforced there.
    """
    root = artifacts_root or ARTIFACT_ROOT
    plan = record.get("rollback") or {}
    artifact = record["artifact"]
    path = _artifact_path(root, record)
    kind = str(plan.get("kind", "missing"))

    if kind == "withdraw":
        return RollbackOutcome(
            kind=kind,
            reference=str(artifact["reference"]),
            from_version=str(artifact["version"]),
            to_version=None,
            digest=None,
            content=None,
            problems=(),
        )

    target_version = plan.get("to_version")
    if not target_version:
        return RollbackOutcome(
            kind=kind,
            reference=str(artifact["reference"]),
            from_version=str(artifact["version"]),
            to_version=None,
            digest=None,
            content=None,
            problems=(f"{ROLLBACK_PLAN_MISSING}: the plan names no version to roll back to",),
        )

    target = _version_path(path, str(target_version))
    if not target.exists():
        return RollbackOutcome(
            kind=kind,
            reference=str(artifact["reference"]),
            from_version=str(artifact["version"]),
            to_version=str(target_version),
            digest=None,
            content=None,
            problems=(
                f"{ROLLBACK_TARGET_UNRESOLVABLE}: {target.name} is not in the tree, so this "
                "rollback cannot land",
            ),
        )

    content = target.read_bytes()
    digest = hashlib.sha256(content).hexdigest()
    problems: list[str] = []
    recorded = plan.get("to_sha256")
    if recorded and recorded != digest:
        problems.append(
            f"{ROLLBACK_TARGET_UNRESOLVABLE}: {target.name} digests to {digest} and the plan "
            f"records {recorded}"
        )
    return RollbackOutcome(
        kind=kind,
        reference=str(artifact["reference"]),
        from_version=str(artifact["version"]),
        to_version=str(target_version),
        digest=digest,
        content=content,
        problems=tuple(problems),
    )


def load_records(directory: pathlib.Path | None = None) -> tuple[dict[str, Any], ...]:
    """Every publication record on disk, in a stable order."""
    root = directory or RECORDS_DIR
    if not root.exists():
        return ()
    return tuple(_document(path) for path in sorted(root.glob("*.json")))


def unrecorded_artifacts(
    *,
    policy: PublicationPolicy | None = None,
    artifacts_root: pathlib.Path | None = None,
    records: Sequence[Mapping[str, Any]] | None = None,
    report: Mapping[str, Any] | None = None,
    today: str | None = None,
) -> list[str]:
    """Every artifact in the tree that this gate would not let through.

    An artifact passes if the policy lists it by digest as predating the
    gate, or if a publication record covers exactly its bytes and that
    record itself survives the gate. A record that exists but is refused
    does not launder the artifact, which is why the refusals are run here
    rather than only at publication time.
    """
    root = artifacts_root or ARTIFACT_ROOT
    terms = policy or load_policy()
    found = tuple(records) if records is not None else load_records()
    document = report if report is not None else load_committed_report()
    when = today or datetime.date.today().isoformat()

    by_file: dict[str, Mapping[str, Any]] = {
        str(record["artifact"]["file"]): record for record in found
    }
    problems: list[str] = []
    for path in sorted(root.rglob("*.json")):
        relative = str(path.relative_to(root))
        digest = _digest_of(path)
        if terms.pre_gate.get(relative) == digest:
            continue
        record = by_file.get(relative)
        if record is None:
            problems.append(
                f"{NO_PUBLICATION_RECORD}: {relative} has no publication record and its "
                "digest is not the one the policy recorded as predating the gate"
            )
            continue
        problems.extend(
            f"{relative}: {refusal}"
            for refusal in publication_refusals(
                record, artifacts_root=root, report=document, policy=terms, today=when
            )
        )
    return problems


def main(argv: Sequence[str] | None = None) -> int:
    """Run the gate over one publication record and report what it refused."""
    parser = argparse.ArgumentParser(description="Gate one artifact publication.")
    parser.add_argument("record", help="Path to the publication record JSON.")
    parser.add_argument("--artifacts-root", default=None, help="The artifact tree to read.")
    parser.add_argument("--report", default=None, help="The evaluation report to read.")
    parser.add_argument("--today", default=None, help="The publication date, for the age check.")
    arguments = parser.parse_args(argv)

    record = _document(pathlib.Path(arguments.record))
    refusals = publication_refusals(
        record,
        artifacts_root=pathlib.Path(arguments.artifacts_root) if arguments.artifacts_root else None,
        report=_document(pathlib.Path(arguments.report)) if arguments.report else None,
        today=arguments.today,
    )
    reference = record["artifact"]["reference"]
    version = record["artifact"]["version"]
    if not refusals:
        print(f"admitted {reference}@{version}")
        return 0
    print(f"REFUSED {reference}@{version}")
    for refusal in refusals:
        print(f"  {refusal}")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
