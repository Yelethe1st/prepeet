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

-- name: SupersedeActionTokens :exec
-- Requesting a new token invalidates every live one of the same purpose, so
-- only the newest email works. Run in the same transaction as the insert that
-- replaces them: a supersede that commits without its successor would leave
-- the person with no working token at all.
UPDATE identity.action_tokens
SET superseded_at = now()
WHERE user_id = sqlc.arg(user_id)::uuid
  AND purpose = sqlc.arg(purpose)::text
  AND used_at IS NULL
  AND superseded_at IS NULL;

-- name: InsertActionToken :exec
INSERT INTO identity.action_tokens (id, user_id, purpose, token_hash, expires_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(user_id)::uuid, sqlc.arg(purpose)::text,
        sqlc.arg(token_hash)::text, sqlc.arg(expires_at)::timestamptz);

-- name: FindActionTokenByHash :one
-- Reads the token whatever its state, because the states are the outcomes:
-- expired, used and superseded each earn their own explanation, and a query
-- that filtered them out would collapse all three into "invalid".
SELECT id::text AS id, user_id::text AS user_id, purpose,
       expires_at, used_at, superseded_at, attempts
FROM identity.action_tokens
WHERE token_hash = sqlc.arg(token_hash)::text;

-- name: FindLiveOTP :one
-- A code is looked up by who it was issued to rather than by hash, because
-- six digits are not unique enough to be an address. Newest first: after a
-- resend the person may still have both emails open.
SELECT id::text AS id, token_hash, expires_at, attempts
FROM identity.action_tokens
WHERE user_id = sqlc.arg(user_id)::uuid
  AND purpose = 'otp'
  AND used_at IS NULL
  AND superseded_at IS NULL
ORDER BY created_at DESC
LIMIT 1;

-- name: MarkActionTokenUsed :execrows
-- The guard columns repeat the liveness check so that two concurrent
-- presentations of one token race on the row update rather than both
-- succeeding: the loser sees zero rows and reports the token used.
UPDATE identity.action_tokens
SET used_at = now()
WHERE id = sqlc.arg(id)::uuid
  AND used_at IS NULL
  AND superseded_at IS NULL;

-- name: RecordTokenAttempt :one
-- Counts a wrong guess and reports the total, so the caller can kill the
-- token at the cap. Only the OTP path calls this: a 32-byte token is not
-- guessable, a six-digit code is.
UPDATE identity.action_tokens
SET attempts = attempts + 1
WHERE id = sqlc.arg(id)::uuid
RETURNING attempts;

-- name: MarkEmailVerified :exec
UPDATE identity.users SET email_verified = true, updated_at = now()
WHERE id = sqlc.arg(user_id)::uuid;

-- name: RevokeAllSessions :exec
-- Every family, not one: after a password reset the old sessions may be the
-- attacker's, and after a recovery the person expects to be the only one
-- signed in.
UPDATE identity.sessions
SET revoked_at = sqlc.arg(revoked_at)::timestamptz,
    revoked_reason = sqlc.arg(reason)::text
WHERE user_id = sqlc.arg(user_id)::uuid AND revoked_at IS NULL;

-- name: InsertElevation :exec
INSERT INTO identity.elevations (id, user_id, reason, ticket, expires_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(user_id)::uuid, sqlc.arg(reason)::text,
        sqlc.arg(ticket)::text, sqlc.arg(expires_at)::timestamptz);

-- name: ActiveElevationByUser :one
-- The newest grant still alive. Expiry is compared here, at read time, so an
-- elevation dies at its timestamp whether or not anything else runs.
SELECT id::text AS id, reason, ticket, granted_at, expires_at
FROM identity.elevations
WHERE user_id = sqlc.arg(user_id)::uuid
  AND revoked_at IS NULL
  AND expires_at > now()
ORDER BY expires_at DESC
LIMIT 1;

-- name: RevokeElevation :execrows
-- Guarded on liveness: revoking an already-dead grant reports zero rows and
-- the caller says so, rather than stamping a second ending on it.
UPDATE identity.elevations
SET revoked_at = now(), revoked_by = sqlc.arg(revoked_by)::uuid
WHERE id = sqlc.arg(id)::uuid AND revoked_at IS NULL AND expires_at > now();

-- name: ListActiveElevations :many
-- The visibility criterion: who is elevated right now, for the operator and
-- their team. Joined to users so the list names people, not identifiers.
SELECT e.id::text AS id, e.user_id::text AS user_id,
       coalesce(u.email::text, '')::text AS email,
       e.reason, e.ticket, e.granted_at, e.expires_at
FROM identity.elevations e
JOIN identity.users u ON u.id = e.user_id
WHERE e.revoked_at IS NULL AND e.expires_at > now()
ORDER BY e.expires_at;

-- name: InsertElevatedRequestAudit :exec
-- One row per authenticated request made under an active grant: the record
-- that access happened, whether or not anything was read, which is the
-- ticket's own wording. Written from session lookup, the choke point every
-- request passes exactly once.
INSERT INTO audit.events
    (id, tenant_id, actor_id, actor_type, action, subject_type, subject_id, outcome, detail, request_id)
VALUES (sqlc.arg(id)::uuid, NULL, sqlc.arg(actor_id)::uuid, 'user',
        'identity.elevated_request', 'elevation', sqlc.arg(grant_id)::text, 'allowed',
        sqlc.arg(detail)::jsonb, nullif(sqlc.arg(request_id)::text, ''));

-- name: InsertElevationAudit :exec
-- Grants and revocations, with reason and ticket in the detail so the trail
-- answers "why" without a join.
INSERT INTO audit.events
    (id, tenant_id, actor_id, actor_type, action, subject_type, subject_id, outcome, detail)
VALUES (sqlc.arg(id)::uuid, NULL, sqlc.arg(actor_id)::uuid, 'user',
        sqlc.arg(action)::text, 'elevation', sqlc.arg(grant_id)::text,
        sqlc.arg(outcome)::text, sqlc.arg(detail)::jsonb);
