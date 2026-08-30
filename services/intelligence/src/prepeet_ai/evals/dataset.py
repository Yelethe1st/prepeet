"""Loading the evaluation fixtures, and holding their provenance to account.

A fixture file states what was said and what the pipeline is expected to
make of it. Speech timing is NOT stored word by word: a case declares a
speaking rate, a transcript confidence and where the long pauses fall, and
this module materialises the word timings from that. Three thousand
generated word objects would make the fixtures unreadable, and a reviewer
cannot check what they cannot read; the rate and the pause positions are
the part a reviewer actually needs to see.

The manifest is the provenance record. Its digests are checked against the
bytes on disk, so a fixture cannot be edited while the record that says
where it came from stays still.
"""

from __future__ import annotations

import hashlib
import json
import pathlib
from dataclasses import dataclass, field
from typing import Any

EVALS_ROOT = pathlib.Path(__file__).resolve().parents[3] / "evals"
"""The repository's evals directory: datasets in, reports out.

Resolved from this file rather than from the working directory, so the
harness reads the same fixtures whether it is run from the service, from
the repository root, or from pytest.
"""

DATASET_DIR = EVALS_ROOT / "datasets"
REPORT_DIR = EVALS_ROOT / "reports"
MANIFEST_PATH = DATASET_DIR / "manifest.json"

SUPPORTED_PROFESSIONS = (
    "engineering",
    "finance",
    "nursing",
    "product",
    "sales",
    "teaching",
)
"""The disciplines the product claims to serve, per QUA-01."""

CASE_CLASSES = (
    "well_evidenced",
    "insufficient_evidence",
    "contradiction",
    "contradiction_false_positive",
    "unassessable",
)
"""What a case is for. The first is the happy path; the rest are the edges
the specification requires a dataset to cover on purpose."""

UNASSESSABLE_REASONS = (
    "no_candidate_speech",
    "no_word_timing",
    "transcript_confidence_low",
    "insufficient_speech",
)
"""Why a case cannot be assessed. Each has a different remedy, so they are
distinct facts rather than one bucket."""

MANIFEST_REQUIRED_FIELDS = frozenset(
    {
        "manifest_version",
        "source_status",
        "consent",
        "legal_basis",
        "licensing",
        "de_identification",
        "splits",
        "expected_behaviour",
        "known_limitations",
        "access",
        "retention",
        "owners",
        "review",
        "datasets",
    }
)
"""Exactly what evaluation-system.md says a dataset manifest must describe,
plus the licensing QUA-01 asks for and a review date with an owner."""

INTER_TURN_GAP_MS = 400
"""Silence between one speaker stopping and the next starting."""

WORD_GAP_MS = 80
"""The ordinary gap between two words, inside one turn."""

LONG_PAUSE_MS = 900
"""What a case means by a long pause: comfortably past the 700 ms the
delivery calculator counts, so a fixture's intent does not sit on the
boundary of the thing it is exercising."""


@dataclass(frozen=True)
class ExpectedEvidence:
    """One span the pipeline is expected to produce, and where."""

    competency_id: str
    kind: str
    quote_contains: str


@dataclass(frozen=True)
class ExpectedContradiction:
    """A pair the pipeline is expected to raise, genuine or not.

    `genuine` is the honest half: evidence-1 pairs statements by shared
    subject tokens and disjoint numbers, which catches real conflicts and
    also pairs two unrelated measurements about one subject. A case that
    expects a false positive says so, and the harness counts them rather
    than letting them pass as successes.
    """

    topic_includes: tuple[str, ...]
    genuine: bool
    why: str


@dataclass(frozen=True)
class Expected:
    """Everything a case asserts about the pipeline's reading of it."""

    sufficiency: dict[str, str]
    evidence: tuple[ExpectedEvidence, ...] = ()
    silent_competencies: tuple[str, ...] = ()
    contradictions: tuple[ExpectedContradiction, ...] = ()
    assessability: str = "assessable"
    unassessable_reason: str | None = None


