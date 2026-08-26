"""The evidence capability's boundary: fetch the sealed input, extract.

The sealed evaluation-input document arrives as a scoped grant plus the
digest it must hash to, exactly like every other pinned read. Extraction
runs only over verified bytes, so a span can never claim provenance in a
conversation nobody checked.
"""

from __future__ import annotations

import json

from prepeet_ai.evaluation.evidence import (
    EXTRACTION_VERSION,
    EvidenceSpan,
    extract_evidence,
)
from prepeet_ai.extraction.service import fetch_verified
from prepeet_ai.transport.envelope import Failure, FailureCode, FailureError

SCHEMA_VERSION = "0.1"
"""The observation schema: kind, quote, character range, clock range."""

__all__ = ["EXTRACTION_VERSION", "SCHEMA_VERSION", "EvidenceSpan", "evidence_from_ref"]


def evidence_from_ref(fetch_url: str, digest: str) -> list[EvidenceSpan]:
    """Fetch one sealed input document and extract its evidence.

    Raises:
        FailureError: INVALID_INPUT for a request missing its grant or a
            document that is not the sealed shape; ARTIFACT_NOT_FOUND from
            the verified fetch.
    """
    if not fetch_url or not digest:
        raise FailureError(
            Failure(
                code=FailureCode.INVALID_INPUT,
                message="the sealed input needs a fetch URL and its digest",
            )
        )

    body = fetch_verified(fetch_url, digest)
    try:
        document = json.loads(body)
        turns = document["turns"]
        competencies = document["competencies"]
    except (json.JSONDecodeError, KeyError, TypeError) as error:
        raise FailureError(
            Failure(
                code=FailureCode.INVALID_INPUT,
                message="the sealed input is not the evaluation document shape",
            )
        ) from error

    return extract_evidence(turns, competencies)
