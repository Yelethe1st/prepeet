"""The graders' failure paths, which are the only paths that matter.

Every measurement in the harness reports zero problems against the
fixtures. That number is worth nothing unless each branch that could
report a problem has been made to report one, so this file drives every
one of them deliberately. It is the same discipline the rest of the
service applies to its guards: a check nobody has seen fail is a check
nobody has checked.
"""

from __future__ import annotations

import json
import pathlib

import pytest

from prepeet_ai.evals import harness
from prepeet_ai.evals.__main__ import main
from prepeet_ai.evals.dataset import (
    load_datasets,
    manifest_digest_mismatches,
    materialise_turns,
    refresh_manifest,
)
from prepeet_ai.evals.metrics import (
    assertions,
    contradiction_schema_problems,
    evidence_span_schema_problems,
    grounding_problems,
)

TURNS = materialise_turns(load_datasets()[0].cases[0].turns)
CANDIDATE = next(turn for turn in TURNS if turn["speaker"] == "candidate")


def _span(**overrides: object) -> dict[str, object]:
    """A span that is correct in every respect until one field is broken."""
    record: dict[str, object] = {
        "competency_id": "systems-design",
        "kind": "supporting",
        "segment_sequence": CANDIDATE["sequence"],
        "quote": CANDIDATE["text"][0:12],
        "char_start": 0,
        "char_end": 12,
        "start_ms": CANDIDATE["start_ms"],
        "end_ms": CANDIDATE["end_ms"],
        "extraction_version": "evidence-1",
    }
    record.update(overrides)
    return record


def _pair() -> dict[str, object]:
    side = {
        "segment_sequence": CANDIDATE["sequence"],
        "quote": CANDIDATE["text"][0:12],
        "char_start": 0,
        "char_end": 12,
        "start_ms": CANDIDATE["start_ms"],
        "end_ms": CANDIDATE["end_ms"],
    }
    return {
        "topic": ["design", "systems"],
        "side_a": dict(side),
        "side_b": dict(side),
        "extraction_version": "evidence-1",
    }


class TestGroundingReportsEveryWayASpanCanComeLoose:
    """Four ways a span stops resolving, and each is named rather than counted."""

    def test_a_span_naming_a_segment_that_does_not_exist(self) -> None:
        """A segment number nobody spoke cannot be a source."""
        assert grounding_problems(TURNS, _span(segment_sequence=999)) == [
            "segment 999 is not in the transcript"
        ]

    def test_a_span_attributed_to_the_interviewer(self) -> None:
        """The interviewer's words are context, never the candidate's evidence."""
        interviewer = next(turn for turn in TURNS if turn["speaker"] == "interviewer")
        problems = grounding_problems(
            TURNS,
            _span(
                segment_sequence=interviewer["sequence"],
                quote=interviewer["text"][0:12],
                start_ms=interviewer["start_ms"],
                end_ms=interviewer["end_ms"],
            ),
        )

        assert any("not the candidate's turn" in problem for problem in problems)

    def test_a_character_range_outside_the_turn(self) -> None:
        """A range past the end of the text is caught before it is sliced."""
        problems = grounding_problems(TURNS, _span(char_start=0, char_end=100_000))

        assert any("outside a turn" in problem for problem in problems)

    def test_a_quote_that_is_not_the_text_at_its_own_range(self) -> None:
        """The exact-substring rule, which is the whole anti-fabrication floor."""
        problems = grounding_problems(TURNS, _span(quote="words nobody said"))

        assert problems == ["the quote is not the text at its own character range"]

    def test_a_clock_range_that_leaves_the_turn(self) -> None:
        """Timing that drifts outside its turn no longer resolves to the audio."""
        problems = grounding_problems(TURNS, _span(start_ms=int(CANDIDATE["start_ms"]) - 1))

        assert any("clock range" in problem for problem in problems)

    def test_a_correct_span_reports_nothing(self) -> None:
        """The grader is not simply always unhappy."""
        assert grounding_problems(TURNS, _span()) == []


