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
       coalesce(timing_policy_version, 0)::integer AS timing_policy_version,
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

-- name: CurrentTimingPolicy :one
-- The rules in force now: the highest published version. Sessions stamp
-- it at start so later policy changes never rewrite a running session.
SELECT version, reconnect_grace_seconds, max_overrun_seconds
FROM interview.timing_policies
ORDER BY version DESC
LIMIT 1;

-- name: StampTimingPolicy :exec
UPDATE interview.sessions
SET timing_policy_version = sqlc.arg(version)::integer
WHERE id = sqlc.arg(id)::uuid AND timing_policy_version IS NULL;

-- name: InsertMediaTrack :execrows
-- Idempotent per (session, track): a reconnection retrying the start must
-- converge on the one egress already running, never begin a second.
INSERT INTO interview.media_tracks
    (id, session_id, mode, candidate_id, tenant_id, track, storage_key, egress_id)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(session_id)::uuid, sqlc.arg(mode)::text,
        sqlc.arg(candidate_id)::uuid, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(track)::text, sqlc.arg(storage_key)::text, sqlc.arg(egress_id)::text)
ON CONFLICT (session_id, track) DO NOTHING;

-- name: ClaimMediaTrack :execrows
-- Take the (session, track) slot before any egress is started.
--
-- The claim comes first and the egress id arrives afterwards, which is the
-- opposite of the original order and the reason it had to change: a unique
-- constraint protects the row, not the external side effect, so checking and
-- then calling LiveKit lets two deliveries both call it and only one keep its
-- egress id. The other job records the same participant to the same key and
-- nothing ever stops it.
--
-- The conflict branch takes over a claim that never started, which is what a
-- provider failure leaves behind. Deleting such a row was the first attempt
-- and cannot work here: this table grants no DELETE, and under FORCE row-level
-- security a delete with no matching policy removes nothing and raises
-- nothing, so the claim survived and every retry found the slot taken and
-- started silently nothing at all. Claiming by update needs no new grant and
-- has the same effect.
--
-- Age is what separates an abandoned claim from one still in flight, and
-- leaving it out was a real bug rather than a detail. egress_id is empty for
-- the whole window between taking the claim and recording the id, so a
-- takeover conditioned on emptiness alone fires against a delivery that is
-- mid-call: both callers claimed, both started egress, and two jobs recorded
-- the same participant to the same key. That is exactly the duplicate this
-- query exists to prevent, reintroduced by the branch meant to allow retries.
--
-- stale_after must therefore exceed the time a live attempt can hold an empty
-- claim, which is bounded by the egress call's own 15 second timeout.
-- Production passes minutes; a test passes something small so it can prove the
-- takeover without waiting. created_at is reset on takeover so the new owner
-- gets a full window rather than inheriting the dead one's.
--
-- Zero rows affected means somebody else owns this track and has an egress
-- running. That is an answer, not an error, and the caller must not start one.
INSERT INTO interview.media_tracks
    (id, session_id, mode, candidate_id, tenant_id, track, storage_key, egress_id)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(session_id)::uuid, sqlc.arg(mode)::text,
        sqlc.arg(candidate_id)::uuid, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(track)::text, sqlc.arg(storage_key)::text, '')
ON CONFLICT (session_id, track) DO UPDATE
    SET storage_key = excluded.storage_key,
        created_at  = now()
    WHERE interview.media_tracks.egress_id = ''
      AND interview.media_tracks.created_at < now() - sqlc.arg(stale_after)::interval;

-- name: RecordTrackEgress :execrows
-- Attach the egress id to a claim this caller owns.
--
-- Only while the id is still empty, so a caller that lost its claim cannot
-- overwrite the winner's egress id and orphan its job.
UPDATE interview.media_tracks
SET egress_id = sqlc.arg(egress_id)::text
WHERE session_id = sqlc.arg(session_id)::uuid
  AND track = sqlc.arg(track)::text
  AND egress_id = '';

-- name: ListMediaTracks :many
SELECT id::text AS id, track, storage_key, egress_id, state, digest,
       size_bytes, created_at, resolved_at
FROM interview.media_tracks
WHERE session_id = sqlc.arg(session_id)::uuid
ORDER BY track;

-- name: ResolveMediaTrack :execrows
-- One-way: only a recording row resolves, to finalized or missing, once.
UPDATE interview.media_tracks
SET state = sqlc.arg(state)::text,
    digest = sqlc.arg(digest)::text,
    size_bytes = sqlc.arg(size_bytes)::bigint,
    resolved_at = now()
WHERE session_id = sqlc.arg(session_id)::uuid
  AND track = sqlc.arg(track)::text
  AND state = 'recording';

-- name: ListSessions :many
-- The owner's history, newest first. RLS scopes the rows; the ORDER is
-- the only policy here.
SELECT id::text AS id, mode, candidate_id::text AS candidate_id,
       coalesce(tenant_id::text, '')::text AS tenant_id,
       blueprint_id, config, recording_preference, consent_version,
       connection_epoch, accepted_sequence, state, version,
       coalesce(bundle_ref, '')::text AS bundle_ref,
       coalesce(bundle_digest, '')::text AS bundle_digest,
       coalesce(bundle_revision, 0)::integer AS bundle_revision,
       coalesce(failure_code, '')::text AS failure_code,
       coalesce(timing_policy_version, 0)::integer AS timing_policy_version,
       created_at, state_changed_at
FROM interview.sessions
ORDER BY created_at DESC;

-- name: InsertRedo :exec
INSERT INTO interview.redos (parent_session_id, sequence, redo_session_id, mode, candidate_id)
VALUES (sqlc.arg(parent_session_id)::uuid, sqlc.arg(sequence)::integer,
        sqlc.arg(redo_session_id)::uuid, 'practice', sqlc.arg(candidate_id)::uuid);

-- name: ListRedos :many
SELECT sequence, redo_session_id::text AS redo_session_id, created_at
FROM interview.redos
WHERE parent_session_id = sqlc.arg(parent_session_id)::uuid
ORDER BY sequence;

-- name: ReleaseMediaClaim :execrows
-- Give up a claim this caller took and could not use.
--
-- Ages the row out rather than deleting it, because this table grants no
-- DELETE and under FORCE row-level security a delete with no matching policy
-- removes nothing and raises nothing. The next attempt then sees a claim past
-- any staleness window and adopts it immediately.
--
-- Only while the egress id is empty, so this can never orphan a running job:
-- a caller that lost the race and is releasing what it thinks is its own claim
-- cannot age out the winner's.
UPDATE interview.media_tracks
SET created_at = 'epoch'::timestamptz
WHERE session_id = sqlc.arg(session_id)::uuid
  AND track = sqlc.arg(track)::text
  AND egress_id = '';
