-- 0046: readiness against a pinned role standard.
--
-- PRG-02. A readiness snapshot is what the candidate's history says about
-- one role standard at one moment, and the schema enforces the three
-- things the ticket exists for, because Go that gets them right today is
-- not what a reader of this table in a year will be relying on.
--
--   1. The snapshot names the standard it was computed against, by
--      reference, version and digest, none of which may be empty. A
--      readiness figure whose standard is unknown cannot be audited, and
--      an unauditable number about a person is worse than no number.
--   2. One row per role standard. Role and discipline are columns, not a
--      grouping applied at read time, and nothing here can hold a figure
--      that spans two standards: there is no table for it and no column
--      for it. Incomparable roles cannot be averaged because there is
--      nowhere to put the average.
--   3. Assessed and unassessed stay apart, by CHECK. An unassessed
--      requirement carries no band, no resolving observation and no date,
--      and must state why; a met or below one carries all three and no
--      reason. The two cannot be confused by a later reader, and there is
--      no way to spell a zero for something nobody measured.
--
-- How many requirements were met, missed or unmeasured is deliberately
-- NOT stored. A count beside the rows it summarises is a count that can
-- disagree with them, and the disagreement this ticket fears is exactly
-- the one that would be invisible: an unassessed requirement counted as a
-- pass. Readers count the requirement rows, which cannot lie about
-- themselves.
--
-- Append-only like everything else in this context. Recomputing an
-- unchanged answer converges on the row already written - answer_digest is
-- the identity - and a changed answer appends beside it, so the history is
-- a record of what changed rather than of how often somebody looked.
--
-- Implements PRG-02.

CREATE TABLE progression.readiness_snapshots (
    id                 uuid        PRIMARY KEY,
    candidate_id       uuid        NOT NULL,
    mode               text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    tenant_id          uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),

    -- The pin. Empty is refused rather than defaulted: a snapshot that
    -- cannot say what judged it must not exist at all.
    standard_reference text        NOT NULL CHECK (standard_reference <> ''),
    standard_version   text        NOT NULL CHECK (standard_version <> ''),
    standard_digest    text        NOT NULL CHECK (standard_digest <> ''),

    -- What this readiness is about. Grouping lives in the row so that no
    -- reader has to reconstruct it, and so a query cannot accidentally
    -- gather two disciplines into one answer.
    role_id            text        NOT NULL CHECK (role_id <> ''),
    discipline_id      text        NOT NULL CHECK (discipline_id <> ''),

    -- The comparability basis: readings from another rubric measure
    -- something else and are reported as incomparable, never counted.
    rubric_reference   text        NOT NULL CHECK (rubric_reference <> ''),

    -- SHA-256 over the pin and every resolved requirement. Two identical
    -- answers are one fact, so a recomputation that changed nothing
    -- converges here instead of filling history with duplicates.
    answer_digest      text        NOT NULL CHECK (answer_digest <> ''),

    computed_at        timestamptz NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),

    UNIQUE (candidate_id, standard_reference, answer_digest)
);

COMMENT ON TABLE progression.readiness_snapshots IS
    'Readiness against one pinned role standard, naming the standard, its '
    'version and its digest. One row per standard, and no combined figure '
    'across roles anywhere: incomparable roles cannot be averaged because '
    'there is nowhere to put the average.';

CREATE INDEX readiness_snapshots_by_candidate
    ON progression.readiness_snapshots
       (candidate_id, standard_reference, computed_at DESC);

CREATE TABLE progression.readiness_competencies (
    snapshot_id    uuid        NOT NULL
                               REFERENCES progression.readiness_snapshots (id),

    -- The owner columns are repeated rather than reached through the
    -- parent, because row-level security is per table and a policy that
    -- has to join is a policy that can be got round.
    candidate_id   uuid        NOT NULL,
    mode           text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    tenant_id      uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),

    competency_id  text        NOT NULL CHECK (competency_id <> ''),
    target_band    text        NOT NULL CHECK (target_band <> ''),
    outcome        text        NOT NULL CHECK (outcome IN ('met', 'below', 'unassessed')),

    -- The reading that resolved this requirement, or nothing at all.
    observed_band  text        NOT NULL DEFAULT '',
    observation_id uuid,
    observed_at    timestamptz,

    -- Why, when there is no reading: never_observed, not_assessed,
    -- incomparable_rubric or incomparable_band. Each asks for a different
    -- next session, which is why a single "unknown" would not do.
    reason         text        NOT NULL DEFAULT ''
                               CHECK (reason IN ('', 'never_observed', 'not_assessed',
                                                 'incomparable_rubric', 'incomparable_band')),

    -- Unassessed is a shape, not a value: no band, no observation, no
    -- date, and a stated reason. Assessed is the exact mirror. Four
    -- equalities rather than four ORs, so neither state can borrow half
    -- of the other's fields.
    CONSTRAINT unassessed_carries_no_band
        CHECK ((outcome = 'unassessed') = (observed_band = '')),
    CONSTRAINT unassessed_carries_no_observation
        CHECK ((outcome = 'unassessed') = (observation_id IS NULL)),
    CONSTRAINT unassessed_carries_no_date
        CHECK ((outcome = 'unassessed') = (observed_at IS NULL)),
    CONSTRAINT unassessed_states_why
        CHECK ((outcome = 'unassessed') = (reason <> '')),

    PRIMARY KEY (snapshot_id, competency_id)
);

COMMENT ON TABLE progression.readiness_competencies IS
    'One requirement of one readiness snapshot: met, below, or unassessed '
    'with the reason why. An unassessed requirement carries no band, no '
    'observation and no date, so silence can never be read as a score.';

CREATE OR REPLACE FUNCTION progression.refuse_readiness_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'progression readiness is append-only: a new answer is a new snapshot';
END $$;

CREATE TRIGGER readiness_snapshots_append_only
    BEFORE UPDATE OR DELETE ON progression.readiness_snapshots
    FOR EACH ROW EXECUTE FUNCTION progression.refuse_readiness_change();

CREATE TRIGGER readiness_competencies_append_only
    BEFORE UPDATE OR DELETE ON progression.readiness_competencies
    FOR EACH ROW EXECUTE FUNCTION progression.refuse_readiness_change();

ALTER TABLE progression.readiness_snapshots ENABLE ROW LEVEL SECURITY;
ALTER TABLE progression.readiness_snapshots FORCE ROW LEVEL SECURITY;

CREATE POLICY readiness_snapshots_tenant ON progression.readiness_snapshots
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY readiness_snapshots_practice_owner ON progression.readiness_snapshots
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

ALTER TABLE progression.readiness_competencies ENABLE ROW LEVEL SECURITY;
ALTER TABLE progression.readiness_competencies FORCE ROW LEVEL SECURITY;

CREATE POLICY readiness_competencies_tenant ON progression.readiness_competencies
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY readiness_competencies_practice_owner ON progression.readiness_competencies
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT ON progression.readiness_snapshots TO prepeet_app;
GRANT SELECT, INSERT ON progression.readiness_competencies TO prepeet_app;
