-- 0051: the candidate's own goals, and the milestones they pass.
--
-- PRG-03. A goal is a target the candidate set for themselves, which is a
-- different kind of thing from a readiness snapshot: readiness is what a
-- role standard says about them, and a goal is what they decided to work
-- on. The schema keeps that difference visible in three ways, each of
-- which is a property the ticket asks for rather than a convention.
--
--   1. There is no tenant_id and no mode. A goal belongs to the person, and
--      the only policy on these tables is keyed to the candidate with no
--      tenant context set, so there is no employer authority under which a
--      goal exists to be read or written. Practice data is candidate-owned;
--      migration 0037 has the dual pattern for data that is genuinely both,
--      and this is deliberately not that.
--   2. A goal pins its own band scale and rubric reference at creation.
--      Progress against a scale that could move under it would not be
--      progress, and pinning is what lets a goal survive a rubric version
--      change: a bump inside one reference keeps counting, and a different
--      reference is reported as incomparable rather than as a reset.
--   3. Milestones are append-only, one per band, each naming the rubric
--      version and the observation that earned it. A milestone records
--      what a candidate did on a day. A later publication can change what
--      the current reading means; it cannot take away what already
--      happened.
--
-- What is deliberately absent is as much of the design as what is here.
-- There is no streak column, no missed count, no target cadence and no
-- last-nagged-at. Practice cadence is derived from the observation history
-- the candidate already has, so there is nothing here that could be
-- rendered as a reproach, and "streaks must not become punitive
-- gamification" is held by the absence of anywhere to keep the score.
--
-- Implements PRG-03.

CREATE TABLE progression.goals (
    id               uuid        PRIMARY KEY,

    -- The owner, and the whole of the owner. No tenant column exists here
    -- by design; see the header.
    candidate_id     uuid        NOT NULL,

    -- Where the goal came from, so it can explain itself on a screen and
    -- in a review. A goal raised from a readiness gap, one adopted from a
    -- drill and one chosen outright are different stories.
    origin           text        NOT NULL
                                 CHECK (origin IN ('gap', 'drill', 'competency')),
    origin_reference text        NOT NULL DEFAULT '',

    competency_id    text        NOT NULL CHECK (competency_id <> ''),
    target_band      text        NOT NULL CHECK (target_band <> ''),

    -- The comparability basis and the scale, both pinned. A goal measured
    -- on a scale it does not carry is a goal whose meaning depends on
    -- whatever the registry publishes next.
    rubric_reference text        NOT NULL CHECK (rubric_reference <> ''),
    bands            text[]      NOT NULL CHECK (array_length(bands, 1) > 0),

    status           text        NOT NULL DEFAULT 'active'
                                 CHECK (status IN ('active', 'paused', 'retired')),

    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),

    -- The target band has to be somewhere on the goal's own scale, or
    -- reaching it is not a question the data can answer.
    CONSTRAINT target_band_is_on_the_scale CHECK (target_band = ANY (bands))
);

COMMENT ON TABLE progression.goals IS
    'A target the candidate set for themselves, pinned to the band scale '
    'and rubric reference it is measured against. Candidate-owned: there '
    'is no tenant column, so no employer authority reaches a goal.';

CREATE INDEX goals_by_candidate
    ON progression.goals (candidate_id, status, competency_id);

-- Status is the only thing a goal may change about itself.
--
-- Editing what a goal measures would silently re-date every milestone
-- already earned under the old target, so a change of subject is a new
-- goal. Pausing and retiring are the edits that make sense, and they are
-- the only ones the trigger admits.
CREATE OR REPLACE FUNCTION progression.refuse_goal_subject_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.id, NEW.candidate_id, NEW.origin, NEW.origin_reference,
        NEW.competency_id, NEW.target_band, NEW.rubric_reference,
        NEW.bands, NEW.created_at)
       IS DISTINCT FROM
       (OLD.id, OLD.candidate_id, OLD.origin, OLD.origin_reference,
        OLD.competency_id, OLD.target_band, OLD.rubric_reference,
        OLD.bands, OLD.created_at) THEN
        RAISE EXCEPTION 'a goal''s subject is fixed: change the status, or set a new goal';
    END IF;
    IF OLD.status = 'retired' AND NEW.status <> 'retired' THEN
        RAISE EXCEPTION 'a retired goal stays retired: set a new goal instead';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END $$;

CREATE TRIGGER goals_subject_immutable
    BEFORE UPDATE ON progression.goals
    FOR EACH ROW EXECUTE FUNCTION progression.refuse_goal_subject_change();

CREATE TABLE progression.goal_milestones (
    goal_id          uuid        NOT NULL REFERENCES progression.goals (id),

    -- Repeated rather than reached through the parent, because row-level
    -- security is per table and a policy that has to join is a policy that
    -- can be got round.
    candidate_id     uuid        NOT NULL,

    band             text        NOT NULL CHECK (band <> ''),

    -- What judged it. A milestone outlives the rubric version that
    -- produced it, which is only true if the version travels with it.
    rubric_reference text        NOT NULL CHECK (rubric_reference <> ''),
    rubric_version   text        NOT NULL CHECK (rubric_version <> ''),
    observation_id   uuid        NOT NULL REFERENCES progression.observations (id),

    -- When the candidate reached it, not when anybody noticed. A milestone
    -- dated to the moment the job ran would make a chart of achievements
    -- into a chart of how often the job ran.
    reached_at       timestamptz NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),

    -- A band is an achievement once. Reaching solid twice is one thing
    -- that happened, and a second row would reward repetition.
    PRIMARY KEY (goal_id, band)
);

COMMENT ON TABLE progression.goal_milestones IS
    'Append-only: a band this goal reached, once, with the observation and '
    'rubric version that showed it. A later rubric cannot take away what a '
    'candidate already did.';

CREATE INDEX goal_milestones_by_candidate
    ON progression.goal_milestones (candidate_id, goal_id, reached_at);

CREATE OR REPLACE FUNCTION progression.refuse_milestone_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'progression.goal_milestones is append-only: what happened, happened';
END $$;

CREATE TRIGGER goal_milestones_append_only
    BEFORE UPDATE OR DELETE ON progression.goal_milestones
    FOR EACH ROW EXECUTE FUNCTION progression.refuse_milestone_change();

ALTER TABLE progression.goals ENABLE ROW LEVEL SECURITY;
ALTER TABLE progression.goals FORCE ROW LEVEL SECURITY;

-- One policy, and it names a person. Under FORCE, a request carrying a
-- tenant context matches nothing here at all, which is the point: an
-- employer has no authority over what a candidate decided to work on.
CREATE POLICY goals_candidate_owner ON progression.goals
    USING (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

ALTER TABLE progression.goal_milestones ENABLE ROW LEVEL SECURITY;
ALTER TABLE progression.goal_milestones FORCE ROW LEVEL SECURITY;

CREATE POLICY goal_milestones_candidate_owner ON progression.goal_milestones
    USING (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

-- UPDATE on goals is the pause and retire path, narrowed to the status
-- column by the trigger above. Milestones get no update grant at all.
GRANT SELECT, INSERT, UPDATE ON progression.goals TO prepeet_app;
GRANT SELECT, INSERT ON progression.goal_milestones TO prepeet_app;
