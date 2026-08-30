-- 0042: aggregate feedback by what generated the insight, not by the session.
--
-- 0038 said artifact_digest was the aggregation key and ART-09 asked for a
-- drop in helpfulness to be attributable to an artifact version rather than to
-- a date. What was written into it was the seal's evaluation-input digest: the
-- candidate's own transcript document, which is unique per session. Grouping
-- by it puts every candidate in a group of one, so the index that made the
-- table worth having answered nothing at all.
--
-- The column now holds the coaching version that produced the insight, which
-- is a governed revision and shared across every session it generated. The
-- transcript digest keeps its own column, because knowing which analysis a
-- verdict was about is still worth having; it is provenance rather than a key.
--
-- Existing rows carried the wrong thing in the aggregation column and there is
-- no way to recover the right one from them, so they move: the digest they
-- held was always the input, and the artifact is left unknown rather than
-- guessed.
--
-- Implements part of ART-09.

ALTER TABLE evaluation.insight_feedback
    ADD COLUMN input_digest text NOT NULL DEFAULT '';

UPDATE evaluation.insight_feedback
SET input_digest = artifact_digest, artifact_digest = 'unknown'
WHERE artifact_digest LIKE 'sha256:%';

COMMENT ON COLUMN evaluation.insight_feedback.artifact_digest IS
    'The governed revision that generated the insight, such as a coaching '
    'version. Shared across every session it generated, which is what makes '
    'the rate per artifact answerable.';

COMMENT ON COLUMN evaluation.insight_feedback.input_digest IS
    'The transcript document the insight was about. Provenance, never a key: '
    'it is unique per session, so grouping by it groups nothing.';
