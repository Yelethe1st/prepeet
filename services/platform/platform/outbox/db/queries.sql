-- The outbox's queries. sqlc generates the Go in this directory from this file;
-- ADR-0008 records why they moved here from hand-written pgx.

-- name: InsertEvent :exec
-- Written inside the caller's transaction, which is the whole point of an
-- outbox: the event and the state change it describes commit together or
-- neither does.
INSERT INTO integration.outbox
    (id, event_type, schema_version, tenant_id, occurred_at, producer,
     actor_type, actor_id, purpose, correlation_id, causation_id, payload)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(event_type)::text, sqlc.arg(schema_version)::text,
        nullif(sqlc.arg(tenant_id)::text, '')::uuid, sqlc.arg(occurred_at)::timestamptz,
        sqlc.arg(producer)::text, sqlc.arg(actor_type)::text, sqlc.arg(actor_id)::text,
        nullif(sqlc.arg(purpose)::text, ''), nullif(sqlc.arg(correlation_id)::text, ''),
        nullif(sqlc.arg(causation_id)::text, ''), sqlc.arg(payload)::jsonb);

-- name: Claim :many
-- FOR UPDATE SKIP LOCKED is what makes more than one dispatcher safe. Without
-- it two dispatchers reading the same rows would both deliver them, and a
-- tenant's ATS would see one candidate submitted twice. With it, the second
-- steps over the rows the first holds, so adding dispatchers adds throughput
-- rather than duplicates.
--
-- The rows stay claimed only for the life of the caller's transaction. A
-- dispatcher that dies mid-delivery releases them and they are redelivered,
-- which is the at-least-once guarantee showing its edges.
SELECT id::text AS id, event_type, schema_version,
       coalesce(tenant_id::text, '')::text AS tenant_id,
       occurred_at, producer, actor_type, actor_id,
       coalesce(purpose, '')::text AS purpose,
       coalesce(correlation_id, '')::text AS correlation_id,
       coalesce(causation_id, '')::text AS causation_id,
       payload, attempts
FROM integration.outbox
WHERE published_at IS NULL
  AND dead_at IS NULL
  AND next_attempt_at <= now()
ORDER BY next_attempt_at, id
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: HideClaimed :exec
-- Marked inside the same transaction that locked the rows, so releasing the
-- lock and recording the attempt happen together.
UPDATE integration.outbox
SET next_attempt_at = now() + sqlc.arg(visibility)::interval
WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: MarkDelivered :exec
UPDATE integration.outbox SET published_at = now() WHERE id = $1 AND published_at IS NULL;

-- name: LockAttempts :one
-- The attempt count is read here and the wait computed in Go, so the backoff
-- curve lives in exactly one place. Computing it in SQL as well would mean two
-- formulas that agree until somebody changes one.
SELECT attempts FROM integration.outbox WHERE id = $1 FOR UPDATE;

-- name: RecordFailure :exec
-- After MaxAttempts the caller passes a dead_at, because an event nobody can
-- deliver is an operational fact somebody needs to see rather than a row
-- retried silently forever.
UPDATE integration.outbox
SET attempts = sqlc.arg(attempts)::integer,
    last_error = sqlc.arg(last_error)::text,
    next_attempt_at = now() + make_interval(secs => sqlc.arg(backoff_seconds)::double precision),
    dead_at = sqlc.narg(dead_at)::timestamptz
WHERE id = sqlc.arg(id)::uuid;
