-- 0035: articulation results, one per session, beside the evaluation.
--
-- ART-02. Delivery is measured by its own workflow and stored in its own
-- row, so a failed or unassessable delivery analysis can never touch the
-- content evaluation: the two share a session id and nothing else. The
-- status and its warnings are the row's whole point - not_assessable is a
-- fact to show with its plain statement, never a low value - and the
-- analysis document carries every measurement with the calculator that
-- produced it. Immutable like every other judgment: re-analysis is a new
-- row under a governed reason, which ART-06's redo comparison will need.
--
-- Implements part of ART-02.

CREATE TABLE evaluation.articulation (
    id                  uuid        PRIMARY KEY,
    session_id          uuid        NOT NULL UNIQUE,
    mode                text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    candidate_id        uuid        NOT NULL,
    tenant_id           uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),

    status              text        NOT NULL CHECK (status IN
                            ('assessable', 'partially_assessable', 'not_assessable')),
    warnings            jsonb       NOT NULL DEFAULT '[]'::jsonb,
    analysis            jsonb       NOT NULL,

    calculation_version text        NOT NULL,
    policy_version      text        NOT NULL,
    input_digest        text        NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE evaluation.articulation IS
    'Delivery measurements per session with their assessability. Separate '
    'from evaluation.results by design so delivery can fail without '
    'touching content evaluation. Immutable.';

CREATE OR REPLACE FUNCTION evaluation.refuse_articulation_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'evaluation.articulation is immutable: re-analysis is a new row';
END $$;

CREATE TRIGGER articulation_immutable
    BEFORE UPDATE OR DELETE ON evaluation.articulation
    FOR EACH ROW EXECUTE FUNCTION evaluation.refuse_articulation_change();

ALTER TABLE evaluation.articulation ENABLE ROW LEVEL SECURITY;
ALTER TABLE evaluation.articulation FORCE ROW LEVEL SECURITY;

CREATE POLICY articulation_tenant ON evaluation.articulation
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY articulation_practice_owner ON evaluation.articulation
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT ON evaluation.articulation TO prepeet_app;
