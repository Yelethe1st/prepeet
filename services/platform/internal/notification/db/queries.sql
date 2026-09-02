-- The email queue's queries. sqlc generates the Go beside this file; ADR-0010
-- records why no SQL lives in Go source.

-- name: Enqueue :exec
-- Runs inside the caller's transaction, which is the guarantee the queue
-- exists for: the email and the state change that wants it sent commit
-- together or neither does.
INSERT INTO notification.emails (id, recipient, template, template_version, subject, body)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(recipient)::citext, sqlc.arg(template)::text,
        sqlc.arg(template_version)::text, sqlc.arg(subject)::text, sqlc.arg(body)::text);

-- name: Claim :many
-- FOR UPDATE SKIP LOCKED for the outbox's reason: two senders must not both
-- deliver the same email, and a second sender steps over what the first holds
-- rather than waiting behind it.
SELECT id::text AS id, recipient::text AS recipient,
       coalesce(subject, '')::text AS subject, coalesce(body, '')::text AS body,
       attempts
FROM notification.emails
WHERE sent_at IS NULL
  AND dead_at IS NULL
  AND next_attempt_at <= now()
ORDER BY next_attempt_at, id
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: HideClaimed :exec
-- Marked inside the same transaction that locked the rows, so a sender that
-- dies mid-send releases its claim by the visibility window at the latest.
UPDATE notification.emails
SET next_attempt_at = now() + sqlc.arg(visibility)::interval
WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: MarkSent :exec
-- The body and subject are erased in the same statement that records the
-- send. A verification link is a secret, and a sent secret has no reason to
-- stay readable at rest.
UPDATE notification.emails
SET sent_at = now(), subject = NULL, body = NULL, last_error = NULL
WHERE id = sqlc.arg(id)::uuid AND sent_at IS NULL;

-- name: LockAttempts :one
-- Read here, computed in Go, for the outbox's reason: one backoff formula.
SELECT attempts FROM notification.emails WHERE id = sqlc.arg(id)::uuid FOR UPDATE;

-- name: RecordFailure :exec
-- After enough failures the caller passes dead_at, because an email nobody
-- can deliver is an operational fact somebody needs to see rather than a row
-- retried silently forever.
UPDATE notification.emails
SET attempts = sqlc.arg(attempts)::integer,
    last_error = sqlc.arg(last_error)::text,
    next_attempt_at = now() + make_interval(secs => sqlc.arg(backoff_seconds)::double precision),
    dead_at = sqlc.narg(dead_at)::timestamptz
WHERE id = sqlc.arg(id)::uuid;

-- name: DeliveryStatusByID :one
-- The delivery facts about one email, for a context that enqueued it and now
-- wants to show whether it arrived. Only the status columns: the body is
-- nulled at send and is never anyone else's to read. A caller that holds an
-- id from a rolled-back transaction finds no row, which the reader turns into
-- an "unknown" rather than a delivered-or-not it cannot honestly answer.
SELECT sent_at, bounced_at, complained_at, dead_at, attempts, last_error
FROM notification.emails
WHERE id = sqlc.arg(id)::uuid;
