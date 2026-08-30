"""QUA-02: the harness, and the three claims it has to be able to make.

That it runs on every prompt, model or rubric change is enforced two ways:
it is part of this suite, so every change runs it, and the committed report
records a digest over the governed inputs, so changing a rubric or a version
constant without regenerating the report fails a named test rather than
passing quietly.

That the unsupported-fact rate is measured rather than estimated is proved
by attacking the meter: a fabricated quote and a figure smuggled into a
placeholder both have to move the number. A grader that can only return zero
is not measuring anything.

That results are comparable is the report artifact: a stable, versioned,
digest-carrying document, with latency deliberately outside the digest
because a slower laptop is not a regression in the evaluation.
"""

from __future__ import annotations

import contextlib
import dataclasses
import json
import socket
from typing import Any

import pytest

from prepeet_ai.articulation.coaching import UnpreservingError
from prepeet_ai.evals import harness
from prepeet_ai.evals.dataset import (
    ExpectedContradiction,
    ExpectedEvidence,
    load_datasets,
    materialise_turns,
)
from prepeet_ai.evals.metrics import assertions, grounding_problems

REPORT = harness.run()
COMMITTED = harness.load_committed_report()
CASES = {case["case_id"]: case for case in REPORT["cases"]}


class TestTheHarnessRunsOnEveryGovernedChange:
    """The first box: a governed change cannot pass without a fresh report."""

    def test_the_committed_report_matches_a_fresh_run(self) -> None:
        """A change to the pipeline fails here until the report is regenerated."""
        assert REPORT["results_digest"] == COMMITTED["results_digest"]

    def test_the_committed_report_was_produced_by_the_current_governed_inputs(self) -> None:
        """Editing the rubric without rerunning the harness is the failure this catches."""
        assert COMMITTED["governed_inputs"]["digest"] == harness.governed_digest()

    def test_the_governed_digest_covers_the_rubric_body(self) -> None:
        """A rubric change is a governed change, so it has to move the digest."""
        inputs = harness.governed_inputs()
        moved = json.loads(json.dumps(inputs))
        moved["rubric"]["sha256"] = "0" * 64

        assert harness.digest_of(moved) != harness.digest_of(inputs)

    def test_the_governed_digest_covers_every_pipeline_version(self) -> None:
        """Prompt, model and rubric versions all ride the same digest."""
        inputs = harness.governed_inputs()
        for key in ("extraction_version", "profile_version", "coaching_version"):
            moved = json.loads(json.dumps(inputs))
            moved[key] = "changed"
            assert harness.digest_of(moved) != harness.digest_of(inputs), key


class TestUnsupportedFactsAreMeasured:
    """The second box, held open by attacking the meter itself."""

    def test_the_fixtures_produce_no_unsupported_facts(self) -> None:
        """The floor the specification already requires: no invented facts."""
        totals = REPORT["totals"]["unsupported_facts"]
        assert totals["assertions"] > 0, "a rate over nothing measures nothing"
        assert totals["unsupported"] == 0
        assert totals["rate"] == 0.0

    def test_every_assertion_the_pipeline_makes_is_checked(self) -> None:
        """Counted, not sampled: spans, both contradiction sides and every shape part."""
        for case in REPORT["cases"]:
            expected = (
                len(case["evidence_spans"])
                + 2 * len(case["contradictions"])
                + len(case["coaching_shape"])
            )
            assert case["unsupported_facts"]["assertions"] == expected, case["case_id"]

    def test_a_fabricated_quote_is_counted_as_unsupported(self) -> None:
        """The meter moves. Without this the zero above proves nothing."""
        case = load_datasets()[0].cases[0]
        turns = materialise_turns(case.turns)
        fabricated = {
            "competency_id": "systems-design",
            "kind": "supporting",
            "segment_sequence": 2,
            "quote": "I have never said this sentence in my life.",
            "char_start": 0,
            "char_end": 43,
            "start_ms": 0,
            "end_ms": 1,
            "extraction_version": "evidence-1",
        }

        measured = assertions(turns, [fabricated], [], None)

        assert [a.supported for a in measured] == [False]
        assert "character range" in measured[0].reason

    def test_a_figure_smuggled_into_a_placeholder_is_counted_as_unsupported(self) -> None:
        """A number in brackets is a fact wearing them, and it is caught as one."""
        case = load_datasets()[0].cases[0]
        turns = materialise_turns(case.turns)
        coaching = {
            "suggested_shape": [
                {"slot": "result", "kind": "placeholder", "text": "[Did it save 40 percent?]"}
            ]
        }

        measured = assertions(turns, [], [], coaching)

        assert measured[0].supported is False
        assert measured[0].reason == "carries a figure in brackets"

    def test_a_quote_attributed_to_the_interviewer_is_counted_as_unsupported(self) -> None:
        """Only the candidate's words can support a claim about the candidate."""
        case = load_datasets()[0].cases[0]
        turns = materialise_turns(case.turns)
        interviewer = next(t for t in turns if t["speaker"] == "interviewer")
        borrowed = {
            "competency_id": "systems-design",
            "kind": "supporting",
            "segment_sequence": interviewer["sequence"],
            "quote": interviewer["text"],
            "char_start": 0,
            "char_end": len(interviewer["text"]),
            "start_ms": interviewer["start_ms"],
            "end_ms": interviewer["end_ms"],
            "extraction_version": "evidence-1",
        }

        measured = assertions(turns, [borrowed], [], None)

        assert measured[0].supported is False


