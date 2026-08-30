"""The automated evaluation harness: QUA-02.

Runs the Python evaluation pipeline over every fixture and writes one
versioned report. Three properties are deliberate.

It runs on every governed change because it is part of the test suite, and
because the committed report carries a digest over its governed inputs: the
extractor, profile and coaching versions, the pinned rubric and policy
bodies, and the dataset manifest. Editing a rubric without regenerating the
report fails a named test rather than passing quietly.

Nothing here estimates. The unsupported-fact rate is a count of assertions
whose quoted words are not in the turn they name, decided by string
comparison, and the graders are attacked in the suite so a zero means the
meter works rather than that it cannot move.

Results are comparable because the report is a stable document with a digest
over everything except timing. Latency is recorded beside the digest rather
than inside it: a slower machine is not a regression in the evaluation.

Scope. This harness runs the stages that live in this service: evidence
extraction, contradiction detection, the delivery profile and the coaching
gate, plus the rubric's own sufficiency threshold. Band aggregation is Go's
and is not run here, so the sufficiency rule is read from the pinned rubric
rather than reimplemented from an opinion. A harness spanning both languages
is the honest next step and is not this ticket.
"""

from __future__ import annotations

import hashlib
import json
import pathlib
import socket
import time
from collections.abc import Iterator
from contextlib import contextmanager
from dataclasses import dataclass, field
from typing import Any

from prepeet_ai.articulation.coaching import COACHING_VERSION, UnpreservingError, coaching_document
from prepeet_ai.articulation.features import CALCULATION_VERSION, session_features
from prepeet_ai.articulation.profile import PROFILE_VERSION
from prepeet_ai.articulation.service import POLICY_VERSION
from prepeet_ai.evals.dataset import (
    EVALS_ROOT,
    MANIFEST_PATH,
    Case,
    Dataset,
    load_datasets,
    materialise_turns,
)
from prepeet_ai.evals.metrics import (
    Assertion,
    assertions,
    contradiction_schema_problems,
    evidence_span_schema_problems,
    grounding_problems,
)
from prepeet_ai.evaluation.evidence import (
    EXTRACTION_VERSION,
    extract_contradictions,
    extract_evidence,
)
from prepeet_ai.evaluation.service import SCHEMA_VERSION as EVALUATION_SCHEMA_VERSION

HARNESS_VERSION = "evals-harness-1"
REPORT_SCHEMA_VERSION = "1.0"

REPORT_PATH = EVALS_ROOT / "reports" / "latest.json"
"""The committed artifact. One path, overwritten deliberately, so the
history of the numbers is the repository's history rather than a directory
of files nobody prunes."""

ARTIFACT_ROOT = EVALS_ROOT.parent / "artifacts"

RUBRIC_PATH = ARTIFACT_ROOT / "rubric" / "practice-default@1.1.0.json"
"""Pinned by version on purpose. The harness never resolves the currently
published rubric, for the same reason no stage resolves `latest` after a
session starts: a report has to say which rubric it was produced against."""

POLICY_PATH = ARTIFACT_ROOT / "policy" / "practice-default@1.0.0.json"

_UNASSESSABLE_REASONS = {
    "NO_CANDIDATE_SPEECH": "no_candidate_speech",
    "NO_WORD_TIMING": "no_word_timing",
    "TRANSCRIPT_CONFIDENCE_LOW": "transcript_confidence_low",
    "INSUFFICIENT_SPEECH": "insufficient_speech",
}
"""The calculator's warnings, mapped to the dataset's vocabulary. Each one
has a different remedy, which is why they are not one bucket."""


@dataclass
class ConnectionMeter:
    """Counts outbound connection attempts made while it is installed."""

    connections: int = 0


@contextmanager
def counted_connections() -> Iterator[ConnectionMeter]:
    """Count every socket connection attempted inside the block.

    Cost is reported as zero for this pipeline, and an unmeasured zero is
    a claim rather than a number. This counts rather than blocks: the job
    is to notice a provider call appearing, not to break a run that makes
    one.
    """
    meter = ConnectionMeter()
    original_connect = socket.socket.connect
    original_connect_ex = socket.socket.connect_ex

    def connect(self: Any, address: Any) -> Any:
        meter.connections += 1
        return original_connect(self, address)

    def connect_ex(self: Any, address: Any) -> Any:
        meter.connections += 1
        return original_connect_ex(self, address)

    socket.socket.connect = connect  # type: ignore[method-assign]
    socket.socket.connect_ex = connect_ex  # type: ignore[method-assign]
    try:
        yield meter
    finally:
        socket.socket.connect = original_connect  # type: ignore[method-assign]
        socket.socket.connect_ex = original_connect_ex  # type: ignore[method-assign]


