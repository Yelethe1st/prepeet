-- 0010: single-use action tokens.
--
-- Email verification, password recovery, magic link and one-time codes are
-- the same shape with different expiries: a secret we sent somewhere, that
-- proves control of that somewhere, exactly once, for a while. One table
-- rather than four keeps single-use and supersession in one place, and the
-- purpose column keeps a token minted for one flow out of another.
--
-- Implements part of IAM-02.

CREATE TABLE identity.action_tokens (
    id           uuid        PRIMARY KEY,
    user_id      uuid        NOT NULL REFERENCES identity.users (id),
    purpose      text        NOT NULL
                             CHECK (purpose IN ('email_verify', 'password_reset', 'magic_link', 'otp')),

    -- Only ever the hash. The plaintext goes into exactly one email and is
    -- otherwise unrecoverable, so this table leaking is inconvenient rather
    -- than an account takeover.
    token_hash   text        NOT NULL UNIQUE,

    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,

    -- Single use. Set in the same transaction as the effect the token grants,
    -- so replaying a link repeats nothing: the second presentation finds the
    -- marker and gets its own distinct outcome.
    used_at      timestamptz,

    -- Requesting a new token invalidates the previous one immediately, which
    -- the prototype promises: only the newest email works. Distinct from used
    -- because the person deserves a different explanation.
    superseded_at timestamptz,

    -- Wrong guesses against this token, for the one purpose short enough to
    -- guess: a six-digit code survives online guessing only because five
    -- wrong attempts kill the token.
    attempts     integer     NOT NULL DEFAULT 0
);

COMMENT ON TABLE identity.action_tokens IS
    'Single-use, expiring proofs of control of an email address. Hashes only. '
    'Issued and consumed by internal/identity; nothing else touches this table.';

-- The issue path supersedes every live token of the same purpose first.
CREATE INDEX action_tokens_live_idx
    ON identity.action_tokens (user_id, purpose)
    WHERE used_at IS NULL AND superseded_at IS NULL;

-- No row-level security, like every identity table: a person is not owned by
-- a tenant, and these rows exist precisely while there is no session to scope
-- a policy by. See ADR-0002.
GRANT SELECT, INSERT, UPDATE ON identity.action_tokens TO prepeet_app;
