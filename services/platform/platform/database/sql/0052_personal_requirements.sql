-- 0052: what a candidate asked a session to look for.
--
-- PRG-06. A personal requirement is the candidate's own statement of what
-- they want tested, resolved into observable criteria they can read. Four
-- things the ticket asks for are held here rather than in Go.
--
--   1. There is no tenant_id and no mode, and the only policy is keyed to
--      the person with no tenant context set. "No personal requirement is
--      reachable through employer authority" is then not a rule anybody
--      has to remember: there is no employer scope in which one of these
--      rows exists. Migration 0037 has the dual pattern for data that is
--      genuinely both a tenant's and a candidate's; this is deliberately
--      not that.
--   2. Criteria are immutable per version, append-only, and an edit is a
--      new version. An outcome is reported against a criterion version, so
--      editing criteria in place would silently rewrite the meaning of
--      every result already given.
--   3. The candidate's own words are kept beside the criteria. A
--      resolution is the system's reading of a request, and somebody has
--      to be able to see whether it read them right.
--   4. When a request asked for something the system will not infer and an
--      observable behaviour was offered instead, the requirement carries
--      the reframing and names the declined inference. A reframing the
--      candidate cannot go back and read is a substitution: they would
--      believe the session is looking for what they asked for.
--
-- Implements part of PRG-06.

CREATE TABLE progression.personal_requirements (
    id           uuid        PRIMARY KEY,

    -- The owner, and the whole of the owner. See the header.
    candidate_id uuid        NOT NULL,

    -- The request as the candidate wrote it, never rewritten.
    intent       text        NOT NULL CHECK (intent <> ''),

    status       text        NOT NULL DEFAULT 'draft'
                             CHECK (status IN ('draft', 'active', 'paused', 'retired')),

    -- The version currently in use. Rises with every revision; the
    -- criteria of every earlier version stay readable beside it.
    version      integer     NOT NULL DEFAULT 1 CHECK (version >= 1),

    -- What was declined, and what is being looked for instead. Empty for a
    -- request that needed neither.
    reframing    text        NOT NULL DEFAULT '',
    prohibited   text        NOT NULL DEFAULT '',

    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE progression.personal_requirements IS
    'A candidate''s own statement of what a session should look for, with '
    'the reframing if one was needed. Candidate-owned: no tenant column, '
    'so no employer authority reaches a personal requirement.';

CREATE INDEX personal_requirements_by_candidate
    ON progression.personal_requirements (candidate_id, status, created_at);

-- A requirement's version may only rise, and retiring is final.
--
-- Both for the same reason: an outcome already recorded names a version,
-- so a version that could fall would make an existing result point at
-- criteria that are no longer what it was judged against. Retiring is a
-- decision, and one that can be silently undone is not one.
CREATE OR REPLACE FUNCTION progression.refuse_requirement_regression()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.id <> OLD.id OR NEW.candidate_id <> OLD.candidate_id
       OR NEW.created_at <> OLD.created_at THEN
        RAISE EXCEPTION 'a personal requirement''s identity is fixed';
    END IF;
    IF NEW.version < OLD.version THEN
        RAISE EXCEPTION 'a requirement version cannot fall: outcomes already name it';
    END IF;
    IF OLD.status = 'retired' AND NEW.status <> 'retired' THEN
        RAISE EXCEPTION 'a retired requirement stays retired: write a new one';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END $$;

CREATE TRIGGER personal_requirements_no_regression
    BEFORE UPDATE ON progression.personal_requirements
    FOR EACH ROW EXECUTE FUNCTION progression.refuse_requirement_regression();

CREATE TABLE progression.requirement_criteria (
    requirement_id uuid        NOT NULL
                               REFERENCES progression.personal_requirements (id)
                               ON DELETE CASCADE,

    -- Repeated rather than reached through the parent, because row-level
    -- security is per table and a policy that has to join is a policy that
    -- can be got round.
    candidate_id   uuid        NOT NULL,

    version        integer     NOT NULL CHECK (version >= 1),
    criterion_id   text        NOT NULL CHECK (criterion_id <> ''),

    -- Ordering is stored so the candidate reads the criteria in the order
    -- they were resolved rather than in whatever order a query returns.
    position       integer     NOT NULL CHECK (position >= 0),

    -- The candidate's copy and the evaluator's. Both, because a criterion
    -- somebody cannot read is one they cannot disagree with.
    statement      text        NOT NULL CHECK (statement <> ''),
    observable     text        NOT NULL CHECK (observable <> ''),

    created_at     timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (requirement_id, version, criterion_id)
);

COMMENT ON TABLE progression.requirement_criteria IS
    'Append-only, immutable per version: the observable criteria one '
    'version of a personal requirement resolves to. An edit is a new '
    'version, so an outcome keeps meaning what it meant.';

CREATE INDEX requirement_criteria_by_candidate
    ON progression.requirement_criteria (candidate_id, requirement_id, version);

CREATE OR REPLACE FUNCTION progression.refuse_criteria_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'requirement criteria are immutable per version: an edit is a new version';
END $$;

-- UPDATE only. A candidate deleting their own requirement cascades to its
-- criteria, which is erasure rather than editing, and 0053's header makes
-- the argument for why erasure is allowed here at all.
CREATE TRIGGER requirement_criteria_immutable
    BEFORE UPDATE ON progression.requirement_criteria
    FOR EACH ROW EXECUTE FUNCTION progression.refuse_criteria_change();

ALTER TABLE progression.personal_requirements ENABLE ROW LEVEL SECURITY;
ALTER TABLE progression.personal_requirements FORCE ROW LEVEL SECURITY;

CREATE POLICY personal_requirements_candidate_owner ON progression.personal_requirements
    USING (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

ALTER TABLE progression.requirement_criteria ENABLE ROW LEVEL SECURITY;
ALTER TABLE progression.requirement_criteria FORCE ROW LEVEL SECURITY;

CREATE POLICY requirement_criteria_candidate_owner ON progression.requirement_criteria
    USING (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT, UPDATE, DELETE ON progression.personal_requirements TO prepeet_app;
GRANT SELECT, INSERT, DELETE ON progression.requirement_criteria TO prepeet_app;