def digest_of(value: Any) -> str:
    """A stable SHA-256 over any JSON-shaped value.

    Keys are sorted and separators fixed, so the digest depends on the
    content and not on how the dictionary happened to be built.
    """
    encoded = json.dumps(value, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(encoded).hexdigest()


def _artifact(path: pathlib.Path) -> dict[str, Any]:
    document: dict[str, Any] = json.loads(path.read_text())
    return document


def governed_inputs() -> dict[str, Any]:
    """Everything a change to which must force a fresh report.

    Prompt, model and rubric are the ticket's three; at this floor the
    prompt and model are the deterministic extractor's own versions, and
    they are recorded under their real names rather than as a placeholder
    that would stop meaning anything when a model arrives.
    """
    rubric = _artifact(RUBRIC_PATH)
    policy = _artifact(POLICY_PATH)
    return {
        "extraction_version": EXTRACTION_VERSION,
        "calculation_version": CALCULATION_VERSION,
        "profile_version": PROFILE_VERSION,
        "coaching_version": COACHING_VERSION,
        "evaluation_schema_version": EVALUATION_SCHEMA_VERSION,
        "articulation_policy_version": POLICY_VERSION,
        "model_policy": "none: evidence-1 consults no model",
        "rubric": {
            "reference": rubric["reference"],
            "version": rubric["version"],
            "sha256": hashlib.sha256(RUBRIC_PATH.read_bytes()).hexdigest(),
        },
        "policy": {
            "reference": policy["reference"],
            "version": policy["version"],
            "sha256": hashlib.sha256(POLICY_PATH.read_bytes()).hexdigest(),
        },
        "dataset_manifest_sha256": hashlib.sha256(MANIFEST_PATH.read_bytes()).hexdigest(),
    }


def governed_digest() -> str:
    """The digest the committed report has to have been produced under."""
    return digest_of(governed_inputs())


def _span_record(span: Any) -> dict[str, Any]:
    return {
        "competency_id": span.competency_id,
        "kind": span.kind,
        "segment_sequence": span.segment_sequence,
        "quote": span.quote,
        "char_start": span.char_start,
        "char_end": span.char_end,
        "start_ms": span.start_ms,
        "end_ms": span.end_ms,
        "extraction_version": span.extraction_version,
    }


def _side_record(side: Any) -> dict[str, Any]:
    return {
        "segment_sequence": side.segment_sequence,
        "quote": side.quote,
        "char_start": side.char_start,
        "char_end": side.char_end,
        "start_ms": side.start_ms,
        "end_ms": side.end_ms,
    }


def _contradiction_record(pair: Any) -> dict[str, Any]:
    return {
        "topic": list(pair.topic),
        "side_a": _side_record(pair.side_a),
        "side_b": _side_record(pair.side_b),
        "extraction_version": pair.extraction_version,
    }


def _sufficiency(
    competencies: tuple[dict[str, str], ...], spans: list[dict[str, Any]], min_supporting: int
) -> dict[str, str]:
    """Per competency: sufficient, insufficient, or never discussed at all.

    The threshold comes from the pinned rubric rather than from a number
    chosen here, so a rubric change moves this and the governed digest
    together. Unassessed is deliberately two facts: nothing was said, or
    something was said and it was not enough.
    """
    outcome: dict[str, str] = {}
    for competency in competencies:
        identifier = competency["id"]
        linked = [span for span in spans if span["competency_id"] == identifier]
        supporting = [span for span in linked if span["kind"] == "supporting"]
        if len(supporting) >= min_supporting:
            outcome[identifier] = "sufficient"
        elif linked:
            outcome[identifier] = "insufficient_evidence"
        else:
            outcome[identifier] = "not_discussed"
    return outcome


def _delivery(turns: list[dict[str, Any]]) -> tuple[dict[str, Any], dict[str, Any] | None]:
    """The delivery status with its reasons, and the coaching if it passed its gate."""
    features = session_features(turns)
    reasons: list[str] = []
    for warning in features.warnings:
        if warning in _UNASSESSABLE_REASONS:
            reasons.append(_UNASSESSABLE_REASONS[warning])
    for turn in features.turns:
        if turn.status == "assessable":
            continue
        for warning in turn.warnings:
            if warning in _UNASSESSABLE_REASONS:
                reasons.append(_UNASSESSABLE_REASONS[warning])

    try:
        coaching = coaching_document(turns)
        withheld = ""
    except UnpreservingError as refused:
        # Coaching that fails fact preservation is an absence with a
        # reason, exactly as the service serves it. The harness records
        # the refusal rather than treating it as a crash, because a
        # refusal is the gate working.
        coaching = None
        withheld = str(refused)

    return (
        {
            "status": features.status,
            "reasons": sorted(set(reasons)),
            "words": features.words,
            "transcript_confidence": features.transcript_confidence,
            "coaching_withheld": withheld,
        },
        coaching,
    )


def _judgement(spans: list[dict[str, Any]]) -> list[list[str]]:
    """What a rephrasing must not change: which competency was read how."""
    return sorted([span["competency_id"], span["kind"]] for span in spans)


@dataclass
class _Counts:
    """Running totals across the whole run."""

    grounding_checked: int = 0
    grounding_ok: int = 0
    assertions: int = 0
    unsupported: int = 0
    schema_records: int = 0
    schema_ok: int = 0
    expectations_checked: int = 0
    expectations_met: int = 0
    contradictions_genuine: int = 0
    contradictions_false: int = 0
    probes: int = 0
    probes_expected_stable: int = 0
    probes_stable: int = 0
    probes_known_sensitivity: int = 0
    failures: list[str] = field(default_factory=list)


def _check_expectations(case: Case, result: dict[str, Any], counts: _Counts) -> None:
    """Hold one case to everything its fixture declared about it."""

    def check(condition: bool, message: str) -> None:
        counts.expectations_checked += 1
        if condition:
            counts.expectations_met += 1
        else:
            counts.failures.append(f"{case.id}: {message}")

    check(
        result["sufficiency"] == case.expected.sufficiency,
        f"sufficiency {result['sufficiency']} is not the declared {case.expected.sufficiency}",
    )
    check(
        result["delivery"]["status"] == case.expected.assessability,
        f"delivery status {result['delivery']['status']!r} is not the declared "
        f"{case.expected.assessability!r}",
    )
    if case.expected.unassessable_reason is not None:
        check(
            case.expected.unassessable_reason in result["delivery"]["reasons"],
            f"delivery reasons {result['delivery']['reasons']} do not include the declared "
            f"{case.expected.unassessable_reason!r}",
        )
    for wanted in case.expected.evidence:
        check(
            any(
                span["competency_id"] == wanted.competency_id
                and span["kind"] == wanted.kind
                and wanted.quote_contains in span["quote"]
                for span in result["evidence_spans"]
            ),
            f"no {wanted.kind} span for {wanted.competency_id} quoting {wanted.quote_contains!r}",
        )
    for silent in case.expected.silent_competencies:
        check(
            not any(span["competency_id"] == silent for span in result["evidence_spans"]),
            f"{silent} was declared silent but produced a span",
        )
    check(
        len(result["contradictions"]) == len(case.expected.contradictions),
        f"{len(result['contradictions'])} contradictions against the declared "
        f"{len(case.expected.contradictions)}",
    )
    for wanted_pair in case.expected.contradictions:
        check(
            any(
                set(wanted_pair.topic_includes) <= set(pair["topic"])
                for pair in result["contradictions"]
            ),
            f"no contradiction whose topic includes {list(wanted_pair.topic_includes)}",
        )


def _run_case(case: Case, dataset: Dataset, min_supporting: int, counts: _Counts) -> dict[str, Any]:
    """One case through the pipeline, measured on the way out."""
    turns = materialise_turns(case.turns)
    spans = [
        _span_record(span)
        for span in extract_evidence(turns, [dict(c) for c in dataset.competencies])
    ]
    pairs = [_contradiction_record(pair) for pair in extract_contradictions(turns)]
    delivery, coaching = _delivery(turns)

    for span in spans:
        counts.grounding_checked += 1
        if not grounding_problems(turns, span):
            counts.grounding_ok += 1
    for pair in pairs:
        for side in ("side_a", "side_b"):
            counts.grounding_checked += 1
            if not grounding_problems(turns, pair[side]):
                counts.grounding_ok += 1

    schema_problems: list[str] = []
    for span in spans:
        counts.schema_records += 1
        problems = evidence_span_schema_problems(span)
        counts.schema_ok += 0 if problems else 1
        schema_problems.extend(f"span: {problem}" for problem in problems)
    for pair in pairs:
        counts.schema_records += 1
        problems = contradiction_schema_problems(pair)
        counts.schema_ok += 0 if problems else 1
        schema_problems.extend(f"contradiction: {problem}" for problem in problems)

    measured: list[Assertion] = assertions(turns, spans, pairs, coaching)
    unsupported = [a for a in measured if not a.supported]
    counts.assertions += len(measured)
    counts.unsupported += len(unsupported)

    declared_genuine = [pair.genuine for pair in case.expected.contradictions]
    counts.contradictions_genuine += sum(1 for genuine in declared_genuine if genuine)
    counts.contradictions_false += sum(1 for genuine in declared_genuine if not genuine)

    shape = list((coaching or {}).get("suggested_shape", []))
    result: dict[str, Any] = {
        "case_id": case.id,
        "profession": case.profession,
        "case_class": case.case_class,
        "seniority": case.seniority,
        "evidence_spans": spans,
        "contradictions": pairs,
        "sufficiency": _sufficiency(dataset.competencies, spans, min_supporting),
        "delivery": delivery,
        "coaching_shape": [
            {"slot": part["slot"], "kind": part["kind"], "sequence": part["sequence"]}
            for part in shape
        ],
        "grounding": {
            "checked": len(spans) + 2 * len(pairs),
            "grounded": len(spans)
            + 2 * len(pairs)
            - sum(1 for span in spans if grounding_problems(turns, span))
            - sum(
                1
                for pair in pairs
                for side in ("side_a", "side_b")
                if grounding_problems(turns, pair[side])
            ),
        },
        "unsupported_facts": {
            "assertions": len(measured),
            "unsupported": len(unsupported),
            "detail": [
                {"kind": a.kind, "sequence": a.sequence, "reason": a.reason} for a in unsupported
            ],
        },
        "schema": {
            "records": len(spans) + len(pairs),
            "problems": schema_problems,
        },
        "stability": [],
    }

    _check_expectations(case, result, counts)

    base_judgement = _judgement(spans)
    for variant in case.variants:
        variant_turns = materialise_turns(variant.turns)
        variant_spans = [
            _span_record(span)
            for span in extract_evidence(variant_turns, [dict(c) for c in dataset.competencies])
        ]
        same = _judgement(variant_spans) == base_judgement
        counts.probes += 1
        if variant.expect_same_judgement:
            counts.probes_expected_stable += 1
            if same:
                counts.probes_stable += 1
            else:
                counts.failures.append(
                    f"{variant.id}: the judgement moved under a change that must not move it"
                )
        else:
            counts.probes_known_sensitivity += 1
        result["stability"].append(
            {
                "variant_id": variant.id,
                "kind": variant.kind,
                "expect_same_judgement": variant.expect_same_judgement,
                "same_judgement": same,
            }
        )
        # A declared sensitivity that quietly starts behaving is worth
        # knowing about too: it means the extractor changed and the
        # dataset's note about it is now wrong.
        counts.expectations_checked += 1
        if same == variant.expect_same_judgement:
            counts.expectations_met += 1
        else:
            counts.failures.append(
                f"{variant.id}: stability was {same}, the fixture declares "
                f"{variant.expect_same_judgement}"
            )

    return result


def _rate(numerator: int, denominator: int) -> float:
    """A rate, rounded so two runs on two machines produce the same digits."""
    return round(numerator / denominator, 6) if denominator else 0.0


def run(datasets: tuple[Dataset, ...] | None = None) -> dict[str, Any]:
    """Run every fixture and answer the whole report document.

    The datasets are a parameter so the detection paths can be driven by a
    deliberately wrong fixture in the suite. A harness that has never been
    shown to notice a fixture misbehaving is not a harness, it is a
    formatter, and the only way to tell the difference is to hand it one.
    """
    datasets = datasets if datasets is not None else load_datasets()
    rubric = _artifact(RUBRIC_PATH)
    min_supporting = int(rubric["body"]["sufficiency"]["min_supporting"])
    counts = _Counts()
    cases: list[dict[str, Any]] = []
    per_case_ms: dict[str, float] = {}

    started = time.perf_counter()
    with counted_connections() as meter:
        for dataset in datasets:
            for case in dataset.cases:
                case_started = time.perf_counter()
                cases.append(_run_case(case, dataset, min_supporting, counts))
                per_case_ms[case.id] = round((time.perf_counter() - case_started) * 1000, 3)
    total_ms = round((time.perf_counter() - started) * 1000, 3)

    by_profession: dict[str, dict[str, int]] = {}
    for result in cases:
        bucket = by_profession.setdefault(
            result["profession"], {"cases": 0, "spans": 0, "contradictions": 0}
        )
        bucket["cases"] += 1
        bucket["spans"] += len(result["evidence_spans"])
        bucket["contradictions"] += len(result["contradictions"])

    document: dict[str, Any] = {
        "report_schema_version": REPORT_SCHEMA_VERSION,
        "harness_version": HARNESS_VERSION,
        "governed_inputs": {**governed_inputs(), "digest": governed_digest()},
        "totals": {
            "professions": len(datasets),
            "cases": len(cases),
            "grounding": {
                "checked": counts.grounding_checked,
                "grounded": counts.grounding_ok,
                "rate": _rate(counts.grounding_ok, counts.grounding_checked),
            },
            "unsupported_facts": {
                "assertions": counts.assertions,
                "unsupported": counts.unsupported,
                "rate": _rate(counts.unsupported, counts.assertions),
            },
            "schema_conformance": {
                "records": counts.schema_records,
                "conformant": counts.schema_ok,
                "rate": _rate(counts.schema_ok, counts.schema_records),
            },
            "expectations": {
                "checked": counts.expectations_checked,
                "met": counts.expectations_met,
                "rate": _rate(counts.expectations_met, counts.expectations_checked),
            },
            "contradictions": {
                "total": counts.contradictions_genuine + counts.contradictions_false,
                "genuine": counts.contradictions_genuine,
                "false_positive": counts.contradictions_false,
                "false_positive_rate": _rate(
                    counts.contradictions_false,
                    counts.contradictions_genuine + counts.contradictions_false,
                ),
            },
            "stability": {
                "probes": counts.probes,
                "expected_stable": counts.probes_expected_stable,
                "stable": counts.probes_stable,
                "known_sensitivities": counts.probes_known_sensitivity,
                "rate": _rate(counts.probes_stable, counts.probes_expected_stable),
            },
            "cost": {
                "provider_calls": 0,
                "network_connections": meter.connections,
                "input_tokens": 0,
                "output_tokens": 0,
                "note": "evidence-1 and the delivery calculator consult no provider. "
                "The connection count is measured, not assumed.",
            },
            "by_profession": by_profession,
        },
        "cases": cases,
        "failures": counts.failures,
    }
    document["results_digest"] = results_digest(document)
    # Timing is attached after the digest and excluded from it on purpose:
    # a slower machine is not a regression in the evaluation, and a report
    # whose digest moved every run would be comparable to nothing.
    document["timing"] = {"total_ms": total_ms, "per_case_ms": per_case_ms}
    return document


def results_digest(document: dict[str, Any]) -> str:
    """The digest over everything a change to which is a real difference."""
    deterministic = {
        key: value
        for key, value in document.items()
        if key not in ("timing", "results_digest", "generated")
    }
    return digest_of(deterministic)


def gate_violations(document: dict[str, Any]) -> list[str]:
    """The hard floors, which are absolutes rather than calibrated numbers.

    No invented facts, every span resolving to the transcript, every record
    in shape, and every fixture behaving as it says it does. Each of these
    is already required by the specification, so none of them is a guess.
    Anything that needs calibrating against human agreement belongs to
    QUA-03 and is reported here without being gated.
    """
    totals = document["totals"]
    violations: list[str] = []
    if totals["unsupported_facts"]["unsupported"] != 0:
        violations.append(
            f"unsupported facts: {totals['unsupported_facts']['unsupported']} assertions "
            "quote words that were not said"
        )
    if totals["grounding"]["grounded"] != totals["grounding"]["checked"]:
        violations.append(
            f"grounding: {totals['grounding']['checked'] - totals['grounding']['grounded']} "
            "spans do not resolve to the transcript"
        )
    if totals["schema_conformance"]["conformant"] != totals["schema_conformance"]["records"]:
        violations.append("schema conformance: a record is not the declared shape")
    if totals["expectations"]["met"] != totals["expectations"]["checked"]:
        violations.append(
            f"expectations: {totals['expectations']['checked'] - totals['expectations']['met']} "
            f"declared behaviours were not observed: {document['failures']}"
        )
    return violations


def load_committed_report() -> dict[str, Any]:
    """The report as committed, which is the baseline a run is compared to."""
    document: dict[str, Any] = json.loads(REPORT_PATH.read_text())
    return document


def write_report(document: dict[str, Any]) -> pathlib.Path:
    """Write the report artifact, stably encoded so a diff is readable."""
    REPORT_PATH.parent.mkdir(parents=True, exist_ok=True)
    REPORT_PATH.write_text(json.dumps(document, indent=2, sort_keys=True) + "\n")
    return REPORT_PATH
