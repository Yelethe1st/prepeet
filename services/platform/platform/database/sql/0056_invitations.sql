-- 0056: screening invitations.
--
-- SCR-04. An invitation is the single-use, expiring link that admits one
-- candidate to one campaign. It sits downstream of everything 0043 froze: a
-- campaign must be open before it can issue one, because opening is the moment
-- its configuration and its jurisdiction determination were fixed, and an
-- invitation to a campaign that had not settled those would be a promise the
-- campaign could still break.
--
-- Three things about the token decide the shape of this table.
--
-- Only the hash is stored, never the token itself. The plaintext exists for
-- exactly one instant, inside the call that mints it and hands it to the
-- email, and is never written down. A database that leaks does not thereby
-- hand out working links, and there is nothing here for an attacker to read
-- back into a usable token. This is identity's action-token discipline
-- (0002) applied to recruiting, deliberately, rather than a second scheme.
--
-- The token is single-use by the same mechanism identity uses: consuming it
-- is an UPDATE guarded on the outcome still being null, so two requests
-- racing the same link produce one winner and one refusal rather than two
-- accepted interviews. Expiry and revocation ride the same guard.
--
-- Enumeration is answered by the token, not by the id: the lookup that a
-- candidate drives is by token_hash, which is the hash of 32 random bytes and
-- has nothing sequential to walk. The id is a UUIDv7 the recruiter's tenant
-- sees; it never reaches the candidate and never validates a link.
--
-- Implements SCR-04.

-- One invitation. Tenant-scoped like every recruiting row, with no
-- practice-owner policy because screening is the only thing that issues one.
CREATE TABLE recruiting.invitation (
    id            uuid        PRIMARY KEY,
    tenant_id     uuid        NOT NULL,

    -- The campaign this admits the candidate to. The invitation carries no
    -- configuration of its own: everything the interview runs against is the
    -- campaign's pinned set, so a link cannot smuggle in a different rubric or
    -- a different disclosure than the one the campaign opened with.
    campaign_id   uuid        NOT NULL REFERENCES recruiting.campaign (id),

    -- Who it was sent to. citext because an email address is the same address
    -- whatever case it was typed in, and a resend must find the live one.
    recipient     citext      NOT NULL,

    -- The hash of the token, never the token. sha256 as 64 hex characters,
    -- which is exactly what platform/token's HashOf produces and what
    -- identity's action tokens store; the CHECK refuses a raw token or a
    -- prefixed digest passed where a bare hash was meant, so a mismatch is a
    -- write that fails rather than a row that never matches. Unique: one row
    -- per token, which is what makes the consume-guard a single-row race with
    -- exactly one winner.
    token_hash    text        NOT NULL UNIQUE
                              CHECK (token_hash ~ '^[0-9a-f]{64}$'),

    -- The email this invitation was carried by, so a recruiter can see whether
    -- it was delivered, bounced or is still pending. A soft reference, not a
    -- foreign key: notification.emails belongs to another context and this
    -- context does not read it. cmd joins the two where contexts compose; the
    -- ownership gate would refuse a query here that named the other schema.
    email_id      uuid        NOT NULL,

    issued_by     uuid        NOT NULL,
    issued_at     timestamptz NOT NULL DEFAULT now(),

    -- When the link stops working on its own. Answered over days, so this is
    -- set in the caller rather than defaulted to a token's minutes.
    expires_at    timestamptz NOT NULL,

    -- How the invitation ended, if it has. Null is the only live state; every
    -- non-null value is terminal and set exactly once. Expiry is deliberately
    -- absent from this set: an expired invitation is one with a null outcome
    -- and an expires_at in the past, a fact time decides, not a column a job
    -- has to flip. accepted and declined are the candidate's, revoked is the
    -- recruiter's, superseded is a resend retiring the link it replaces.
    outcome       text        CHECK (outcome IN (
                                  'accepted', 'declined', 'revoked', 'superseded')),
    outcome_at    timestamptz,

    -- An outcome and its timestamp are one fact: neither exists without the
    -- other. A row with one and not the other would be a half-recorded ending.
    CHECK ((outcome IS NULL) = (outcome_at IS NULL))
);

COMMENT ON TABLE recruiting.invitation IS
    'One single-use, expiring link admitting one candidate to one campaign. '
    'Only the token hash is stored; the plaintext lives for one call and is '
    'never written. A null outcome is the only live state; expiry is time '
    'against expires_at, not a stored flag.';

COMMENT ON COLUMN recruiting.invitation.email_id IS
    'Soft reference to notification.emails, joined in cmd for delivery status. '
    'Not a foreign key: recruiting does not read the notification schema.';

-- The recruiter's list: this campaign's invitations, newest first.
CREATE INDEX invitation_by_campaign
    ON recruiting.invitation (tenant_id, campaign_id, issued_at DESC);

-- The live invitations for a recipient, which a resend supersedes. Partial on
-- a null outcome because that is the only set a resend touches, and because a
-- recipient may hold many spent invitations to the same campaign over time.
CREATE INDEX invitation_live_by_recipient
    ON recruiting.invitation (tenant_id, campaign_id, recipient)
    WHERE outcome IS NULL;

ALTER TABLE recruiting.invitation ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.invitation FORCE ROW LEVEL SECURITY;

-- Tenant-scoped exactly like the campaign it belongs to. The candidate who
-- accepts a link is not acting as the tenant and does not read this table;
-- acceptance (SCR-05) resolves the token in a path that carries no tenant and
-- reaches the row by hash, which is why the consume query is written to run
-- outside this policy rather than under it.
CREATE POLICY invitation_tenant ON recruiting.invitation
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE ON recruiting.invitation TO prepeet_app;
