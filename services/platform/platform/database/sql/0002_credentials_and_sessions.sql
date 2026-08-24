-- 0002: password credentials and the session family model.
--
-- ADR-0003 chose to build authentication rather than buy it, and two of its
-- decisions live in this schema.
--
-- Sessions are rows rather than self-describing tokens, because membership
-- revocation must take effect within seconds. A stateless token cannot be
-- withdrawn without building the lookup that stateless tokens exist to avoid.
--
-- Sessions belong to a family. A family is one login; each refresh appends a
-- new row to it and retires the previous one. Presenting a retired token means
-- either a stolen token or a client bug, and both revoke the whole family.
--
-- Neither table carries tenant_id. A person is not owned by a tenant: the same
-- candidate practises privately and may screen for several employers, and their
-- practice history is never reachable from any employer authority. Membership
-- in tenancy.memberships is what connects them, and that table is tenant
-- scoped. See ADR-0002.

CREATE TABLE identity.credentials (
    user_id       uuid        PRIMARY KEY REFERENCES identity.users (id) ON DELETE CASCADE,
    -- The full PHC encoding, carrying the argon2id parameters it was made
    -- under, so raising the cost upgrades old hashes on next login rather than
    -- locking anyone out.
    password_hash text        NOT NULL,
    updated_at    timestamptz NOT NULL DEFAULT now()
);

COMMENT ON COLUMN identity.credentials.password_hash IS
    'argon2id in PHC format. Never logged, never returned, never compared outside platform/password.';

CREATE TABLE identity.sessions (
    id                 uuid        PRIMARY KEY,
    user_id            uuid        NOT NULL REFERENCES identity.users (id) ON DELETE CASCADE,
    -- One login. Every rotation shares it, which is what makes a reuse
    -- revocable across the whole descent rather than one row at a time.
    family_id          uuid        NOT NULL,

    -- Hashes, never the tokens. A read of this table must not yield anything
    -- that can be presented as a credential.
    session_token_hash text        NOT NULL UNIQUE,
    refresh_token_hash text        NOT NULL UNIQUE,

    issued_at          timestamptz NOT NULL DEFAULT now(),
    expires_at         timestamptz NOT NULL,
    refresh_expires_at timestamptz NOT NULL,
    -- When the user last proved who they are, which is not when this row was
    -- issued: a rotation carries the original authentication forward. Step-up
    -- decisions in platform/authz read this.
    authenticated_at   timestamptz NOT NULL,

    -- Set when a rotation supersedes this row. A retired row is not revoked:
    -- presenting its refresh token is what triggers the revocation.
    retired_at         timestamptz,
    revoked_at         timestamptz,
    revoked_reason     text
);

-- The lookup on every authenticated request.
CREATE INDEX sessions_session_token_idx ON identity.sessions (session_token_hash)
    WHERE revoked_at IS NULL;
-- The refresh lookup has to find retired rows too, because finding one is
-- exactly how reuse is detected.
CREATE INDEX sessions_refresh_token_idx ON identity.sessions (refresh_token_hash);
-- Revoking a family touches every row in it.
CREATE INDEX sessions_family_idx ON identity.sessions (family_id);
CREATE INDEX sessions_user_idx ON identity.sessions (user_id);

COMMENT ON TABLE identity.sessions IS
    'One row per issued token pair. A family is one login; rotation appends and '
    'retires. Deliberately carries no IP address or user agent: they would be '
    'personal data collected for convenience rather than necessity.';

GRANT SELECT, INSERT, UPDATE, DELETE ON identity.credentials, identity.sessions TO prepeet_app;
GRANT SELECT ON identity.credentials, identity.sessions TO prepeet_readonly;
