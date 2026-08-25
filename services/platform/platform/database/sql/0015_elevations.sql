-- 0015: time-bound platform elevation.
--
-- Support access to tenant data is exceptional: reason-bound, ticket-linked,
-- time-limited, revocable, and recorded whether or not anything was read.
-- The grant is a row; the expiry is a timestamp compared at read time rather
-- than a job that might not run; and every authenticated request made while
-- a grant is active writes its own audit row from the session lookup, which
-- is the one choke point all reads pass.
--
-- Implements part of IAM-07.

CREATE TABLE identity.elevations (
    id         uuid        PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES identity.users (id),

    -- Why, and under which ticket. NOT NULL and non-empty by CHECK rather
    -- than by validation alone: an elevation whose reason arrived blank
    -- through some future path would be unauditable by construction.
    reason     text        NOT NULL CHECK (reason <> ''),
    ticket     text        NOT NULL CHECK (ticket <> ''),

    granted_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,

    revoked_at timestamptz,
    revoked_by uuid        REFERENCES identity.users (id),

    -- An expiry before the grant would be a row that was never active and
    -- still looks like one in a list.
    CHECK (expires_at > granted_at)
);

COMMENT ON TABLE identity.elevations IS
    'Time-bound platform elevations, per ADR authorization-model: reason and '
    'ticket required, expiry compared at read time, revocation immediate. '
    'Requests made under an active grant are audited from session lookup.';

-- The per-request check: one user's active grant, newest first.
CREATE INDEX elevations_active_idx
    ON identity.elevations (user_id, expires_at DESC)
    WHERE revoked_at IS NULL;

-- No row-level security, like every identity table: elevation is platform
-- authority, which is not tenant-scoped, and the visibility criterion wants
-- the whole active list readable by the operator's team.
GRANT SELECT, INSERT, UPDATE ON identity.elevations TO prepeet_app;
