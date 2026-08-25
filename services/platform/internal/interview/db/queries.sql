-- The session store's queries. sqlc generates the Go beside this file;
-- ADR-0010 records why no SQL lives in Go source.

-- name: InsertSession :exec
INSERT INTO interview.sessions (id, mode, candidate_id, tenant_id, blueprint_id)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(mode)::text, sqlc.arg(candidate_id)::uuid,
        nullif(sqlc.arg(tenant_id)::text, '')::uuid, sqlc.arg(blueprint_id)::text);

-- name: GetSession :one
SELECT id::text AS id, mode, candidate_id::text AS candidate_id,
       coalesce(tenant_id::text, '')::text AS tenant_id,
       blueprint_id, state, version,
       coalesce(bundle_ref, '')::text AS bundle_ref,
       coalesce(bundle_digest, '')::text AS bundle_digest,
       coalesce(bundle_revision, 0)::integer AS bundle_revision,
       coalesce(failure_code, '')::text AS failure_code,
       created_at, state_changed_at
FROM interview.sessions
WHERE id = sqlc.arg(id)::uuid;

-- name: TransitionSession :execrows
-- The guards are the aggregate's whole concurrency story. Zero rows means the
-- state or the version the caller read is no longer true, and the caller
-- re-reads to say which; the update never overwrites either silently.
UPDATE interview.sessions
SET state = sqlc.arg(to_state)::text,
    version = version + 1,
    state_changed_at = now(),
    bundle_ref = coalesce(nullif(sqlc.arg(bundle_ref)::text, ''), bundle_ref),
    bundle_digest = coalesce(nullif(sqlc.arg(bundle_digest)::text, ''), bundle_digest),
    bundle_revision = CASE WHEN sqlc.arg(bundle_revision)::integer > 0
                           THEN sqlc.arg(bundle_revision)::integer ELSE bundle_revision END,
    failure_code = nullif(sqlc.arg(failure_code)::text, '')
WHERE id = sqlc.arg(id)::uuid
  AND state = sqlc.arg(from_state)::text
  AND version = sqlc.arg(expected_version)::integer;

-- name: InsertAuditEvent :exec
-- Interview's own audit append, inside the transition's transaction. tenant_id
-- is NULL for practice transitions, where the untenanted-write policy binds
-- the row to the acting user.
INSERT INTO audit.events
    (id, tenant_id, actor_id, actor_type, action, subject_type, subject_id, outcome)
VALUES (sqlc.arg(id)::uuid, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(actor_id)::uuid, sqlc.arg(actor_type)::text, sqlc.arg(action)::text,
        'session', sqlc.arg(session_id)::text, sqlc.arg(outcome)::text);

-- name: InsertSessionBundle :exec
-- Written in the ready transition's transaction: a session marked ready
-- whose bundle failed to persist would pin a digest nothing can resolve.
INSERT INTO interview.session_bundles (session_id, digest, body)
VALUES (sqlc.arg(session_id)::uuid, sqlc.arg(digest)::text, sqlc.arg(body)::jsonb);

-- name: GetSessionBundle :one
-- What review, replay and audit reconstruct a session's configuration from.
SELECT session_id::text AS session_id, digest, body, created_at
FROM interview.session_bundles
WHERE session_id = sqlc.arg(session_id)::uuid;
