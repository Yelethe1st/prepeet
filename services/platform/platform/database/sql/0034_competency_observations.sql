-- 0034: competency observations, append-only with rubric provenance.
--
-- PRG-01. Progression is a projection over what evaluation published:
-- one row per evaluated competency per evaluation, carrying everything
-- that judged it - rubric reference, version and digest, aggregation and
-- extraction versions, model and policy versions - so any historical
-- point on a progression chart can be reconstructed against exactly what
-- produced it.
--
-- Append-only is the whole design: rows are never updated or deleted, a
-- correction is a NEW row naming what it supersedes, and a re-evaluation
-- under a new rubric adds its own rows while the earlier view stands.
-- Reading picks the latest per (session, competency); history keeps every
-- version. The trigger refuses the alternative.
--
-- Implements PRG-01.

GRANT USAGE ON SCHEMA progression TO prepeet_app;

CREATE TABLE progression.observations (
    id             uuid        PRIMARY KEY,
    candidate_id   uuid        NOT NULL,
    mode           text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    tenant_id      uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),

    session_id     uuid        NOT NULL,
    evaluation_id  uuid        NOT NULL,
    competency_id  text        NOT NULL,

    -- The outcome as evaluation published it. Unassessed rows are stored
    -- too: absence of measurement is a fact progression must carry, or a
    -- chart would quietly read silence as decline.
    status         text        NOT NULL CHECK (status IN ('assessed', 'unassessed')),
    band           text        NOT NULL DEFAULT '',
    confidence     text        NOT NULL CHECK (confidence IN ('high', 'medium', 'low', 'not_assessable')),
    evidence_count integer     NOT NULL CHECK (evidence_count >= 0),
    supporting     integer     NOT NULL CHECK (supporting >= 0),
    contradictory  integer     NOT NULL CHECK (contradictory >= 0),
    unverified     integer     NOT NULL CHECK (unverified >= 0),
    gaps           integer     NOT NULL CHECK (gaps >= 0),

    -- The provenance: exactly what judged this observation.
    rubric_reference    text   NOT NULL,
    rubric_version      text   NOT NULL,
    rubric_digest       text   NOT NULL,
    aggregation_version text   NOT NULL,
    extraction_version  text   NOT NULL,
    model_version       text   NOT NULL,
    policy_version      text   NOT NULL,

    -- A correction or re-reading names its predecessor; the predecessor
    -- stands.
    supersedes     uuid        REFERENCES progression.observations (id),

    observed_at    timestamptz NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),

    -- One observation per competency per evaluation: the consumer's
    -- retry converges instead of duplicating history.
    UNIQUE (evaluation_id, competency_id)
);

COMMENT ON TABLE progression.observations IS
    'Append-only competency history with full rubric provenance. '
    'Corrections and re-evaluations add rows naming what they supersede; '
    'nothing is ever edited in place.';

CREATE INDEX observations_by_candidate
    ON progression.observations (candidate_id, competency_id, observed_at);

CREATE OR REPLACE FUNCTION progression.refuse_observation_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'progression.observations is append-only: a correction is a new row';
END $$;

CREATE TRIGGER observations_append_only
    BEFORE UPDATE OR DELETE ON progression.observations
    FOR EACH ROW EXECUTE FUNCTION progression.refuse_observation_change();

ALTER TABLE progression.observations ENABLE ROW LEVEL SECURITY;
ALTER TABLE progression.observations FORCE ROW LEVEL SECURITY;

CREATE POLICY observations_tenant ON progression.observations
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY observations_practice_owner ON progression.observations
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT ON progression.observations TO prepeet_app;