class TestEvidenceGroundingAndSchemaConformance:
    """Grounding and schema, measured the same way and attacked the same way."""

    def test_every_span_the_fixtures_produce_is_grounded(self) -> None:
        """Exact substring, real segment, timing inside the turn."""
        totals = REPORT["totals"]["grounding"]
        assert totals["checked"] > 0
        assert totals["grounded"] == totals["checked"]
        assert totals["rate"] == 1.0

    def test_a_span_whose_clock_leaves_its_turn_is_not_grounded(self) -> None:
        """The grounding check moves too."""
        case = load_datasets()[0].cases[0]
        turns = materialise_turns(case.turns)
        turn = next(t for t in turns if t["speaker"] == "candidate")
        drifted = {
            "segment_sequence": turn["sequence"],
            "quote": turn["text"][0:10],
            "char_start": 0,
            "char_end": 10,
            "start_ms": turn["start_ms"],
            "end_ms": int(turn["end_ms"]) + 5_000,
        }

        problems = grounding_problems(turns, drifted)

        assert any("clock range" in problem for problem in problems)

    def test_every_record_conforms_to_the_declared_schema(self) -> None:
        """Schema conformance is a count of records, not an impression."""
        totals = REPORT["totals"]["schema_conformance"]
        assert totals["records"] > 0
        assert totals["conformant"] == totals["records"]
        assert totals["rate"] == 1.0


class TestResultsAreComparableAcrossRunsAndVersions:
    """The third box: a stable artifact, not a printout."""

    def test_two_runs_of_the_same_code_produce_the_same_digest(self) -> None:
        """Comparable starts with reproducible."""
        assert harness.run()["results_digest"] == REPORT["results_digest"]

    def test_latency_is_recorded_but_kept_out_of_the_digest(self) -> None:
        """A slower machine is not a regression in the evaluation."""
        assert REPORT["timing"]["total_ms"] >= 0
        assert all(case["case_id"] in REPORT["timing"]["per_case_ms"] for case in REPORT["cases"])

        slowed = json.loads(json.dumps(REPORT))
        slowed["timing"]["total_ms"] += 10_000

        assert harness.results_digest(slowed) == harness.results_digest(REPORT)

    def test_a_changed_result_does_change_the_digest(self) -> None:
        """The digest is not inert: it has to notice a real difference."""
        changed = json.loads(json.dumps(REPORT))
        changed["cases"][0]["sufficiency"] = {"invented": "sufficient"}

        assert harness.results_digest(changed) != harness.results_digest(REPORT)

    def test_the_report_names_the_versions_that_produced_it(self) -> None:
        """A report that cannot say what produced it is not comparable to anything."""
        governed = REPORT["governed_inputs"]
        assert governed["extraction_version"]
        assert governed["profile_version"]
        assert governed["coaching_version"]
        assert governed["rubric"]["reference"] and governed["rubric"]["version"]
        assert REPORT["report_schema_version"] and REPORT["harness_version"]


class TestCostIsMeasuredRatherThanAssumed:
    """Cost is zero here, and zero is measured rather than asserted."""

    def test_the_run_reached_no_provider(self) -> None:
        """evidence-1 consults no model, and the socket meter proves it."""
        cost = REPORT["totals"]["cost"]
        assert cost["provider_calls"] == 0
        assert cost["network_connections"] == 0
        assert cost["input_tokens"] == 0
        assert cost["output_tokens"] == 0

    def test_the_connection_meter_counts_a_real_connection(self) -> None:
        """Without this, network_connections zero would only mean nothing counts."""
        with harness.counted_connections() as meter:
            probe = socket.socket()
            probe.settimeout(0.2)
            # A closed loopback port: the call is refused immediately and
            # never leaves the machine, which is all the meter has to see.
            probe.connect_ex(("127.0.0.1", 1))
            probe.close()

        assert meter.connections >= 1


