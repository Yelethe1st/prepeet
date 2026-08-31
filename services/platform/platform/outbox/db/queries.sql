-- The outbox's queries. sqlc generates the Go in this directory from this file;
-- ADR-0008 records why they moved here from hand-written pgx.

-- name: InsertEvent :exec
-- Written inside the caller's transaction, which is the whole point of an
-- outbox: the event and the state change it describes commit together or
-- neither does.
INSERT INTO integration.outbox
    (id, event_type, schema_version, tenant_id, occurred_at, producer,
     actor_type, actor_id, purpose, correlation_id, causation_id, payload,
     trace_parent, trace_state)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(event_type)::text, sqlc.arg(schema_version)::text,
        nullif(sqlc.arg(tenant_id)::text, '')::uuid, sqlc.arg(occurred_at)::timestamptz,
        sqlc.arg(producer)::text, sqlc.arg(actor_type)::text, sqlc.arg(actor_id)::text,
        nullif(sqlc.arg(purpose)::text, ''), nullif(sqlc.arg(correlation_id)::text, ''),
        nullif(sqlc.arg(causation_id)::text, ''), sqlc.arg(payload)::jsonb,
        -- Null rather than empty: published outside a trace is a real state,
        -- and a zero-valued traceparent would point at a trace nobody can find.
        nullif(sqlc.arg(trace_parent)::text, ''), nullif(sqlc.arg(trace_state)::text, ''));

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
       coalesce(trace_parent, '')::text AS trace_parent,
       coalesce(trace_state, '')::text AS trace_state,
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

-- ── OPS-03: what an operator sees, and the two transitions they may make.

-- name: PendingBacklog :one
-- Depth and age together, because depth alone cannot be alerted on: a large
-- backlog draining in a second is throughput, and three items that have waited
-- ten minutes is a candidate still looking at a spinner.
--
-- The age is measured from occurred_at rather than from the last attempt. That
-- is the wait the candidate experiences, and it is the only version of the
-- number that can be compared against a completion-to-review objective.
--
-- The predicates match outbox_backlog_age_idx from migration 0047 exactly, so
-- this stays an index scan as delivered rows accumulate.
--
-- Clamped at zero because occurred_at is the publisher's clock and now() is the
-- database's. A publisher a few milliseconds ahead would otherwise produce a
-- negative age, and a negative age is not a harmless oddity here: it is smaller
-- than every threshold, so the one number the alert depends on would quietly
-- read healthy.
SELECT count(*)::bigint AS depth,
       greatest(coalesce(extract(epoch FROM now() - min(occurred_at)), 0), 0)::double precision AS oldest_seconds
FROM integration.outbox
WHERE published_at IS NULL AND dead_at IS NULL AND discarded_at IS NULL;

-- name: FailedBacklog :one
-- Work that has exhausted its attempts and has not been discarded: the queue of
-- decisions waiting for a person.
SELECT count(*)::bigint AS depth
FROM integration.outbox
WHERE published_at IS NULL AND dead_at IS NOT NULL AND discarded_at IS NULL;

-- name: ListFailed :many
-- Newest failure first, because an operator working an incident is looking for
-- what just broke rather than for the oldest thing in the table.
SELECT id::text AS id, event_type,
       coalesce(tenant_id::text, '')::text AS tenant_id,
       occurred_at, attempts,
       coalesce(last_error, '')::text AS last_error,
       dead_at
FROM integration.outbox
WHERE published_at IS NULL AND dead_at IS NOT NULL AND discarded_at IS NULL
ORDER BY dead_at DESC, id
LIMIT $1;

-- name: RecoverFailed :one
-- The retry transition: dead work goes back to pending with a fresh budget.
--
-- RETURNING is the whole point. The WHERE clause is the guard, since only work
-- that is still failed, still undelivered and not discarded may be revived, and
-- the returned row is how the caller learns the guard held. An UPDATE that
-- matched nothing returns no row, so a second operator retrying the same item
-- cannot revive it twice, and no caller can mistake the no-op for success.
--
-- attempts resets because an operator retrying means "try this properly again",
-- not "make one more attempt before dying". last_error is deliberately left
-- alone: until it fails again, why it needed a person is still the useful fact.
UPDATE integration.outbox
SET dead_at = NULL,
    attempts = 0,
    next_attempt_at = now()
WHERE id = sqlc.arg(id)::uuid
  AND published_at IS NULL
  AND dead_at IS NOT NULL
  AND discarded_at IS NULL
RETURNING id::text AS id;

-- name: DiscardFailed :one
-- The other transition: this event must never be delivered.
--
-- A state change rather than a DELETE, for the reason migration 0047 states at
-- length: a delete that matches nothing is silent, and the row is the only
-- remaining answer to what happened to that notification.
UPDATE integration.outbox
SET discarded_at = now(),
    discard_reason = sqlc.arg(reason)::text
WHERE id = sqlc.arg(id)::uuid
  AND published_at IS NULL
  AND dead_at IS NOT NULL
  AND discarded_at IS NULL
RETURNING id::text AS id;
