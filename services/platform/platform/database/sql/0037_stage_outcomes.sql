-- 0037: what each evaluation stage did, and what it cost.
--
-- EVL-07. One row per stage attempt, append-only: a stage that failed and
-- was retried keeps both rows, because "it failed once and then worked"
-- and "it worked first time" are different facts and an operator needs to
-- tell them apart. The latest row per stage is the stage's standing; the
-- sum of cost_units per stage is what it has spent against the budget its
-- pinned policy allows (ADR-0019).
--
-- The distinction the whole ticket turns on is a column here: required
-- says whether the result could be produced without this stage, so an
-- optional stage failing or being omitted can be shown as an absence
-- rather than a broken evaluation. retryable says whether the operator
-- should expect it to resolve on its own.
--
-- Implements part of EVL-07.

CREATE TABLE evaluation.stage_outcomes (
    id           uuid        PRIMARY KEY,
    session_id   uuid        NOT NULL,
    mode         text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    candidate_id uuid        NOT NULL,
    tenant_id    uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),

    stage        text        NOT NULL,
    status       text        NOT NULL CHECK (status IN ('completed', 'omitted', 'failed')),

    -- Why, when it is not completed: a failure code, or an omission
    -- reason such as BUDGET_EXHAUSTED. Empty on success.
    reason       text        NOT NULL DEFAULT '',
    retryable    boolean     NOT NULL DEFAULT false,
    required     boolean     NOT NULL,
    cost_units   integer     NOT NULL DEFAULT 0 CHECK (cost_units >= 0),

    created_at   timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE evaluation.stage_outcomes IS
    'Append-only record of every evaluation stage attempt: what it did, '
    'why not, whether it was required, whether it is worth retrying, and '
    'what it spent. The latest row per stage is that stage''s standing.';

CREATE INDEX stage_outcomes_by_session
    ON evaluation.stage_outcomes (session_id, stage, created_at);

CREATE OR REPLACE FUNCTION evaluation.refuse_stage_outcome_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'evaluation.stage_outcomes is append-only: a later attempt is a new row';
END $$;

CREATE TRIGGER stage_outcomes_append_only
    BEFORE UPDATE OR DELETE ON evaluation.stage_outcomes
    FOR EACH ROW EXECUTE FUNCTION evaluation.refuse_stage_outcome_change();

ALTER TABLE evaluation.stage_outcomes ENABLE ROW LEVEL SECURITY;
ALTER TABLE evaluation.stage_outcomes FORCE ROW LEVEL SECURITY;

CREATE POLICY stage_outcomes_tenant ON evaluation.stage_outcomes
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY stage_outcomes_practice_owner ON evaluation.stage_outcomes
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT ON evaluation.stage_outcomes TO prepeet_app;
