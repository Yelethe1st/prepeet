"""Canonicalisation against Go's encoder, on the cases that diverge.

Each test here is a shape that broke, or would have broken, a real
artifact. The expected strings are Go's own output for the same input:
`json.Unmarshal` into `any` followed by `json.Marshal`, which is exactly
what content.DigestOf does when it computes the identity these bodies are
published under.
"""

from __future__ import annotations

import json
import pathlib

import pytest

from prepeet_ai.composition.canonical import canonical_bytes, canonical_digest

REPOSITORY = pathlib.Path(__file__).resolve().parents[3]


def canonical(value: object) -> str:
    """The canonical form of a value, as text, for readable assertions."""
    return canonical_bytes(json.dumps(value).encode()).decode()


class TestNumbers:
    """Go decodes every number into float64 and writes the shortest form."""

    def test_a_whole_number_float_loses_its_decimal_point(self) -> None:
        """0.0 canonicalises to 0, as Go's float64 encoder writes it."""
        # The divergence that refused every composition against the shipped
        # rubric: its bands carry "min_ratio": 0.0, and Python's own encoder
        # writes 0.0 where Go writes 0.
        assert canonical({"min_ratio": 0.0}) == '{"min_ratio":0}'
        assert canonical({"n": 1.0}) == '{"n":1}'
        assert canonical({"n": -0.0}) == '{"n":-0}'

    def test_a_fractional_number_keeps_its_shortest_digits(self) -> None:
        """A real fraction keeps the digits that round-trip it."""
        assert canonical({"n": 0.55}) == '{"n":0.55}'
        assert canonical({"n": 0.8}) == '{"n":0.8}'

    def test_integers_and_whole_floats_are_indistinguishable(self) -> None:
        """2 and 2.0 are one document to Go, so they share one identity."""
        # Go has only float64 here, so 2 and 2.0 are the same document and
        # must carry the same identity.
        assert canonical_digest(b'{"n":2}') == canonical_digest(b'{"n":2.0}')

    def test_fixed_notation_holds_where_go_holds_it(self) -> None:
        """Fixed notation up to the bounds Go switches at, not Python's."""
        # Go switches to exponent below 1e-6 and at or above 1e21, and
        # nowhere else; Python's repr switches at different points.
        assert canonical({"n": 1e16}) == '{"n":10000000000000000}'
        assert canonical({"n": 1e-6}) == '{"n":0.000001}'

    def test_exponent_notation_matches_gos_trimmed_form(self) -> None:
        """Exponents carry Go's trimmed spelling, not Python's padded one."""
        # Go trims the leading zero from a two-digit exponent: 1e-07 is
        # Python's spelling, 1e-7 is Go's.
        assert canonical({"n": 1e-7}) == '{"n":1e-7}'
        assert canonical({"n": 1e21}) == '{"n":1e+21}'

    def test_a_number_json_cannot_hold_has_no_digest(self) -> None:
        """NaN and the infinities have no canonical form, so no digest."""
        # Go refuses to encode these rather than emitting a token no parser
        # accepts, so there is no canonical form to disagree with.
        with pytest.raises(ValueError):
            canonical_bytes(b'{"n": 1e999}')


class TestStrings:
    """Go escapes HTML characters and emits everything else as UTF-8."""

    def test_html_characters_are_escaped_as_go_escapes_them(self) -> None:
        """Go escapes <, > and & by default; matching it is not optional."""
        # The shipped catalogue has a competency named with an ampersand,
        # which Python's encoder leaves raw and Go's does not.
        assert canonical({"k": "Debugging & response"}) == '{"k":"Debugging \\u0026 response"}'
        assert canonical({"k": "<b>"}) == '{"k":"\\u003cb\\u003e"}'

    def test_non_ascii_stays_raw_utf8(self) -> None:
        """Go emits UTF-8 rather than Python's ensure_ascii escapes."""
        # Python's ensure_ascii default would write \\u2019 here.
        assert canonical_bytes(b'{"k":"it\xe2\x80\x99s"}') == b'{"k":"it\xe2\x80\x99s"}'

    def test_control_characters_and_quotes_escape(self) -> None:
        """The JSON minimum escapes, spelled as Go spells them."""
        assert canonical({"k": 'a"b\\c\nd\te'}) == '{"k":"a\\"b\\\\c\\nd\\te"}'
        assert canonical({"k": "\x01"}) == '{"k":"\\u0001"}'

    def test_line_terminators_are_escaped(self) -> None:
        """U+2028 and U+2029 are escaped, as Go escapes them."""
        assert canonical({"k": "  "}) == '{"k":"\\u2028\\u2029"}'  # noqa: RUF001


class TestStructure:
    """Key order is not meaning; sorting is what gives a document identity."""

    def test_keys_sort_and_whitespace_goes(self) -> None:
        """Sorted keys and no whitespace: the shape of a canonical form."""
        assert canonical_bytes(b'{ "b": 1, "a": 2 }') == b'{"a":2,"b":1}'

    def test_two_authorings_of_one_document_share_a_digest(self) -> None:
        """Formatting is not meaning, so it cannot change an identity."""
        assert canonical_digest(b'{"a":1,"b":[1,2]}') == canonical_digest(
            b'{\n  "b": [1, 2],\n  "a": 1\n}'
        )

    def test_arrays_keep_their_order(self) -> None:
        """An array's order is meaning and survives untouched."""
        assert canonical([3, 1, 2]) == "[3,1,2]"

    def test_literals_and_nesting(self) -> None:
        """Literals and nesting encode as Go encodes them."""
        assert canonical({"a": None, "b": True, "c": [{"d": False}]}) == (
            '{"a":null,"b":true,"c":[{"d":false}]}'
        )


class TestShippedArtifacts:
    """Every artifact this repository ships must canonicalise.

    The bug this file exists for reached a running stack because the Go
    fixture that claimed to mirror the shipped rubric wrote 0 where the
    rubric writes 0.0. Reading the real files closes that gap: a new
    artifact whose shape diverges fails here rather than in composition.
    """

    def test_every_shipped_artifact_has_a_canonical_form(self) -> None:
        """Every artifact this repository ships can be canonicalised."""
        artifacts = sorted((REPOSITORY / "services/intelligence/artifacts").rglob("*.json"))
        assert artifacts, "no artifacts found; the path is wrong"
        for path in artifacts:
            document = json.loads(path.read_text())
            body = json.dumps(document["body"]).encode()
            assert canonical_digest(body).startswith("sha256:"), path

    def test_the_practice_rubric_matches_gos_digest(self) -> None:
        """The shipped rubric's digest is the one Go computes for it."""
        # Pinned against the value Go's content.DigestOf produces for this
        # exact file. If the rubric changes, this changes with it; if the
        # canonicaliser drifts from Go, only this fails.
        path = REPOSITORY / "services/intelligence/artifacts/rubric/practice-default@1.1.0.json"
        body = json.dumps(json.loads(path.read_text())["body"]).encode()
        assert canonical_digest(body) == (
            "sha256:2c45988e09acd4259e12224f673a481730a49b051cc4a10853ea6fc17c399cbb"
        )
