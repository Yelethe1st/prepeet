r"""Go-compatible JSON canonicalisation: the digest an artifact *is*.

Every registry artifact is identified by the SHA-256 of its canonical JSON,
and Go's `content.DigestOf` computes that identity when the artifact is
published. This module recomputes it. The two must agree byte for byte,
because a disagreement is not a cosmetic difference: it makes composition
refuse a body the registry considers correct, which is exactly the failure
this comment exists to stop somebody re-introducing.

Go's canonical form is `json.Unmarshal` into `any` followed by
`json.Marshal`, so this mirrors that pipeline rather than Python's own
defaults. The three places the defaults disagree, each of which has broken
or would have broken a real artifact:

  Numbers. Go decodes every JSON number into float64 and re-encodes with
  the shortest representation that round-trips, which for a whole number
  carries no decimal point: `0.0` in the source becomes `0`. Python's
  json.dumps preserves the float and writes `0.0`. The shipped
  practice-default rubric contains `"min_ratio": 0.0`, so before this
  module existed every composition against it was refused.

  Non-ASCII. Go emits raw UTF-8; Python's json.dumps defaults to
  ensure_ascii=True and writes `\\uXXXX`. Any curly quote in a rubric's
  prose would have diverged the same way.

  HTML. Go's Marshal escapes `<`, `>` and `&` by default; Python does not.
  A competency named "Debugging & incident response" - which the shipped
  catalogue has - diverges on the ampersand alone.
"""

from __future__ import annotations

import hashlib
import json
import math
from decimal import Decimal
from typing import Any

# What Go's encoder escapes beyond the JSON minimum, mapped to the exact
# escapes it emits. U+2028 and U+2029 are escaped because they are line
# terminators to a JavaScript parser but not to a JSON one.
_ESCAPES = {
    '"': '\\"',
    "\\": "\\\\",
    "\n": "\\n",
    "\r": "\\r",
    "\t": "\\t",
    "<": "\\u003c",
    ">": "\\u003e",
    "&": "\\u0026",
    " ": "\\u2028",  # noqa: RUF001 - the character is the point
    " ": "\\u2029",  # noqa: RUF001 - the character is the point
}

# The bounds at which Go's encoder switches from fixed to exponent notation
# for a float64 (encoding/json's floatEncoder).
_EXPONENT_BELOW = 1e-6
_EXPONENT_AT_OR_ABOVE = 1e21


def canonical_digest(body: bytes) -> str:
    """The digest the registry stores for this body.

    Raises ValueError when the body is not JSON Go could have produced,
    which the caller reports as an invalid pin rather than a mismatch: a
    body that cannot be canonicalised has no digest to disagree with.
    """
    return "sha256:" + hashlib.sha256(canonical_bytes(body)).hexdigest()


def canonical_bytes(body: bytes) -> bytes:
    """Re-encode a JSON document exactly as Go's json.Marshal would.

    parse_int=float mirrors Go decoding every number into float64, losing
    the same precision beyond 2^53 that Go loses. Matching Go's answer
    matters more here than being more precise than it.
    """
    decoded = json.loads(body, parse_int=float, parse_float=float)
    return _encode(decoded).encode("utf-8")


def _encode(value: Any) -> str:
    """One value, in Go's canonical form."""
    if value is None:
        return "null"
    if value is True:
        return "true"
    if value is False:
        return "false"
    if isinstance(value, str):
        return _encode_string(value)
    if isinstance(value, float):
        return _encode_number(value)
    if isinstance(value, dict):
        # Go sorts map keys; a JSON object's key order is not meaning, and
        # sorting is what makes two authorings of the same document share
        # an identity.
        parts = (f"{_encode_string(key)}:{_encode(item)}" for key, item in sorted(value.items()))
        return "{" + ",".join(parts) + "}"
    if isinstance(value, list):
        return "[" + ",".join(_encode(item) for item in value) + "]"
    raise ValueError(f"cannot canonicalise {type(value).__name__}")


def _encode_string(value: str) -> str:
    """A JSON string with Go's escaping, raw UTF-8 for everything else."""
    out = ['"']
    for character in value:
        escape = _ESCAPES.get(character)
        if escape is not None:
            out.append(escape)
        elif character < "\x20":
            out.append(f"\\u{ord(character):04x}")
        else:
            out.append(character)
    out.append('"')
    return "".join(out)


def _encode_number(value: float) -> str:
    """A float64 formatted as Go's encoder writes it.

    Go refuses to encode NaN and the infinities at all rather than writing
    a token no JSON parser accepts, so a body carrying one has no canonical
    form and no digest.
    """
    if math.isnan(value) or math.isinf(value):
        raise ValueError("JSON has no representation for NaN or Infinity")

    magnitude = abs(value)
    if magnitude != 0.0 and (magnitude < _EXPONENT_BELOW or magnitude >= _EXPONENT_AT_OR_ABOVE):
        # Exponent form. Python's repr is already the shortest round-trip
        # and always exponential in this range; only the exponent's own
        # zero padding differs, which Go trims (1e-07 becomes 1e-7).
        mantissa, _, exponent = repr(value).partition("e")
        sign, digits = exponent[0], exponent[1:].lstrip("0") or "0"
        return f"{mantissa}e{sign}{digits}"

    # Fixed form. repr gives the shortest round-tripping digits but may
    # choose exponent notation where Go would not, so it goes through
    # Decimal to be re-laid-out without one; the digits never change.
    text = format(Decimal(repr(value)), "f")
    if "." in text:
        text = text.rstrip("0").rstrip(".")
    return text
