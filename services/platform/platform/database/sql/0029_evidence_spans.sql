-- 0029: evidence spans, and the seal's evaluation-input digest.
--
-- EVL-01. Evidence is a span of what was actually said, linked to a
-- competency, with timing that resolves back to the audio and the version
-- of the extractor that found it. Spans are replaced wholesale per
-- (session, extraction_version) when the stage retries - the extractor is
-- deterministic, so a retry converges - and the table refuses updates:
-- a regeneration is a delete-and-insert inside one transaction, never an
-- edit of rows something may have read.
--
-- The seal gains the digest of the evaluation-input object it wrote: the
-- exact bytes Python was given, verifiable forever.

ALTER TABLE interview.seals
    ADD COLUMN evaluation_input_digest text NOT NULL DEFAULT '';

COMMENT ON COLUMN interview.seals.evaluation_input_digest IS
    'SHA-256 of the evaluation-input object written at completion: the '
    'turns and competencies exactly as evaluation received them.';

CREATE SCHEMA IF NOT EXISTS evaluation;
GRANT USAGE ON SCHEMA evaluation TO prepeet_app;

CREATE TABLE evaluation.evidence_spans (
    id                 uuid        PRIMARY KEY,
    session_id         uuid        NOT NULL,
    mode               text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    candidate_id       uuid        NOT NULL,
    tenant_id          uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),

    competency_id      text        NOT NULL,
    kind               text        NOT NULL CHECK (kind IN
                           ('supporting', 'contradictory', 'claim_unverified', 'gap')),

    -- The provenance: which sealed segment, the exact quoted text, its
    -- character range inside that segment, and its span on the room clock.
    segment_sequence   integer     NOT NULL CHECK (segment_sequence >= 1),
    quote              text        NOT NULL,
    char_start         integer     NOT NULL CHECK (char_start >= 0),
    char_end           integer     NOT NULL CHECK (char_end > char_start),
    start_ms           integer     NOT NULL CHECK (start_ms >= 0),
    end_ms             integer     NOT NULL CHECK (end_ms > start_ms),

    extraction_version text        NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE evaluation.evidence_spans IS
    'Evidence linked to what was actually said: quote, character range, '
    'room-clock timing, competency and extractor version. Validated against '
    'the sealed transcript before storage; a fabricated span never lands.';

CREATE INDEX evidence_spans_session_idx
    ON evaluation.evidence_spans (session_id, competency_id);

CREATE OR REPLACE FUNCTION evaluation.refuse_span_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'evaluation.evidence_spans rows are never edited; regeneration replaces them';
END $$;

CREATE TRIGGER evidence_spans_no_update
    BEFORE UPDATE ON evaluation.evidence_spans
    FOR EACH ROW EXECUTE FUNCTION evaluation.refuse_span_update();

ALTER TABLE evaluation.evidence_spans ENABLE ROW LEVEL SECURITY;
ALTER TABLE evaluation.evidence_spans FORCE ROW LEVEL SECURITY;

CREATE POLICY evidence_tenant ON evaluation.evidence_spans
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY evidence_practice_owner ON evaluation.evidence_spans
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT, DELETE ON evaluation.evidence_spans TO prepeet_app;