@dataclass(frozen=True)
class Variant:
    """A rephrasing of a case that must not change the judgement.

    Stability under irrelevant phrasing is one of the specification's
    metrics. A variant declares whether the judgement is expected to
    survive it: `expect_same_judgement` false records a known sensitivity
    of the current extractor rather than hiding it.
    """

    id: str
    kind: str
    why: str
    expect_same_judgement: bool
    turns: tuple[dict[str, Any], ...]


@dataclass(frozen=True)
class Case:
    """One transcript and everything expected of it."""

    id: str
    case_class: str
    profession: str
    seniority: str
    notes: str
    turns: tuple[dict[str, Any], ...]
    expected: Expected
    known_limitations: tuple[str, ...] = ()
    variants: tuple[Variant, ...] = ()


@dataclass(frozen=True)
class Dataset:
    """One profession's fixtures."""

    profession: str
    dataset_version: str
    role_shape: str
    competencies: tuple[dict[str, str], ...]
    cases: tuple[Case, ...]
    path: pathlib.Path = field(compare=False, default=pathlib.Path())


def materialise_turns(
    turns: tuple[dict[str, Any], ...] | list[dict[str, Any]],
) -> list[dict[str, Any]]:
    """Turn the readable speech shape into the sealed document's turns.

    Words are laid out at the turn's declared rate, so the clock a span
    resolves to is derived from the same text a reader sees rather than
    from a table nobody reads. The layout is deterministic: the same case
    yields the same milliseconds on every run, which is what lets a report
    be compared across runs at all.
    """
    materialised: list[dict[str, Any]] = []
    clock = 0
    for turn in turns:
        tokens = str(turn["text"]).split()
        rate = int(turn.get("words_per_minute", 150))
        period = max(60_000 // max(rate, 1), WORD_GAP_MS + 40)
        speak_ms = period - WORD_GAP_MS
        long_pause_after = {int(index) for index in turn.get("long_pause_after", ())}

        start = clock
        words: list[dict[str, Any]] = []
        cursor = start
        for index, token in enumerate(tokens):
            words.append(
                {
                    "w": token,
                    "start_ms": cursor,
                    "end_ms": cursor + speak_ms,
                    "confidence": float(turn.get("confidence", 0.95)),
                }
            )
            cursor += speak_ms + WORD_GAP_MS
            if index in long_pause_after:
                cursor += LONG_PAUSE_MS
        end = words[-1]["end_ms"] if words else start + period

        record: dict[str, Any] = {
            "sequence": int(turn["sequence"]),
            "speaker": str(turn["speaker"]),
            "text": str(turn["text"]),
            "start_ms": start,
            "end_ms": end,
        }
        # Word timing rides only the candidate's turns, which is what the
        # sealed document carries: the interviewer's audio is context, and
        # nothing measures it. A case exercising NO_WORD_TIMING withholds
        # it deliberately, and says so in its expectation.
        if turn["speaker"] == "candidate" and not turn.get("no_word_timing", False):
            record["words"] = words
        materialised.append(record)
        clock = end + INTER_TURN_GAP_MS
    return materialised


def _expected(raw: dict[str, Any]) -> Expected:
    return Expected(
        sufficiency=dict(raw["sufficiency"]),
        evidence=tuple(
            ExpectedEvidence(
                competency_id=item["competency_id"],
                kind=item["kind"],
                quote_contains=item["quote_contains"],
            )
            for item in raw.get("evidence", ())
        ),
        silent_competencies=tuple(raw.get("silent_competencies", ())),
        contradictions=tuple(
            ExpectedContradiction(
                topic_includes=tuple(item["topic_includes"]),
                genuine=bool(item["genuine"]),
                why=item["why"],
            )
            for item in raw.get("contradictions", ())
        ),
        assessability=raw.get("assessability", "assessable"),
        unassessable_reason=raw.get("unassessable_reason"),
    )


def _case(raw: dict[str, Any], profession: str) -> Case:
    return Case(
        id=raw["id"],
        case_class=raw["case_class"],
        profession=profession,
        seniority=raw.get("seniority", "mid"),
        notes=raw.get("notes", ""),
        turns=tuple(raw["turns"]),
        expected=_expected(raw["expected"]),
        known_limitations=tuple(raw.get("known_limitations", ())),
        variants=tuple(
            Variant(
                id=variant["id"],
                kind=variant["kind"],
                why=variant["why"],
                expect_same_judgement=bool(variant["expect_same_judgement"]),
                turns=tuple(variant["turns"]),
            )
            for variant in raw.get("variants", ())
        ),
    )


def load_datasets() -> tuple[Dataset, ...]:
    """Every profession's fixtures, in a stable order."""
    datasets: list[Dataset] = []
    for path in sorted(DATASET_DIR.glob("*.json")):
        if path.name == MANIFEST_PATH.name:
            continue
        raw = json.loads(path.read_text())
        profession = raw["profession"]
        datasets.append(
            Dataset(
                profession=profession,
                dataset_version=raw["dataset_version"],
                role_shape=raw["role_shape"],
                competencies=tuple(raw["competencies"]),
                cases=tuple(_case(case, profession) for case in raw["cases"]),
                path=path,
            )
        )
    return tuple(datasets)


def load_manifest() -> dict[str, Any]:
    """The provenance record, as written."""
    document: dict[str, Any] = json.loads(MANIFEST_PATH.read_text())
    return document


def file_digest(path: pathlib.Path) -> str:
    """The dataset file's SHA-256, over its exact bytes."""
    return hashlib.sha256(path.read_bytes()).hexdigest()


def manifest_digest_mismatches(directory: pathlib.Path | None = None) -> list[str]:
    """Names every dataset file the manifest no longer describes correctly.

    A list rather than a boolean, because the useful failure message is
    which file moved, not that something did. The directory is a parameter
    so the three failure branches can be exercised against a temporary
    dataset instead of by damaging the real one: a guard whose failure path
    is only ever reached by hand is a guard nobody has checked lately.
    """
    root = directory or DATASET_DIR
    manifest = json.loads((root / MANIFEST_PATH.name).read_text())
    recorded = {entry["file"]: entry for entry in manifest["datasets"]}
    on_disk = {path.name for path in root.glob("*.json")} - {MANIFEST_PATH.name}
    problems: list[str] = []
    for name in sorted(on_disk - set(recorded)):
        problems.append(f"{name} is not listed in the manifest")
    for name in sorted(set(recorded) - on_disk):
        problems.append(f"{name} is listed in the manifest but is not on disk")
    for name in sorted(on_disk & set(recorded)):
        actual = file_digest(root / name)
        if actual != recorded[name]["sha256"]:
            problems.append(
                f"{name} digest is {actual}, the manifest records {recorded[name]['sha256']}"
            )
    return problems


def refresh_manifest() -> list[str]:
    """Rewrite the manifest's per-file facts from the files themselves.

    Only the mechanical half is regenerated: digests, case counts, case
    classes and variant counts. Provenance, consent, licensing and the
    known limitations are prose somebody has to mean, so they are never
    written by a tool. Returns the files whose recorded digest moved.
    """
    manifest = load_manifest()
    previous = {entry["file"]: entry.get("sha256") for entry in manifest["datasets"]}
    entries: list[dict[str, Any]] = []
    moved: list[str] = []
    for path in sorted(DATASET_DIR.glob("*.json")):
        if path.name == MANIFEST_PATH.name:
            continue
        raw = json.loads(path.read_text())
        cases = raw["cases"]
        digest = file_digest(path)
        if previous.get(path.name) != digest:
            moved.append(path.name)
        entries.append(
            {
                "file": path.name,
                "profession": raw["profession"],
                "dataset_version": raw["dataset_version"],
                "role_shape": raw["role_shape"],
                "competencies": len(raw["competencies"]),
                "cases": len(cases),
                "case_classes": sorted({case["case_class"] for case in cases}),
                "variants": sum(len(case.get("variants", ())) for case in cases),
                "sha256": digest,
            }
        )
    manifest["datasets"] = entries
    manifest["totals"] = {
        "professions": len(entries),
        "cases": sum(entry["cases"] for entry in entries),
        "variants": sum(entry["variants"] for entry in entries),
    }
    MANIFEST_PATH.write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n")
    return moved
