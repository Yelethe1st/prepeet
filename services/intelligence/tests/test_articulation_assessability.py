"""ART-02: an explicit status for every way delivery can be unmeasurable.

Clipping, insufficient speech and low transcript confidence each name
themselves; a not-assessable result says plainly that it is not a low
result and touched no score; and the recording's quality is measured on
samples, never guessed.
"""

from __future__ import annotations

import json

from prepeet_ai.articulation.features import (
    CLIPPING_FLOOR,
    NOT_A_LOW_RESULT,
    audio_quality,
    turn_features,
)
from prepeet_ai.articulation.service import analysis_from_ref


class TestEachCauseHasItsOwnStatus:
    """The first box: three causes, three names."""

    def test_clipping_is_named_from_the_samples(self) -> None:
        """One in twenty samples on the rail is clipped; one in a thousand is not."""
        clean = [0.3 * ((-1) ** i) for i in range(1000)]
        assert audio_quality(clean).status == "assessable"

        clipped = clean[:950] + [1.0] * 50
        result = audio_quality(clipped)
        assert result.status == "not_assessable"
        assert "AUDIO_CLIPPED" in result.warnings
        assert result.clipping_ratio > CLIPPING_FLOOR

    def test_silence_is_named_not_measured_as_slow_speech(self) -> None:
        """A near-silent recording has no speech to time."""
        result = audio_quality([0.0] * 990 + [0.5] * 10)
        assert result.status == "not_assessable"
        assert "AUDIO_SILENT" in result.warnings

    def test_insufficient_speech_and_low_confidence_keep_their_names(self) -> None:
        """The transcript-side causes from ART-01 remain distinct statuses."""
        thin = turn_features(
            {
                "sequence": 1,
                "speaker": "candidate",
                "text": "yes",
                "start_ms": 0,
                "end_ms": 1000,
                "words": [{"w": "yes", "start_ms": 0, "end_ms": 300, "confidence": 0.9}],
            }
        )
        assert "INSUFFICIENT_SPEECH" in thin.warnings
        words = [
            {"w": f"w{i}", "start_ms": i * 400, "end_ms": i * 400 + 300, "confidence": 0.2}
            for i in range(25)
        ]
        unsure = turn_features(
            {
                "sequence": 2,
                "speaker": "candidate",
                "text": " ".join(w["w"] for w in words),
                "start_ms": 0,
                "end_ms": 12000,
                "words": words,
            }
        )
        assert "TRANSCRIPT_CONFIDENCE_LOW" in unsure.warnings
        assert unsure.status == "not_assessable"


class TestTheStatementRidesTheResult:
    """The second box: server-supplied, present exactly when it applies."""

    def test_a_not_assessable_analysis_says_it_is_not_a_low_result(self, monkeypatch) -> None:
        """Delivered in the document, not left to a screen to remember."""
        document = json.dumps(
            {
                "session_id": "ses-9",
                "turns": [
                    {
                        "sequence": 3,
                        "speaker": "candidate",
                        "text": "just a few words",
                        "start_ms": 0,
                        "end_ms": 2000,
                        "words": [{"w": "just", "start_ms": 0, "end_ms": 300, "confidence": 0.9}],
                    }
                ],
            }
        ).encode()
        monkeypatch.setattr(
            "prepeet_ai.articulation.service.fetch_verified", lambda url, digest: document
        )

        analysis = json.loads(analysis_from_ref("http://x/input.json", "sha256:whatever"))

        assert analysis["assessability"]["status"] == "not_assessable"
        assert analysis["assessability"]["note"] == NOT_A_LOW_RESULT
        assert "not a low result" in NOT_A_LOW_RESULT
        assert "not affected any score" in NOT_A_LOW_RESULT
