-- Identity's queries. sqlc generates the Go in this directory from this file;
-- ADR-0008 records why they moved here from hand-written pgx.
--
-- Nothing here is tenant scoped, because identity is not: a person is not owned
-- by a tenant. Membership carries the tenant and its policy. See ADR-0002.
--
-- Several statements below are deliberately reached only inside a transaction
-- that has already set the acting user, because row-level security, not a WHERE
-- clause, is what decides which rows they can see. The repository is where that
-- happens; sqlc's Queries takes the transaction and cannot set it itself.

-- name: FindCredentialsByEmail :one
-- The login read. Returns nothing for a suspended or deleted account, so those
-- fail exactly as a wrong password does rather than in a distinguishable way.
SELECT u.id::text AS user_id, c.password_hash
FROM identity.users u
JOIN identity.credentials c ON c.user_id = u.id
WHERE u.email = $1 AND u.status = 'active';

-- name: FindUserByID :one
-- The fields GET /me reports, and only those. A SELECT * here would put status
-- and version into a struct one refactor away from being serialised to a
-- browser, and a person's suspension status is not theirs to read from an
-- endpoint about themselves.
SELECT id::text AS id, coalesce(email::text, '')::text AS email, email_verified
FROM identity.users
WHERE id = $1 AND status <> 'deleted';

-- name: MembershipExists :one
-- The one membership check in the system. Everything downstream reads the
-- selection from the session rather than re-deriving it, so this is the only
-- place it can be got wrong.
SELECT EXISTS (
    SELECT 1 FROM tenancy.memberships
    WHERE user_id = $1 AND tenant_id = $2 AND status = 'active'
) AS permitted;

-- name: SetSessionActiveTenant :exec
-- An empty string clears the selection rather than failing, which is what
-- leaving a workspace means.
UPDATE identity.sessions SET active_tenant_id = nullif(sqlc.arg(tenant_id)::text, '')::uuid
WHERE id = sqlc.arg(session_id)::uuid;

-- name: InsertAuditEvent :exec
-- tenant_id is left NULL because tenant selection happens before a tenant is
-- chosen. The audit policy makes such a row writable and readable by its own
-- actor, which is the only meaningful scope it has.
INSERT INTO audit.events
    (id, tenant_id, actor_id, actor_type, action, subject_type, subject_id, outcome, request_id)
VALUES (sqlc.arg(id)::uuid, NULL, sqlc.arg(actor_id)::uuid, 'user', sqlc.arg(action)::text,
        nullif(sqlc.arg(subject_type)::text, ''), nullif(sqlc.arg(subject_id)::text, ''),
        sqlc.arg(outcome)::text, nullif(sqlc.arg(request_id)::text, ''));

-- name: FindRole :one
-- A revoked membership is not found, so revoking one takes effect on the next
-- request rather than whenever a session happens to end.
SELECT role FROM tenancy.memberships
WHERE user_id = $1 AND tenant_id = $2 AND status = 'active';

-- name: ListMembershipsByUser :many
-- The WHERE on user_id is not the security boundary. It is a filter for the
-- planner; the policy from migration 0007 would refuse the rows even if it were
-- removed, which is the property that makes forgetting it survivable.
SELECT m.tenant_id::text AS tenant_id, t.name AS tenant_name, m.status, m.role
FROM tenancy.memberships m
JOIN tenancy.tenants t ON t.id = m.tenant_id
WHERE m.user_id = $1 AND m.status <> 'revoked'
ORDER BY t.name;

-- name: InsertUser :exec
INSERT INTO identity.users (id, email) VALUES ($1, $2);

-- name: InsertCredentials :exec
INSERT INTO identity.credentials (user_id, password_hash) VALUES ($1, $2);

-- name: InsertTenant :exec
-- Reachable only from a transaction already scoped to the identifier being
-- inserted: the policy on tenancy.tenants is written against the primary key,
-- since a tenants table carrying a tenant_id would be circular. Nothing can
-- create the first tenant from outside.
INSERT INTO tenancy.tenants (id, name, slug, region) VALUES ($1, $2, $3, $4);

-- name: InsertOwningMembership :exec
-- What makes a workspace administrable. A tenant without one is a row nobody
-- can reach, including support.
INSERT INTO tenancy.memberships (id, tenant_id, user_id, status, role)
VALUES ($1, $2, $3, 'active', 'owner');

-- name: UpdatePasswordHash :exec
-- Used when a successful login verified against outdated argon2 parameters.
UPDATE identity.credentials SET password_hash = $2, updated_at = now() WHERE user_id = $1;

-- name: InsertSession :exec
INSERT INTO identity.sessions
    (id, user_id, family_id, session_token_hash, refresh_token_hash,
     issued_at, expires_at, refresh_expires_at, authenticated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: FindSessionByToken :one
SELECT id::text AS id, user_id::text AS user_id, family_id::text AS family_id,
       session_token_hash, refresh_token_hash,
       issued_at, expires_at, refresh_expires_at, authenticated_at,
       retired_at, revoked_at, coalesce(active_tenant_id::text, '')::text AS active_tenant_id
FROM identity.sessions
WHERE session_token_hash = $1;

-- name: FindSessionByRefresh :one
-- Finds retired rows too, deliberately. Finding one is exactly how token reuse
-- is detected, so excluding them would remove the signal.
SELECT id::text AS id, user_id::text AS user_id, family_id::text AS family_id,
       session_token_hash, refresh_token_hash,
       issued_at, expires_at, refresh_expires_at, authenticated_at,
       retired_at, revoked_at, coalesce(active_tenant_id::text, '')::text AS active_tenant_id
FROM identity.sessions
WHERE refresh_token_hash = $1;

-- name: RetireSession :exec
-- Retired is not revoked. A retired row stays valid to look up precisely so
-- that presenting its refresh token can be recognised as reuse.
UPDATE identity.sessions SET retired_at = $2 WHERE id = $1 AND retired_at IS NULL;

-- name: RevokeFamily :exec
-- Whole family rather than one row, because a reused refresh token means the
-- legitimate client cannot be told from the attacker, and revoking only the row
-- that was reused would leave whichever of them holds the current pair.
UPDATE identity.sessions
SET revoked_at = sqlc.arg(revoked_at)::timestamptz,
    revoked_reason = sqlc.arg(reason)::text
WHERE family_id = sqlc.arg(family_id)::uuid AND revoked_at IS NULL;