class TestTheDatasetExpectationsAreEnforced:
    """The dataset declares what should happen; the harness checks it did."""

    def test_every_case_meets_its_declared_expectation(self) -> None:
        """A fixture whose expectation is not met is a failure, not a note."""
        totals = REPORT["totals"]["expectations"]
        assert totals["checked"] > 0
        assert totals["met"] == totals["checked"], REPORT["failures"]

    @pytest.mark.parametrize(
        "profession", ["engineering", "finance", "nursing", "product", "sales", "teaching"]
    )
    def test_every_profession_appears_in_the_report(self, profession: str) -> None:
        """The report segments by discipline, which is what makes it readable."""
        assert profession in REPORT["totals"]["by_profession"]

    def test_contradiction_false_positives_are_counted_not_hidden(self) -> None:
        """The dataset declares which pairs are genuine; the report keeps the split."""
        pairs = REPORT["totals"]["contradictions"]
        assert pairs["genuine"] > 0
        assert pairs["false_positive"] > 0
        assert pairs["genuine"] + pairs["false_positive"] == pairs["total"]

    def test_stability_is_reported_with_its_known_sensitivities(self) -> None:
        """Stability is measured over probes that declare what should survive them."""
        stability = REPORT["totals"]["stability"]
        assert stability["probes"] > 0
        assert stability["stable"] == stability["expected_stable"]
        assert stability["known_sensitivities"] > 0


class TestTheGates:
    """The hard floors, and proof that they can actually fail."""

    def test_the_fixtures_pass_every_hard_floor(self) -> None:
        """No invented facts, everything grounded, every record in shape."""
        assert harness.gate_violations(REPORT) == []

    def test_an_unsupported_fact_breaks_the_gate(self) -> None:
        """The gate is not decorative."""
        broken: dict[str, Any] = json.loads(json.dumps(REPORT))
        broken["totals"]["unsupported_facts"]["unsupported"] = 1
        broken["totals"]["unsupported_facts"]["rate"] = 0.01

        assert any("unsupported" in violation for violation in harness.gate_violations(broken))

    def test_an_ungrounded_span_breaks_the_gate(self) -> None:
        """So does a span that stops resolving to the transcript."""
        broken = json.loads(json.dumps(REPORT))
        broken["totals"]["grounding"]["grounded"] -= 1
        broken["totals"]["grounding"]["rate"] = 0.9

        assert any("grounding" in violation for violation in harness.gate_violations(broken))

    def test_an_unmet_expectation_breaks_the_gate(self) -> None:
        """And so does a fixture that stops behaving the way it says it does."""
        broken = json.loads(json.dumps(REPORT))
        broken["totals"]["expectations"]["met"] -= 1
        broken["failures"].append("invented failure")

        assert any("expectation" in violation for violation in harness.gate_violations(broken))


def _mutated(case_index: int, **changes: Any) -> tuple[Any, ...]:
    """One real dataset with one case deliberately altered."""
    base = load_datasets()[0]
    case = base.cases[case_index]
    altered = dataclasses.replace(case, **changes)
    cases = list(base.cases)
    cases[case_index] = altered
    return (dataclasses.replace(base, cases=tuple(cases)),)


