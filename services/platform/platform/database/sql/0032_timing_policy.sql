-- 0032: the timing policy, versioned in the database.
--
-- SES-05. Reconnect grace and duration ceilings are policy, and policy
-- changes: a value compiled into a client survives in cached bundles for
-- weeks, but a versioned row can be read at start and stamped on the
-- session, so every session knows exactly which rules governed it. New
-- values are new versions - rows are never edited - and the stamp makes an
-- old session's behaviour reconstructable after the policy moves on, the
-- same property the rubric pin gives evaluation.
--
-- Implements part of SES-05.

CREATE TABLE interview.timing_policies (
    version                 integer     PRIMARY KEY CHECK (version >= 1),
    reconnect_grace_seconds integer     NOT NULL CHECK (reconnect_grace_seconds > 0),

    -- How far past the configured interview length a session may run
    -- before the platform ends it. A ceiling on overrun, not a target.
    max_overrun_seconds     integer     NOT NULL CHECK (max_overrun_seconds >= 0),

    note                    text        NOT NULL,
    created_at              timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE interview.timing_policies IS
    'Versioned timing rules. Sessions stamp the version in force at start; '
    'rows are never edited, so a stamped session stays reconstructable.';

CREATE OR REPLACE FUNCTION interview.refuse_timing_policy_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'interview.timing_policies rows are immutable: publish a new version';
END $$;

CREATE TRIGGER timing_policies_immutable
    BEFORE UPDATE OR DELETE ON interview.timing_policies
    FOR EACH ROW EXECUTE FUNCTION interview.refuse_timing_policy_change();

-- Policy is platform-wide, not per-person: readable by anyone the schema
-- admits, so no RLS. Writes come only from migrations.
GRANT SELECT ON interview.timing_policies TO prepeet_app;

INSERT INTO interview.timing_policies (version, reconnect_grace_seconds, max_overrun_seconds, note)
VALUES (1, 120, 300,
        'Initial values pending DEC-14: two minutes of reconnect grace '
        'mirrors the join window; five minutes of overrun bounds a closing '
        'answer without letting a session run open-ended.');

-- The stamp: which timing rules governed this session, set at start.
ALTER TABLE interview.sessions
    ADD COLUMN timing_policy_version integer REFERENCES interview.timing_policies (version);

COMMENT ON COLUMN interview.sessions.timing_policy_version IS
    'The timing policy in force when the session started. NULL until start.';
