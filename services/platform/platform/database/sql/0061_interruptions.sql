-- 0061: interruptions, recorded as a fact rather than resolved silently.
--
-- SES-06 and SCR-08. A connection or device failure in an interview is a thing
-- that happened, with a cause and a duration, not an automatic retry that
-- quietly advantages whoever has the better network. This records it: what
-- interrupted the session, when, and for how long. The resulting coverage is
-- the evaluation's, tied to the same session, because coverage is what
-- evaluation measured of what the interruption left; recording a number here
-- too would be a second place for it to disagree with the evidence.
--
-- Append-only. An interruption is a moment that occurred; a later one is a new
-- row. Both a practice and a screening session can be interrupted, so the row
-- carries the same owner columns the session does and the same policies, so
-- whoever may see the session may see what interrupted it and nobody else can.
--
-- Implements part of SES-06 and SCR-08.

CREATE TABLE interview.interruption (
    id            uuid        PRIMARY KEY,
    session_id    uuid        NOT NULL REFERENCES interview.sessions (id) ON DELETE CASCADE,

    -- The owner columns, mirrored from the session so this table's policies can
    -- be the session's policies: a screening interruption belongs to a tenant,
    -- a practice one to the candidate alone.
    candidate_id  uuid        NOT NULL REFERENCES identity.users (id),
    tenant_id     uuid        REFERENCES tenancy.tenants (id),

    -- Why the interview stopped. A closed vocabulary, because the difference
    -- between a candidate's device failing and their leaving is exactly what a
    -- human deciding on re-invitation needs, and a free-text cause is where
    -- that distinction gets lost.
    cause         text        NOT NULL CHECK (cause IN (
                                  'connection_lost', 'device_failure', 'candidate_left', 'grace_expired')),

    -- When the interruption began and how long the interview was down before it
    -- was recorded, so "duration" is answerable without joining events.
    occurred_at       timestamptz NOT NULL DEFAULT now(),
    duration_seconds  integer     NOT NULL CHECK (duration_seconds >= 0),

    -- The epoch that dropped, so an interruption is placed in the same attempt
    -- timeline every control event names.
    connection_epoch  integer     NOT NULL CHECK (connection_epoch >= 0)
);

COMMENT ON TABLE interview.interruption IS
    'A connection or device failure in an interview, with its cause and '
    'duration. Append-only; resulting coverage is the session''s evaluation, '
    'not restated here. Read by whoever may read the session.';

CREATE INDEX interruption_by_session ON interview.interruption (session_id, occurred_at);

ALTER TABLE interview.interruption ENABLE ROW LEVEL SECURITY;
ALTER TABLE interview.interruption FORCE ROW LEVEL SECURITY;

-- The session's own policies, applied to what interrupted it: the tenant sees a
-- screening interruption, the candidate sees their own in an untenanted
-- transaction, exactly as they do the session.
CREATE POLICY interruption_tenant ON interview.interruption
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY interruption_candidate ON interview.interruption
    USING (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT ON interview.interruption TO prepeet_app;
