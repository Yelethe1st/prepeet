-- 0027: connection attempts, epochs, and the control event log.
--
-- RTC-02, to realtime-protocol.md. A session has one authoritative
-- conversation timeline across many connection attempts: each start or
-- resume issues a monotonically increasing connection epoch, events order
-- by sequence within their epoch, event ids deduplicate retries, and the
-- accepted cursor - the highest contiguous sequence - is persisted so
-- recovery never relies on browser memory.
--
-- The event log is append-only like the billing ledger and for the same
-- reason: corrections supersede earlier segments as new events rather than
-- edits, so what the candidate said and heard is re-derivable forever.
--
-- Both tables carry the session's own dual policy shape: tenant rows to
-- tenant scope, practice rows to the owner with the tenant-absence clause,
-- denormalising mode/candidate/tenant for exactly that purpose.

CREATE TABLE interview.attempts (
    id               uuid        PRIMARY KEY,
    session_id       uuid        NOT NULL REFERENCES interview.sessions (id),
    mode             text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    candidate_id     uuid        NOT NULL,
    tenant_id        uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),
    connection_epoch integer     NOT NULL CHECK (connection_epoch >= 1),
    started_at       timestamptz NOT NULL DEFAULT now(),
    superseded_at    timestamptz,

    UNIQUE (session_id, connection_epoch)
);

COMMENT ON TABLE interview.attempts IS
    'One connection attempt: an epoch in the session''s single timeline. A '
    'new attempt supersedes the last, and events from superseded epochs are '
    'refused.';

ALTER TABLE interview.attempts ENABLE ROW LEVEL SECURITY;
ALTER TABLE interview.attempts FORCE ROW LEVEL SECURITY;

CREATE POLICY attempts_tenant ON interview.attempts
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY attempts_practice_owner ON interview.attempts
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT, UPDATE ON interview.attempts TO prepeet_app;

CREATE TABLE interview.control_events (
    event_id         uuid        PRIMARY KEY,
    session_id       uuid        NOT NULL REFERENCES interview.sessions (id),
    mode             text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    candidate_id     uuid        NOT NULL,
    tenant_id        uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),
    connection_epoch integer     NOT NULL CHECK (connection_epoch >= 1),
    sequence         integer     NOT NULL CHECK (sequence >= 1),
    event_type       text        NOT NULL,
    payload          jsonb       NOT NULL DEFAULT '{}'::jsonb,
    occurred_at      timestamptz NOT NULL,
    received_at      timestamptz NOT NULL DEFAULT now(),

    -- One event per slot: a different event claiming an occupied sequence
    -- is corruption, refused rather than resolved.
    UNIQUE (session_id, connection_epoch, sequence)
);

COMMENT ON TABLE interview.control_events IS
    'The durable control event log: what the candidate said and heard, in '
    'order. event_id deduplicates retries; the sequence slot is unique.';

CREATE INDEX control_events_replay_idx
    ON interview.control_events (session_id, connection_epoch, sequence);

CREATE OR REPLACE FUNCTION interview.refuse_event_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'interview.control_events is append-only: corrections supersede, they never edit';
END $$;

CREATE TRIGGER control_events_append_only
    BEFORE UPDATE OR DELETE ON interview.control_events
    FOR EACH ROW EXECUTE FUNCTION interview.refuse_event_change();

ALTER TABLE interview.control_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE interview.control_events FORCE ROW LEVEL SECURITY;

CREATE POLICY events_tenant ON interview.control_events
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY events_practice_owner ON interview.control_events
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT ON interview.control_events TO prepeet_app;

-- The session's current epoch and accepted cursor. Zero means no attempt
-- yet; the cursor resets to zero when a new epoch begins.
ALTER TABLE interview.sessions
    ADD COLUMN connection_epoch  integer NOT NULL DEFAULT 0 CHECK (connection_epoch >= 0),
    ADD COLUMN accepted_sequence integer NOT NULL DEFAULT 0 CHECK (accepted_sequence >= 0);

COMMENT ON COLUMN interview.sessions.connection_epoch IS
    'The current attempt''s epoch; events from lower epochs are refused.';
COMMENT ON COLUMN interview.sessions.accepted_sequence IS
    'Highest contiguous accepted sequence in the current epoch: the cursor '
    'recovery proves itself against, persisted so it never lives only in '
    'browser memory.';