class TestTheHarnessNoticesAFixtureThatStopsBehaving:
    """Detection, proved by handing the harness a fixture that is wrong.

    The gate tests above prove the gate fires once a number is bad. These
    prove the number goes bad in the first place, which is the half a
    report generator would quietly skip.
    """

    def test_an_unmet_sufficiency_expectation_is_recorded_and_gated(self) -> None:
        """A competency that stops being sufficient fails, it does not warn."""
        base = load_datasets()[0].cases[0]
        wrong = dataclasses.replace(
            base.expected,
            sufficiency=dict.fromkeys(base.expected.sufficiency, "not_discussed"),
        )

        document = harness.run(_mutated(0, expected=wrong))

        assert (
            document["totals"]["expectations"]["met"]
            < document["totals"]["expectations"]["checked"]
        )
        assert any("sufficiency" in failure for failure in document["failures"])
        assert any("expectation" in v for v in harness.gate_violations(document))

    def test_a_missing_evidence_span_is_recorded(self) -> None:
        """A quote the extractor stops producing is named with the quote."""
        base = load_datasets()[0].cases[0]
        wrong = dataclasses.replace(
            base.expected,
            evidence=(
                ExpectedEvidence(
                    competency_id="systems-design",
                    kind="supporting",
                    quote_contains="a sentence this candidate never uttered",
                ),
            ),
        )

        document = harness.run(_mutated(0, expected=wrong))

        assert any("never uttered" in failure for failure in document["failures"])

    def test_a_competency_declared_silent_that_speaks_is_recorded(self) -> None:
        """Silence is a claim about the transcript and it is checked as one."""
        base = load_datasets()[0].cases[0]
        wrong = dataclasses.replace(base.expected, silent_competencies=("systems-design",))

        document = harness.run(_mutated(0, expected=wrong))

        assert any("declared silent but produced a span" in f for f in document["failures"])

    def test_an_expected_contradiction_that_never_appears_is_recorded(self) -> None:
        """A pair the fixture expects and the extractor stops making."""
        base = load_datasets()[0].cases[0]
        wrong = dataclasses.replace(
            base.expected,
            contradictions=(
                ExpectedContradiction(
                    topic_includes=("nothing", "matching"),
                    genuine=True,
                    why="a contradiction this transcript does not contain",
                ),
            ),
        )

        document = harness.run(_mutated(0, expected=wrong))

        assert any("contradictions against the declared" in f for f in document["failures"])

    def test_a_wrongly_declared_delivery_status_is_recorded(self) -> None:
        """Assessability is an expectation like any other."""
        base = load_datasets()[0].cases[0]
        wrong = dataclasses.replace(base.expected, assessability="not_assessable")

        document = harness.run(_mutated(0, expected=wrong))

        assert any("delivery status" in failure for failure in document["failures"])

    def test_a_wrongly_declared_unassessable_reason_is_recorded(self) -> None:
        """The four unassessable causes are distinct, so naming the wrong one fails."""
        cases = load_datasets()[0].cases
        index = next(i for i, c in enumerate(cases) if c.case_class == "unassessable")
        wrong = dataclasses.replace(
            cases[index].expected, unassessable_reason="no_candidate_speech"
        )

        document = harness.run(_mutated(index, expected=wrong))

        assert any("do not include the declared" in failure for failure in document["failures"])


class TestStabilityProbesAreEnforcedInBothDirections:
    """A probe that moves fails, and so does a sensitivity that stops existing."""

    def test_a_probe_declared_stable_that_moves_is_a_failure(self) -> None:
        """The known synonym sensitivity, relabelled as something that must hold."""
        case = load_datasets()[0].cases[0]
        synonym = next(v for v in case.variants if v.kind == "vocabulary")
        relabelled = dataclasses.replace(synonym, expect_same_judgement=True)

        document = harness.run(_mutated(0, variants=(relabelled,)))

        assert any("must not move it" in failure for failure in document["failures"])

    def test_a_declared_sensitivity_that_quietly_stops_existing_is_a_failure(self) -> None:
        """If the extractor improves, the fixture's note about it is now wrong."""
        case = load_datasets()[0].cases[0]
        verbose = next(v for v in case.variants if v.kind == "verbosity")
        relabelled = dataclasses.replace(verbose, expect_same_judgement=False)

        document = harness.run(_mutated(0, variants=(relabelled,)))

        assert any("stability was True" in failure for failure in document["failures"])


class TestDegradationAndTheArtifactItself:
    """Coaching may fail without taking the evaluation with it."""

    def test_a_coaching_refusal_becomes_an_honest_absence(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Optional coaching failing does not block a valid core evaluation."""

        def refuse(turns: Any) -> dict[str, Any]:
            raise UnpreservingError("the headline slot quotes words the candidate did not say")

        monkeypatch.setattr(harness, "coaching_document", refuse)

        document = harness.run()

        assert document["cases"], "the run still produced results"
        assert all(case["delivery"]["coaching_withheld"] for case in document["cases"])
        assert all(case["coaching_shape"] == [] for case in document["cases"])
        assert document["totals"]["unsupported_facts"]["unsupported"] == 0

    def test_a_malformed_record_breaks_the_schema_gate(self) -> None:
        """The third hard floor fires like the other two."""
        broken = json.loads(json.dumps(REPORT))
        broken["totals"]["schema_conformance"]["conformant"] -= 1

        assert any("schema conformance" in v for v in harness.gate_violations(broken))

    def test_the_report_is_written_as_a_stable_readable_document(
        self, tmp_path: Any, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Sorted keys and indentation, so a diff between versions is readable."""
        target = tmp_path / "latest.json"
        monkeypatch.setattr(harness, "REPORT_PATH", target)

        harness.write_report(REPORT)

        first = target.read_text()
        harness.write_report(REPORT)
        assert target.read_text() == first
        assert json.loads(first)["results_digest"] == REPORT["results_digest"]

    def test_the_connect_meter_counts_the_blocking_call_too(self) -> None:
        """Connect and connect_ex are two doors, and both are counted."""
        with harness.counted_connections() as meter:
            probe = socket.socket()
            probe.settimeout(0.2)
            with contextlib.suppress(OSError):
                probe.connect(("127.0.0.1", 1))
            probe.close()

        assert meter.connections >= 1
