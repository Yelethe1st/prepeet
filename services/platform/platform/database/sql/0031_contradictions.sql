-- 0031: contradiction pairs, both sides quoted with timestamps.
--
-- EVL-04's storage. A contradiction is two candidate statements about one
-- subject whose numbers conflict, stored as ONE row with both sides quoted
-- exactly and clocked on the room clock, so any surface that shows one can
-- always show both sides in the candidate's own words. The vocabulary is
-- deliberately descriptive: nothing in this table records or implies a
-- judgment about honesty, integrity or credibility - the pair is a prompt
-- for clarification, framed by server-supplied copy at every surface.
--
-- Same lifecycle as evidence_spans: validated against the sealed input
-- before storage, replaced wholesale per (session, extraction version) so
-- a retried stage converges, and never edited in place.
--
-- Implements part of EVL-04.

CREATE TABLE evaluation.contradictions (
    id                 uuid        PRIMARY KEY,
    session_id         uuid        NOT NULL,
    mode               text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    candidate_id       uuid        NOT NULL,
    tenant_id          uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),

    -- The shared subject tokens that made the two statements comparable,
    -- so a reviewer can see WHY the pair was made.
    topic              jsonb       NOT NULL,

    -- Side A: exact quote, character range in its segment, room clock.
    a_segment_sequence integer     NOT NULL CHECK (a_segment_sequence >= 1),
    a_quote            text        NOT NULL,
    a_char_start       integer     NOT NULL CHECK (a_char_start >= 0),
    a_char_end         integer     NOT NULL CHECK (a_char_end > a_char_start),
    a_start_ms         integer     NOT NULL CHECK (a_start_ms >= 0),
    a_end_ms           integer     NOT NULL CHECK (a_end_ms > a_start_ms),

    -- Side B, identically.
    b_segment_sequence integer     NOT NULL CHECK (b_segment_sequence >= 1),
    b_quote            text        NOT NULL,
    b_char_start       integer     NOT NULL CHECK (b_char_start >= 0),
    b_char_end         integer     NOT NULL CHECK (b_char_end > b_char_start),
    b_start_ms         integer     NOT NULL CHECK (b_start_ms >= 0),
    b_end_ms           integer     NOT NULL CHECK (b_end_ms > b_start_ms),

    extraction_version text        NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE evaluation.contradictions IS
    'Two candidate statements that appear to conflict, both sides quoted '
    'with timestamps. A clarification prompt, never a judgment: no honesty '
    'or credibility inference exists anywhere in this pipeline.';

CREATE INDEX contradictions_session_idx
    ON evaluation.contradictions (session_id);

CREATE OR REPLACE FUNCTION evaluation.refuse_contradiction_update()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'evaluation.contradictions rows are never edited; regeneration replaces them';
END $$;

CREATE TRIGGER contradictions_no_update
    BEFORE UPDATE ON evaluation.contradictions
    FOR EACH ROW EXECUTE FUNCTION evaluation.refuse_contradiction_update();

ALTER TABLE evaluation.contradictions ENABLE ROW LEVEL SECURITY;
ALTER TABLE evaluation.contradictions FORCE ROW LEVEL SECURITY;

CREATE POLICY contradictions_tenant ON evaluation.contradictions
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY contradictions_practice_owner ON evaluation.contradictions
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT, DELETE ON evaluation.contradictions TO prepeet_app;
