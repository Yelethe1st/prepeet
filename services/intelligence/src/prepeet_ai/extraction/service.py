"""The extraction capability's boundary: fetch, verify, extract, encode.

The document arrives as a short-lived presigned URL - the caller's scoped
grant - and the digest it must hash to. Verification comes before reading:
extracting from bytes that do not match the pin would produce facts whose
provenance lies, and ARTIFACT_NOT_FOUND is the contract's word for "the
pinned content is what was asked for and it no longer exists".

Formats extract-1 cannot read are UNASSESSABLE_INPUT, never half-read: the
caller records the state and the profile continues manually, which is the
degradation PRO-03 designs the journey around.
"""

from __future__ import annotations

import hashlib
import json
import urllib.error
import urllib.request
from dataclasses import dataclass

from prepeet_ai.extraction.extractor import (
    EXTRACTOR_VERSION,
    SUPPORTED_MEDIA_TYPES,
    extract,
)

__all__ = ["EXTRACTOR_VERSION", "SCHEMA_VERSION", "Claim", "extract_document"]
from prepeet_ai.transport.envelope import Failure, FailureCode, FailureError

SCHEMA_VERSION = "0.1"
"""The claim schema: kind, a JSON value shaped per kind, and the span."""

_MAX_DOCUMENT_BYTES = 10 * 1024 * 1024
_FETCH_TIMEOUT_SECONDS = 20


@dataclass(frozen=True, slots=True)
class Claim:
    """One claim as the contract carries it."""

    kind: str
    value: bytes
    source_span: str


def extract_document(fetch_url: str, media_type: str, digest: str) -> list[Claim]:
    """Fetch the pinned document and extract it.

    Raises:
        FailureError: INVALID_INPUT for a request missing its URL or digest;
            UNASSESSABLE_INPUT for a format extract-1 cannot honestly read;
            ARTIFACT_NOT_FOUND when the fetched bytes do not hash to the pin
            or the URL no longer answers.
    """
    if not fetch_url:
        raise FailureError(
            Failure(code=FailureCode.INVALID_INPUT, message="a fetch URL is required")
        )
    if not digest:
        raise FailureError(
            Failure(code=FailureCode.INVALID_INPUT, message="the document's digest is required")
        )
    if media_type not in SUPPORTED_MEDIA_TYPES:
        raise FailureError(
            Failure(
                code=FailureCode.UNASSESSABLE_INPUT,
                message=(
                    f"extract-1 cannot read {media_type or 'an undeclared type'}; "
                    "the profile continues manually"
                ),
                detail={"media_type": media_type},
            )
        )

    try:
        with urllib.request.urlopen(fetch_url, timeout=_FETCH_TIMEOUT_SECONDS) as response:
            body = response.read(_MAX_DOCUMENT_BYTES + 1)
    except urllib.error.URLError as error:
        raise FailureError(
            Failure(
                code=FailureCode.ARTIFACT_NOT_FOUND,
                message="the document could not be fetched through its grant",
            )
        ) from error
    if len(body) > _MAX_DOCUMENT_BYTES:
        raise FailureError(
            Failure(code=FailureCode.INVALID_INPUT, message="the document exceeds the size bound")
        )

    fetched = hashlib.sha256(body).hexdigest()
    expected = digest.removeprefix("sha256:")
    if fetched != expected:
        raise FailureError(
            Failure(
                code=FailureCode.ARTIFACT_NOT_FOUND,
                message="the fetched bytes do not match the pinned digest",
            )
        )

    text = body.decode("utf-8", errors="replace")
    return [
        Claim(
            kind=fact.kind,
            value=json.dumps(
                {**fact.value, "confidence": fact.confidence},
                sort_keys=True,
                separators=(",", ":"),
            ).encode(),
            source_span=f"{fact.span_start}-{fact.span_end}",
        )
        for fact in extract(text)
    ]