class TestTheSpanSchemaRefusesEveryMalformedShape:
    """Schema conformance is a count, and this is what it counts."""

    def test_a_missing_field_is_named(self) -> None:
        """The missing field is named, because that is the useful message."""
        record = _span()
        del record["kind"]

        assert evidence_span_schema_problems(record) == ["missing kind"]

    def test_an_undeclared_evidence_kind(self) -> None:
        """A new evidence category cannot be introduced by writing one."""
        problems = evidence_span_schema_problems(_span(kind="excellent"))

        assert any("not one of the declared evidence kinds" in problem for problem in problems)

    def test_an_empty_competency_identifier(self) -> None:
        """A span linked to nothing links to nothing."""
        problems = evidence_span_schema_problems(_span(competency_id=""))

        assert "competency_id is not a non-empty string" in problems

    def test_an_empty_quote_is_an_assertion_about_nothing(self) -> None:
        """An empty quote would pass grounding and mean nothing."""
        problems = evidence_span_schema_problems(_span(quote="   "))

        assert "quote is empty, which is an assertion about nothing" in problems

    @pytest.mark.parametrize(
        "field", ["segment_sequence", "char_start", "char_end", "start_ms", "end_ms"]
    )
    def test_a_numeric_field_that_is_not_an_integer(self, field: str) -> None:
        """A string where an offset belongs breaks every consumer downstream."""
        problems = evidence_span_schema_problems(_span(**{field: "3"}))

        assert f"{field} is not an integer" in problems

    def test_a_boolean_is_not_accepted_as_an_integer(self) -> None:
        """True is an int in Python and is not a segment number anywhere else."""
        problems = evidence_span_schema_problems(_span(segment_sequence=True))

        assert "segment_sequence is not an integer" in problems

    def test_a_missing_extraction_version_cannot_be_reproduced(self) -> None:
        """Without a version the reading cannot be reproduced or superseded."""
        problems = evidence_span_schema_problems(_span(extraction_version=""))

        assert any("cannot be reproduced" in problem for problem in problems)

    def test_a_correct_record_reports_nothing(self) -> None:
        """The span schema check is not simply always unhappy."""
        assert evidence_span_schema_problems(_span()) == []


class TestTheContradictionSchemaRefusesEveryMalformedShape:
    """A pair nobody can see the reason for is not publishable evidence."""

    def test_an_empty_topic_hides_why_the_pair_was_made(self) -> None:
        """A reviewer must be able to see why two statements were paired."""
        record = _pair()
        record["topic"] = []

        assert any("topic is empty" in problem for problem in contradiction_schema_problems(record))

    def test_a_topic_holding_something_that_is_not_a_word(self) -> None:
        """A topic is the shared subject tokens, not arbitrary values."""
        record = _pair()
        record["topic"] = ["design", 7]

        assert any("not a word" in problem for problem in contradiction_schema_problems(record))

    def test_a_missing_side(self) -> None:
        """A contradiction with one side is not a contradiction."""
        record = _pair()
        del record["side_b"]

        assert "side_b is missing" in contradiction_schema_problems(record)

    def test_a_side_missing_a_field(self) -> None:
        """Each side has to resolve to the transcript like any other quote."""
        record = _pair()
        del record["side_a"]["quote"]

        assert "side_a is missing quote" in contradiction_schema_problems(record)

    def test_a_missing_extraction_version(self) -> None:
        """Same rule as a span: an unversioned reading cannot be reproduced."""
        record = _pair()
        record["extraction_version"] = ""

        assert any(
            "cannot be reproduced" in problem for problem in contradiction_schema_problems(record)
        )

    def test_a_correct_record_reports_nothing(self) -> None:
        """The contradiction schema check is not simply always unhappy."""
        assert contradiction_schema_problems(_pair()) == []


