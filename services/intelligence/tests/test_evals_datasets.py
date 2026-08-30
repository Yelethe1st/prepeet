"""QUA-01: the evaluation datasets, and the provenance that makes them usable.

The dataset is only evidence if a reader can tell where it came from, what
each case is supposed to demonstrate, and what it deliberately does not
cover. These tests hold the fixtures to that: every profession the product
claims to serve is present, every case declares the evidence it expects,
the three edge classes the ticket names are present on purpose rather than
by accident, and the manifest's digests match the bytes on disk so a fixture
cannot be edited without the provenance record moving with it.
"""

from __future__ import annotations

from prepeet_ai.evals.dataset import (
    CASE_CLASSES,
    MANIFEST_REQUIRED_FIELDS,
    SUPPORTED_PROFESSIONS,
    load_datasets,
    load_manifest,
    manifest_digest_mismatches,
)

DATASETS = load_datasets()
MANIFEST = load_manifest()


class TestEveryProfessionIsRepresented:
    """The first box: fixtures per profession, with expectations and edges."""

    def test_every_supported_profession_has_a_dataset(self) -> None:
        """Nursing, teaching, finance, sales, product and engineering, not just software."""
        assert {dataset.profession for dataset in DATASETS} == set(SUPPORTED_PROFESSIONS)

    def test_every_case_declares_the_evidence_it_expects(self) -> None:
        """A fixture with no expectation measures nothing."""
        for dataset in DATASETS:
            for case in dataset.cases:
                assert case.expected.sufficiency, f"{case.id} declares no expected sufficiency"
                assert set(case.expected.sufficiency) == {
                    competency["id"] for competency in dataset.competencies
                }, f"{case.id} does not account for every competency"

    def test_every_case_names_why_it_exists(self) -> None:
        """A case without a stated purpose cannot be reviewed."""
        for dataset in DATASETS:
            for case in dataset.cases:
                assert case.notes.strip(), f"{case.id} has no note saying what it demonstrates"
                assert case.case_class in CASE_CLASSES, case.id


class TestTheDeliberateEdgeCases:
    """The second box: insufficient evidence, contradiction and unassessable."""

    def test_every_profession_carries_all_three_deliberate_classes(self) -> None:
        """Deliberate means present in every profession, not once across the set."""
        for dataset in DATASETS:
            classes = {case.case_class for case in dataset.cases}
            for required in ("insufficient_evidence", "contradiction", "unassessable"):
                assert required in classes, f"{dataset.profession} has no {required} case"

    def test_the_unassessable_shapes_are_covered_across_the_set(self) -> None:
        """Unassessable has more than one cause, and each has its own remedy."""
        shapes = {
            case.expected.unassessable_reason
            for dataset in DATASETS
            for case in dataset.cases
            if case.case_class == "unassessable"
        }
        assert shapes >= {
            "no_candidate_speech",
            "no_word_timing",
            "transcript_confidence_low",
            "insufficient_speech",
        }


class TestProvenance:
    """The third box: provenance, consent and licensing, recorded and checkable."""

    def test_the_manifest_carries_every_field_the_specification_names(self) -> None:
        """evaluation-system.md lists what a dataset manifest must describe."""
        assert set(MANIFEST) >= MANIFEST_REQUIRED_FIELDS

    def test_the_manifest_states_that_the_fixtures_are_synthetic(self) -> None:
        """Synthetic is a claim about the data, and it has to be made explicitly."""
        assert MANIFEST["source_status"] == "synthetic"

    def test_every_dataset_file_is_listed_and_its_digest_matches(self) -> None:
        """A fixture edited without the manifest moving is a broken provenance record."""
        assert manifest_digest_mismatches() == []
