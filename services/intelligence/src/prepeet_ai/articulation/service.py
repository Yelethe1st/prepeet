"""The articulation capability's boundary: fetch the sealed input, measure.

The request names a manifest by reference and digest, exactly like every
other pinned read; the calculator runs only over verified bytes. At this
floor the manifest IS the sealed evaluation input (turns with word
timings), which carries everything articulation-features-v1 measures;
audio-derived quality joins when the recording is decoded here, and the
result says so until then rather than inventing a number.
"""

from __future__ import annotations

import dataclasses
import json

from prepeet_ai.articulation.features import CALCULATION_VERSION, session_features
from prepeet_ai.extraction.service import fetch_verified
from prepeet_ai.transport.envelope import Failure, FailureCode, FailureError

SCHEMA_VERSION = "1.0"
POLICY_VERSION = "articulation-practice-v1"

__all__ = ["CALCULATION_VERSION", "POLICY_VERSION", "SCHEMA_VERSION", "analysis_from_ref"]


def analysis_from_ref(fetch_url: str, digest: str) -> bytes:
    """Fetch the sealed input and answer the analysis document, JSON encoded.

    Raises:
        FailureError: INVALID_INPUT for a request missing its grant or a
            document that is not the sealed shape; ARTIFACT_NOT_FOUND from
            the verified fetch.
    """
    if not fetch_url or not digest:
        raise FailureError(
            Failure(
                code=FailureCode.INVALID_INPUT,
                message="the manifest needs a fetch URL and its digest",
            )
        )
    body = fetch_verified(fetch_url, digest)
    try:
        document = json.loads(body)
        turns = list(document["turns"])
    except (json.JSONDecodeError, KeyError, TypeError) as error:
        raise FailureError(
            Failure(
                code=FailureCode.INVALID_INPUT,
                message="the manifest is not the sealed evaluation document shape",
            )
        ) from error

    features = session_features(turns)
    analysis = {
        "schema_version": SCHEMA_VERSION,
        "calculation_version": CALCULATION_VERSION,
        "policy_version": POLICY_VERSION,
        "assessability": {
            "status": features.status,
            "audio_quality": None,
            "transcript_confidence": features.transcript_confidence,
            "warnings": list(features.warnings),
        },
        "metrics": {
            "words": features.words,
            "words_per_minute": features.words_per_minute,
            "fillers_per_100_words": features.fillers_per_100_words,
            "long_pause_count": features.long_pause_count,
        },
        "turns": [dataclasses.asdict(turn) for turn in features.turns],
    }
    return json.dumps(analysis, sort_keys=True, separators=(",", ":")).encode()