class TestTheUnsupportedFactMeterOnEverySurface:
    """Each kind of assertion has to be capable of being counted unsupported."""

    def test_a_contradiction_side_quoting_words_nobody_said(self) -> None:
        """One fabricated side moves the rate; the honest side does not."""
        record = _pair()
        record["side_b"]["quote"] = "a sentence from another interview entirely"

        measured = assertions(TURNS, [], [record], None)

        assert [a.supported for a in measured] == [True, False]
        assert measured[1].kind == "contradiction_quote"

    def test_a_coaching_quote_the_candidate_never_said(self) -> None:
        """Coaching is where invented prose would appear, so it is measured."""
        coaching = {
            "suggested_shape": [
                {
                    "slot": "headline",
                    "kind": "quote",
                    "text": "I single handedly saved the company.",
                    "sequence": CANDIDATE["sequence"],
                }
            ]
        }

        measured = assertions(TURNS, [], [], coaching)

        assert measured[0].supported is False
        assert measured[0].reason == "the quote is not in the turn it names"

    def test_a_coaching_quote_attributed_to_no_turn_at_all(self) -> None:
        """A quote with no source is unsupported however plausible it reads."""
        coaching = {
            "suggested_shape": [
                {"slot": "headline", "kind": "quote", "text": "anything", "sequence": None}
            ]
        }

        measured = assertions(TURNS, [], [], coaching)

        assert measured[0].supported is False
        assert "not a candidate turn" in measured[0].reason

    def test_a_placeholder_that_is_not_a_question(self) -> None:
        """A placeholder that states rather than asks is the pipeline talking."""
        coaching = {
            "suggested_shape": [
                {"slot": "result", "kind": "placeholder", "text": "You should say more here."}
            ]
        }

        measured = assertions(TURNS, [], [], coaching)

        assert measured[0].reason == "not a bracketed question"

    def test_a_real_coaching_document_is_fully_supported(self) -> None:
        """The gate the service already runs, measured rather than trusted."""
        from prepeet_ai.articulation.coaching import coaching_document

        measured = assertions(TURNS, [], [], coaching_document(TURNS))

        assert measured, "this fixture does produce a suggested shape"
        assert all(a.supported for a in measured)


class TestTheManifestGuardNamesWhatMoved:
    """Three ways provenance and data can drift apart, each driven for real."""

    def _dataset_dir(self, tmp_path: pathlib.Path, digest: str) -> pathlib.Path:
        (tmp_path / "example.json").write_text('{"profession": "example"}')
        (tmp_path / "manifest.json").write_text(
            json.dumps({"datasets": [{"file": "example.json", "sha256": digest}]})
        )
        return tmp_path

    def test_a_file_the_manifest_does_not_list(self, tmp_path: pathlib.Path) -> None:
        """A fixture added without a provenance entry is named."""
        directory = self._dataset_dir(tmp_path, "unused")
        (directory / "manifest.json").write_text(json.dumps({"datasets": []}))

        assert manifest_digest_mismatches(directory) == [
            "example.json is not listed in the manifest"
        ]

    def test_a_file_the_manifest_lists_but_nobody_wrote(self, tmp_path: pathlib.Path) -> None:
        """A provenance entry for a file nobody wrote is named too."""
        (tmp_path / "manifest.json").write_text(
            json.dumps({"datasets": [{"file": "ghost.json", "sha256": "x"}]})
        )

        assert manifest_digest_mismatches(tmp_path) == [
            "ghost.json is listed in the manifest but is not on disk"
        ]

    def test_a_file_whose_digest_no_longer_matches(self, tmp_path: pathlib.Path) -> None:
        """An edited fixture is caught by the digest, with both values shown."""
        directory = self._dataset_dir(tmp_path, "0" * 64)

        problems = manifest_digest_mismatches(directory)

        assert len(problems) == 1
        assert problems[0].startswith("example.json digest is ")

    def test_refreshing_the_manifest_is_idempotent(self) -> None:
        """Regeneration must not churn the file, or every run becomes a diff."""
        from prepeet_ai.evals.dataset import MANIFEST_PATH

        before = MANIFEST_PATH.read_bytes()

        moved = refresh_manifest()

        assert moved == []
        assert MANIFEST_PATH.read_bytes() == before


class TestTheCommandLine:
    """The regeneration path a person actually runs."""

    def test_it_succeeds_and_reports_the_totals(
        self, capsys: pytest.CaptureFixture[str], monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """The report write is stubbed so the test cannot damage the committed artifact."""
        monkeypatch.setattr(harness, "write_report", lambda document: pathlib.Path("unused"))

        assert main() == 0

        printed = capsys.readouterr().out
        assert "results digest" in printed
        assert "GATE FAILED" not in printed
        # QUA-03 did not calibrate anything, and the command that prints
        # the numbers says so rather than leaving a reader to assume.
        assert "NOT calibrated" in printed
        assert "NO_HUMAN_BENCHMARK_SET" in printed

    def test_a_gate_failure_is_a_non_zero_exit(
        self, capsys: pytest.CaptureFixture[str], monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Without this the command could fail silently and CI would be happy."""
        broken = json.loads(json.dumps(harness.run()))
        broken["totals"]["unsupported_facts"]["unsupported"] = 3

        monkeypatch.setattr(harness, "run", lambda: broken)
        monkeypatch.setattr(harness, "write_report", lambda document: pathlib.Path("unused"))

        assert main() == 1
        assert "GATE FAILED" in capsys.readouterr().out
