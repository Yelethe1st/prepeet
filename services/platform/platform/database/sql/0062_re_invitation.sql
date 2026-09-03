-- 0062: re-invitation, a named human's decision to let a candidate try again.
--
-- SCR-08 and ADR-0016. When an interview is interrupted, the platform never
-- re-invites on its own initiative: a human decides, per candidate, per
-- campaign, and records why. This is that decision. A re-invitation authorizes
-- exactly one further screening session; the interrupted session's evidence is
-- retained and never merged, so a reviewer sees each attempt on its own.
--
-- The reason and the decider are the point. An automatic retry policy would
-- quietly advantage whoever has the better network; a decision with a named
-- human and a recorded reason is the opposite, and both are NOT NULL with no
-- default so the schema refuses a re-invitation that cannot say who authorized
-- it or why.
--
-- Implements part of SCR-08.

CREATE TABLE recruiting.re_invitation (
    id            uuid        PRIMARY KEY,
    campaign_id   uuid        NOT NULL REFERENCES recruiting.campaign (id),
    tenant_id     uuid        NOT NULL,
    candidate_id  uuid        NOT NULL REFERENCES identity.users (id),

    -- Why this candidate may try again, from the human who decided it. A
    -- re-invitation without a reason is exactly the automatic retry this exists
    -- to refuse, so it is required with no default.
    reason        text        NOT NULL CHECK (length(trim(reason)) > 0),
    decided_by    uuid        NOT NULL REFERENCES identity.users (id),

    -- The session whose interruption prompted this, when there is one. A soft
    -- reference: interview.sessions is another context's, and the link is for
    -- the reviewer's audit rather than a foreign key this schema enforces.
    interrupted_session uuid,

    -- Set when a new session is started against this authorization, so one
    -- re-invitation admits exactly one further attempt: the start path claims
    -- it, and a claimed re-invitation authorizes nothing more.
    consumed_session uuid,

    created_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE recruiting.re_invitation IS
    'A named human''s decision, with a recorded reason, to let one candidate '
    'take one further screening session on one campaign. Never automatic; the '
    'interrupted session''s evidence is retained separately.';

CREATE INDEX re_invitation_unclaimed
    ON recruiting.re_invitation (campaign_id, candidate_id)
    WHERE consumed_session IS NULL;

ALTER TABLE recruiting.re_invitation ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.re_invitation FORCE ROW LEVEL SECURITY;

-- The tenant's, for the recruiter who authorizes and reads it. The candidate
-- does not read this table: they learn of a re-invitation by being told, which
-- is SCR-08's third criterion, not by reading the recruiter's reasoning.
CREATE POLICY re_invitation_tenant ON recruiting.re_invitation
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- The candidate's claim path acts untenanted as themselves and must update the
-- one re-invitation that authorizes their next attempt, so a second, narrow
-- policy admits exactly that: their own, and only to claim it.
CREATE POLICY re_invitation_candidate_claim ON recruiting.re_invitation
    USING (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT, UPDATE ON recruiting.re_invitation TO prepeet_app;
