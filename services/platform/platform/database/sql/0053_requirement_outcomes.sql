-- 0053: what sessions said about personal requirements, and the
-- candidate's own confidence rating, which is a different kind of thing.
--
-- PRG-06's second half. Three arguments are worth making here because
-- they are the ones a later reader is most likely to undo.
--
-- Not assessable is a shape, not a low score. A session that never created
-- a fair opportunity to demonstrate something has said nothing about the
-- candidate, so the CHECK constraints below refuse a not-assessable row
-- that lists anything as missing or demonstrated, and refuse an assessed
-- row that carries a reason. There is no way in this schema to spell "we
-- never asked" as a zero, and no column a percentage could be computed
-- from without noticing the difference.
--
-- Confidence is a table of its own with no rubric, no band, no criterion
-- and no evidence column. It cannot be joined into an evaluated reading
-- for want of anything to join on, which is how "confidence is never
-- inferred from delivery" stays true under maintenance rather than under
-- supervision. It is the candidate's own optional self-rating and nothing
-- else.
--
-- These tables admit DELETE, which nothing else in progression does.
-- Practice requirement history is the candidate's private evidence about
-- themselves: no employer can see it, no decision rests on it, and no
-- audit obligation attaches to it, so the candidate's right to erase it
-- has nothing to weigh against. Screening observations are the opposite
-- case and stay append-only. UPDATE is still refused, because editing a
-- recorded result is not erasure and has no honest use.
--
-- Implements part of PRG-06.

CREATE TABLE progression.requirement_outcomes (
    id                uuid        PRIMARY KEY,
    requirement_id    uuid        NOT NULL
                                  REFERENCES progression.personal_requirements (id)
                                  ON DELETE CASCADE,
    candidate_id      uuid        NOT NULL,

    -- What judged it. An outcome without its criterion version is a result
    -- whose meaning depends on whatever the requirement says today.
    criterion_version integer     NOT NULL CHECK (criterion_version >= 1),
    session_id        uuid        NOT NULL,

    -- The comparison basis. Two sessions for different roles or in
    -- different shapes are not two readings of one thing, and carrying the
    -- basis on the row is what lets a metric keep them apart.
    role_id           text        NOT NULL DEFAULT '',
    shape_id          text        NOT NULL DEFAULT '',

    outcome           text        NOT NULL
                                  CHECK (outcome IN ('achieved', 'partially_achieved',
                                                     'not_demonstrated', 'not_assessable')),

    reason            text        NOT NULL DEFAULT ''
                                  CHECK (reason IN ('', 'no_fair_opportunity',
                                                    'no_evidence_offered')),

    demonstrated      text[]      NOT NULL DEFAULT '{}',
    missing           text[]      NOT NULL DEFAULT '{}',
    evidence          text[]      NOT NULL DEFAULT '{}',
    next_actions      text[]      NOT NULL DEFAULT '{}',

    observed_at       timestamptz NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),

    -- One outcome per requirement per session: a redelivered projection
    -- converges rather than counting one session twice in a metric.
    UNIQUE (session_id, requirement_id),

    -- Not assessable states why, and every other outcome does not. The
    -- equality rather than two ORs, so neither state can borrow half of
    -- the other's fields.
    CONSTRAINT not_assessable_states_why
        CHECK ((outcome = 'not_assessable') = (reason <> '')),

    -- And it says nothing about the candidate. A not-assessable row that
    -- listed a criterion as missing would be a failure with a gentler
    -- name, which is exactly what this ticket forbids.
    CONSTRAINT not_assessable_blames_nobody
        CHECK (outcome <> 'not_assessable'
               OR (cardinality(demonstrated) = 0 AND cardinality(missing) = 0)),

    -- The three assessed outcomes each mean something about the criteria,
    -- and the schema holds them to it: achieved means nothing was missing,
    -- not demonstrated means nothing was shown, partial means both.
    CONSTRAINT achieved_has_nothing_missing
        CHECK (outcome <> 'achieved'
               OR (cardinality(missing) = 0 AND cardinality(demonstrated) > 0)),
    CONSTRAINT not_demonstrated_shows_nothing
        CHECK (outcome <> 'not_demonstrated'
               OR (cardinality(demonstrated) = 0 AND cardinality(missing) > 0)),
    CONSTRAINT partial_shows_some_and_misses_some
        CHECK (outcome <> 'partially_achieved'
               OR (cardinality(demonstrated) > 0 AND cardinality(missing) > 0))
);

