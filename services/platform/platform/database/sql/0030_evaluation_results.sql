-- 0030: the evaluation result, one per session, immutable.
--
-- EVL-02. The result records what judged it - rubric reference, version
-- and digest as pinned in the session's bundle, aggregation and extraction
-- versions, model and policy versions - beside the competency outcomes,
-- so "what was this person judged by" is a row, forever. One row per
-- session is the retry guarantee's shape: a re-run converges on the
-- existing result, and a re-evaluation (a governed act, later) is a NEW
-- linked row, never a rewrite.

CREATE TABLE evaluation.results (
    id                  uuid        PRIMARY KEY,
    session_id          uuid        NOT NULL UNIQUE,
    mode                text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    candidate_id        uuid        NOT NULL,
    tenant_id           uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),

    rubric_reference    text        NOT NULL,
    rubric_version      text        NOT NULL,
    rubric_digest       text        NOT NULL,

    aggregation_version text        NOT NULL,
    extraction_version  text        NOT NULL,
    model_version       text        NOT NULL DEFAULT 'none',
    policy_version      text        NOT NULL DEFAULT 'none',

    -- The competency outcomes, as aggregate-N serialized them; the digest
    -- is over exactly these bytes, so the published event and this row can
    -- never describe different results.
    competencies        jsonb       NOT NULL,
    result_digest       text        NOT NULL,

    covered_competencies integer    NOT NULL CHECK (covered_competencies >= 0),
    total_competencies   integer    NOT NULL CHECK (total_competencies >= covered_competencies),
    warnings             jsonb      NOT NULL DEFAULT '[]'::jsonb,

    created_at          timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE evaluation.results IS
    'One evaluation per session, immutable, recording everything that '
    'judged it. Re-evaluation is a new linked row under a governed reason, '
    'never a rewrite.';

CREATE OR REPLACE FUNCTION evaluation.refuse_result_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'evaluation.results is immutable: re-evaluation is a new linked row';
END $$;

CREATE TRIGGER results_immutable
    BEFORE UPDATE OR DELETE ON evaluation.results
    FOR EACH ROW EXECUTE FUNCTION evaluation.refuse_result_change();

ALTER TABLE evaluation.results ENABLE ROW LEVEL SECURITY;
ALTER TABLE evaluation.results FORCE ROW LEVEL SECURITY;

CREATE POLICY results_tenant ON evaluation.results
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY results_practice_owner ON evaluation.results
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT ON evaluation.results TO prepeet_app;
