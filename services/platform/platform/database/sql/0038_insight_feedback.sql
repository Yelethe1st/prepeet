-- 0038: whether the coaching described the person it was about.
--
-- ART-09. QUA-03 calibrates against human benchmarks and QUA-06 monitors
-- production, and neither had an input from the candidate reading the
-- output, who is the only one who knows whether "your opening establishes a
-- clear, defensible position" is true of their own answer.
--
-- Three things are structural here rather than checked in code.
--
-- Practice only, by CHECK and by having no tenant policy at all. A screening
-- candidate rating their own assessment would be a channel for pressure, and
-- the way to make that impossible is to leave out the policy that would let a
-- tenant row exist: there is no tenant_id column to scope by, so no tenant
-- read and no tenant write can ever match.
--
-- Once, and changeable. The unique constraint is the "once", and UPDATE is
-- granted because somebody who presses the wrong one must be able to correct
-- it. This is the one judgment in the schema that is deliberately not
-- immutable: it is a report about the coaching, not a record of what happened.
--
-- Attributable to what produced it. The digest and the policy version are
-- carried on the row, so a drop in helpfulness is traceable to an artifact
-- version rather than to a date, which is the difference between a signal
-- QUA-06 can act on and a graph nobody can explain.
--
-- What is deliberately absent is the aggregate read. Both database roles are
-- NOBYPASSRLS, and the only policy here scopes to one candidate, so a query
-- counting rejections across candidates matches zero rows and returns an
-- empty result rather than an error. A rates query was written and removed
-- for exactly that reason: a report that silently answers "none" is worse
-- than no report, because somebody will believe it. QUA-06's read needs its
-- own path - a metric emitted per verdict, or a reporting role with a policy
-- written for it - and that belongs with QUA-06 rather than being faked here.
--
-- Implements ART-09.

CREATE TABLE evaluation.insight_feedback (
    id                  uuid        PRIMARY KEY,
    session_id          uuid        NOT NULL,
    candidate_id        uuid        NOT NULL,

    -- Only practice. There is deliberately no 'screening' branch: see above.
    mode                text        NOT NULL CHECK (mode = 'practice'),

    insight_kind        text        NOT NULL CHECK (insight_kind IN
                            ('strength', 'priority', 'drill')),
    -- Which strength, priority or drill: the dimension or drill key it was
    -- generated for. Stable across a re-read of the same analysis.
    insight_key         text        NOT NULL,
    dimension           text,

    helpful             boolean     NOT NULL,

    -- What produced the insight, pinned at the moment it was shown.
    artifact_digest     text        NOT NULL,
    policy_version      text        NOT NULL,

    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),

    UNIQUE (session_id, candidate_id, insight_kind, insight_key)
);

COMMENT ON TABLE evaluation.insight_feedback IS
    'Whether a generated strength, priority or drill described the candidate. '
    'Practice only, one per insight per candidate, changeable. Feeds QUA-06; '
    'changes nothing the candidate is shown.';

CREATE INDEX insight_feedback_by_artifact
    ON evaluation.insight_feedback (artifact_digest, insight_kind, helpful);

COMMENT ON INDEX evaluation.insight_feedback_by_artifact IS
    'The rejection rate per artifact version is what makes the table worth '
    'having. The index is here for that read; the read itself has nowhere to '
    'run from yet - see the note in the table comment.';

ALTER TABLE evaluation.insight_feedback ENABLE ROW LEVEL SECURITY;
ALTER TABLE evaluation.insight_feedback FORCE ROW LEVEL SECURITY;

-- The only policy. A tenant scope is absent on purpose: with no tenant_id to
-- match, a screening context cannot read or write a row here at all.
CREATE POLICY insight_feedback_practice_owner ON evaluation.insight_feedback
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT, UPDATE ON evaluation.insight_feedback TO prepeet_app;