COMMENT ON TABLE progression.requirement_outcomes IS
    'One session''s answer about one personal requirement, against the '
    'criterion version that judged it. Not assessable carries a reason and '
    'blames nobody: there is no way here to spell "we never asked" as a '
    'zero.';

CREATE INDEX requirement_outcomes_by_candidate
    ON progression.requirement_outcomes
       (candidate_id, requirement_id, criterion_version, observed_at);

CREATE OR REPLACE FUNCTION progression.refuse_outcome_edit()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'a requirement outcome cannot be edited: erase it or leave it';
END $$;

CREATE TRIGGER requirement_outcomes_no_edit
    BEFORE UPDATE ON progression.requirement_outcomes
    FOR EACH ROW EXECUTE FUNCTION progression.refuse_outcome_edit();

CREATE TABLE progression.confidence_self_reports (
    candidate_id uuid        NOT NULL,
    session_id   uuid        NOT NULL,

    -- Before or after, because the pair is the point: a candidate
    -- comparing their own two numbers is doing something the system never
    -- does for them.
    phase        text        NOT NULL CHECK (phase IN ('before', 'after')),

    -- One to five, the candidate's own. There is deliberately no rubric,
    -- no band, no criterion and no evidence column on this table: nothing
    -- here can be joined to an evaluated reading, so nothing here can
    -- become one.
    rating       smallint    NOT NULL CHECK (rating BETWEEN 1 AND 5),

    reported_at  timestamptz NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (candidate_id, session_id, phase)
);

COMMENT ON TABLE progression.confidence_self_reports IS
    'The candidate''s own optional confidence rating, before or after a '
    'session. Never inferred, never joined to an evaluated observation, '
    'and carrying no rubric or evidence column that would let it become '
    'one.';

CREATE TABLE progression.personalisation (
    candidate_id uuid        PRIMARY KEY,

    -- Off means prior practice evidence must not shape a recommendation.
    -- A row exists only once somebody has expressed a preference; absence
    -- means the default, which is on.
    enabled      boolean     NOT NULL DEFAULT true,

    updated_at   timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE progression.personalisation IS
    'Whether prior practice evidence may shape this candidate''s '
    'recommendations. The candidate''s switch, not the product''s.';

ALTER TABLE progression.requirement_outcomes ENABLE ROW LEVEL SECURITY;
ALTER TABLE progression.requirement_outcomes FORCE ROW LEVEL SECURITY;

CREATE POLICY requirement_outcomes_candidate_owner ON progression.requirement_outcomes
    USING (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

ALTER TABLE progression.confidence_self_reports ENABLE ROW LEVEL SECURITY;
ALTER TABLE progression.confidence_self_reports FORCE ROW LEVEL SECURITY;

CREATE POLICY confidence_self_reports_candidate_owner ON progression.confidence_self_reports
    USING (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

ALTER TABLE progression.personalisation ENABLE ROW LEVEL SECURITY;
ALTER TABLE progression.personalisation FORCE ROW LEVEL SECURITY;

CREATE POLICY personalisation_candidate_owner ON progression.personalisation
    USING (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT, DELETE ON progression.requirement_outcomes TO prepeet_app;
GRANT SELECT, INSERT, DELETE ON progression.confidence_self_reports TO prepeet_app;
GRANT SELECT, INSERT, UPDATE ON progression.personalisation TO prepeet_app;
