-- The session store's queries. sqlc generates the Go beside this file;
-- ADR-0010 records why no SQL lives in Go source.

-- name: InsertSession :exec
INSERT INTO interview.sessions (id, mode, candidate_id, tenant_id, blueprint_id, config,
                                recording_preference, consent_version)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(mode)::text, sqlc.arg(candidate_id)::uuid,
        nullif(sqlc.arg(tenant_id)::text, '')::uuid, sqlc.arg(blueprint_id)::text,
        sqlc.arg(config)::jsonb,
        sqlc.arg(recording_preference)::text, sqlc.arg(consent_version)::text);

-- name: GetSession :one
SELECT id::text AS id, mode, candidate_id::text AS candidate_id,
       coalesce(tenant_id::text, '')::text AS tenant_id,
       blueprint_id, config, recording_preference, consent_version,
       connection_epoch, accepted_sequence, state, version,
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

-- ── RTC-02: attempts, epochs and the control event log.

-- name: SupersedeAttempts :exec
UPDATE interview.attempts SET superseded_at = now()
WHERE session_id = sqlc.arg(session_id)::uuid AND superseded_at IS NULL;

-- name: InsertAttempt :exec
INSERT INTO interview.attempts
    (id, session_id, mode, candidate_id, tenant_id, connection_epoch)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(session_id)::uuid, sqlc.arg(mode)::text,
        sqlc.arg(candidate_id)::uuid, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(connection_epoch)::integer);

-- name: AdvanceSessionEpoch :execrows
-- Monotonic by guard: an epoch can only go up, and the cursor resets with
-- it because sequence orders within an epoch.
UPDATE interview.sessions
SET connection_epoch = sqlc.arg(epoch)::integer, accepted_sequence = 0
WHERE id = sqlc.arg(id)::uuid AND connection_epoch < sqlc.arg(epoch)::integer;

-- name: InsertControlEvent :exec
INSERT INTO interview.control_events
    (event_id, session_id, mode, candidate_id, tenant_id,
     connection_epoch, sequence, event_type, payload, occurred_at)
VALUES (sqlc.arg(event_id)::uuid, sqlc.arg(session_id)::uuid, sqlc.arg(mode)::text,
        sqlc.arg(candidate_id)::uuid, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(connection_epoch)::integer, sqlc.arg(sequence)::integer,
        sqlc.arg(event_type)::text, sqlc.arg(payload)::jsonb, sqlc.arg(occurred_at)::timestamptz);

-- name: ControlEventExists :one
SELECT EXISTS (
    SELECT 1 FROM interview.control_events WHERE event_id = sqlc.arg(event_id)::uuid
) AS present;

-- name: StoredSequences :many
-- The contiguity computation's input: which slots this epoch holds.
SELECT sequence FROM interview.control_events
WHERE session_id = sqlc.arg(session_id)::uuid
  AND connection_epoch = sqlc.arg(epoch)::integer
ORDER BY sequence;

-- name: PersistCursor :execrows
-- Guarded by epoch so a cursor from a superseded attempt cannot land after
-- a takeover already moved the session on.
UPDATE interview.sessions
SET accepted_sequence = sqlc.arg(accepted)::integer
WHERE id = sqlc.arg(id)::uuid
  AND connection_epoch = sqlc.arg(epoch)::integer
  AND accepted_sequence < sqlc.arg(accepted)::integer;

-- name: ReplayControlEvents :many
-- The replay read: everything after the cursor, in the one authoritative
-- order. Replaying twice from the same cursor answers identically, which
-- is the property the client rebuilds itself on.
SELECT event_id::text AS event_id, connection_epoch, sequence, event_type,
       payload, occurred_at
FROM interview.control_events
WHERE session_id = sqlc.arg(session_id)::uuid
  AND (connection_epoch, sequence) > (sqlc.arg(after_epoch)::integer, sqlc.arg(after_sequence)::integer)
ORDER BY connection_epoch, sequence;

-- ── SES-04: the seal.

-- name: InsertSeal :exec
INSERT INTO interview.seals
    (session_id, mode, candidate_id, tenant_id, sealed_epoch, sealed_sequence,
     gaps, transcript_digest, bundle_digest, media_status, warnings,
     evaluation_input_digest)
VALUES (sqlc.arg(session_id)::uuid, sqlc.arg(mode)::text, sqlc.arg(candidate_id)::uuid,
        nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(sealed_epoch)::integer, sqlc.arg(sealed_sequence)::integer,
        sqlc.arg(gaps)::jsonb, sqlc.arg(transcript_digest)::text,
        sqlc.arg(bundle_digest)::text, sqlc.arg(media_status)::text,
        sqlc.arg(warnings)::jsonb, sqlc.arg(evaluation_input_digest)::text);

-- name: GetSeal :one
SELECT session_id::text AS session_id, sealed_epoch, sealed_sequence,
       gaps, transcript_digest, bundle_digest, media_status, warnings,
       evaluation_input_digest, created_at
FROM interview.seals
WHERE session_id = sqlc.arg(session_id)::uuid;
