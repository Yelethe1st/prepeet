-- 0039: configured OAuth, in two tables.
--
-- IAM-08. DEC-02 required OAuth for the first release and ADR-0003 put it in
-- Go beside the password flow; no ticket ever claimed the work, so this is the
-- storage it needed all along.
--
-- Two tables because there are two lifetimes. A state lives for minutes and
-- dies on first use; a link between a provider account and a person lives as
-- long as the account does.
--
-- Implements part of IAM-08.

-- The in-flight authorisation: anti-forgery state and the PKCE verifier.
--
-- The row exists before anybody is known, which is why there is no user_id
-- here: it is minted when the person clicks "continue with Google" and
-- consumed when the provider sends them back, and in between there is no
-- session and no identity to scope it by.
CREATE TABLE identity.oauth_states (
    id            uuid        PRIMARY KEY,
    provider      text        NOT NULL,

    -- Only the hash, as with action_tokens: the state goes to the browser and
    -- comes back in a query string, so this table leaking must not let anybody
    -- complete somebody else's authorisation.
    state_hash    text        NOT NULL UNIQUE,

    -- The PKCE verifier, in plaintext, because it has to be replayed to the
    -- provider's token endpoint and a hash cannot be. It is meaningless
    -- without the matching authorisation code, which is single-use at the
    -- provider and never stored here.
    code_verifier text        NOT NULL,

    -- Where to send them afterwards. Validated when it is written, not when it
    -- is read: an open redirect stored is an open redirect.
    redirect_to   text        NOT NULL DEFAULT '',

    created_at    timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL,

    -- Single use. Set in the same transaction as the exchange it authorises,
    -- so a replayed callback finds the marker and is refused rather than
    -- completing a second time.
    used_at       timestamptz
);

COMMENT ON TABLE identity.oauth_states IS
    'One in-flight OAuth authorisation: anti-forgery state (hashed) and its '
    'PKCE verifier. Single-use and short-lived. No user: the row predates '
    'knowing who the person is.';

CREATE INDEX oauth_states_expiry_idx ON identity.oauth_states (expires_at)
    WHERE used_at IS NULL;

COMMENT ON INDEX identity.oauth_states_expiry_idx IS
    'Sweeping the dead ones. Unused states accumulate at the rate people '
    'start a sign-in and abandon it, which is not a small rate.';

-- The durable link between a provider account and a person.
CREATE TABLE identity.oauth_identities (
    id         uuid        PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES identity.users (id) ON DELETE CASCADE,
    provider   text        NOT NULL,

    -- The provider's own stable identifier for the account. Not the email:
    -- an email can be reassigned by its domain owner and changed by its
    -- holder, and both would silently move a link to a different person.
    subject    text        NOT NULL,

    -- What the provider asserted at link time, for support and for display.
    -- Never used to find a user: see the note on the unique constraint.
    email      text        NOT NULL,

    linked_at  timestamptz NOT NULL DEFAULT now(),
    last_seen  timestamptz NOT NULL DEFAULT now(),

    -- One person per provider account. Without this a second link would let
    -- one Google account sign in as two people.
    UNIQUE (provider, subject)
);

COMMENT ON TABLE identity.oauth_identities IS
    'Which provider accounts sign in as which person. Keyed by the provider '
    'subject, never by email: an email can be reassigned and would move the '
    'link to somebody else.';

-- One provider account per person per provider, so "sign in with Google"
-- resolves to one identity rather than an arbitrary one of several.
CREATE UNIQUE INDEX oauth_identities_one_per_provider
    ON identity.oauth_identities (user_id, provider);

-- No row-level security, like every identity table: a person is not owned by
-- a tenant, and these rows exist precisely while there is no session to scope
-- a policy by. See ADR-0002.
GRANT SELECT, INSERT, UPDATE, DELETE ON identity.oauth_states TO prepeet_app;
GRANT SELECT, INSERT, UPDATE ON identity.oauth_identities TO prepeet_app;
