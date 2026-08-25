-- The profile's queries. sqlc generates the Go beside this file; ADR-0010
-- records why no SQL lives in Go source.

-- name: GetProfile :one
SELECT user_id::text AS user_id, disciplines, target_roles,
       coalesce(seniority, '')::text AS seniority,
       coalesce(career_context, '')::text AS career_context,
       coalesce(default_duration_minutes, 0)::integer AS default_duration_minutes,
       coalesce(default_style, '')::text AS default_style,
       coalesce(default_pressure, '')::text AS default_pressure,
       extended_time, captions, reduced_motion,
       coalesce(accessibility_notes, '')::text AS accessibility_notes,
       notify_product_updates, notify_practice_reminders,
       created_at, updated_at
FROM candidate.profiles
WHERE user_id = sqlc.arg(user_id)::uuid;

-- name: UpsertProfile :exec
-- One statement for create and update, because the row's existence is an
-- implementation detail: a candidate who never saved has the empty profile,
-- and their first save is not a different operation from their tenth.
INSERT INTO candidate.profiles
    (user_id, disciplines, target_roles, seniority, career_context,
     default_duration_minutes, default_style, default_pressure,
     extended_time, captions, reduced_motion, accessibility_notes,
     notify_product_updates, notify_practice_reminders)
VALUES (sqlc.arg(user_id)::uuid, sqlc.arg(disciplines)::text[], sqlc.arg(target_roles)::text[],
        nullif(sqlc.arg(seniority)::text, ''), nullif(sqlc.arg(career_context)::text, ''),
        nullif(sqlc.arg(default_duration_minutes)::integer, 0),
        nullif(sqlc.arg(default_style)::text, ''), nullif(sqlc.arg(default_pressure)::text, ''),
        sqlc.arg(extended_time)::boolean, sqlc.arg(captions)::boolean,
        sqlc.arg(reduced_motion)::boolean, nullif(sqlc.arg(accessibility_notes)::text, ''),
        sqlc.arg(notify_product_updates)::boolean, sqlc.arg(notify_practice_reminders)::boolean)
ON CONFLICT (user_id) DO UPDATE SET
    disciplines = excluded.disciplines,
    target_roles = excluded.target_roles,
    seniority = excluded.seniority,
    career_context = excluded.career_context,
    default_duration_minutes = excluded.default_duration_minutes,
    default_style = excluded.default_style,
    default_pressure = excluded.default_pressure,
    extended_time = excluded.extended_time,
    captions = excluded.captions,
    reduced_motion = excluded.reduced_motion,
    accessibility_notes = excluded.accessibility_notes,
    notify_product_updates = excluded.notify_product_updates,
    notify_practice_reminders = excluded.notify_practice_reminders,
    updated_at = now();

-- name: NextDocumentVersion :one
-- The next version for one person's document kind, computed under the row
-- lock of the aggregate's own scope so two concurrent uploads cannot both be
-- version 3. Absence is version 1.
SELECT coalesce(max(version), 0)::integer + 1 AS next_version
FROM candidate.documents
WHERE user_id = sqlc.arg(user_id)::uuid AND kind = sqlc.arg(kind)::text;

-- name: InsertDocument :exec
INSERT INTO candidate.documents
    (id, user_id, kind, version, storage_key, media_type, size_bytes, upload_id)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(user_id)::uuid, sqlc.arg(kind)::text,
        sqlc.arg(version)::integer, sqlc.arg(storage_key)::text,
        sqlc.arg(media_type)::text, sqlc.arg(size_bytes)::bigint,
        sqlc.arg(upload_id)::text);

-- name: GetDocument :one
SELECT id::text AS id, user_id::text AS user_id, kind, version, storage_key,
       media_type, size_bytes, state,
       coalesce(upload_id, '')::text AS upload_id,
       coalesce(sha256, '')::text AS sha256,
       extraction_state, created_at, stored_at, deleted_at
FROM candidate.documents
WHERE id = sqlc.arg(id)::uuid;

-- name: ListDocuments :many
-- Every version, newest first: the history PRO-02 requires, states included,
-- so a stuck upload is visible beside the version that superseded it.
SELECT id::text AS id, user_id::text AS user_id, kind, version, storage_key,
       media_type, size_bytes, state,
       coalesce(upload_id, '')::text AS upload_id,
       coalesce(sha256, '')::text AS sha256,
       extraction_state, created_at, stored_at, deleted_at
FROM candidate.documents
WHERE user_id = sqlc.arg(user_id)::uuid AND kind = sqlc.arg(kind)::text
ORDER BY version DESC;

-- name: MarkDocumentStored :execrows
-- Guarded on the uploading state, so completing twice - or completing an
-- aborted upload - loses on rows affected rather than rewriting history.
-- Storing also queues extraction: pending is set here, in the same statement
-- that records the digest, so there is no stored document whose extraction
-- state nobody decided.
UPDATE candidate.documents
SET state = 'stored', sha256 = sqlc.arg(sha256)::text,
    size_bytes = sqlc.arg(size_bytes)::bigint, stored_at = now(), upload_id = NULL,
    extraction_state = 'pending'
WHERE id = sqlc.arg(id)::uuid AND state = 'uploading';

-- name: MarkDocumentFailed :execrows
UPDATE candidate.documents
SET state = 'failed', upload_id = NULL
WHERE id = sqlc.arg(id)::uuid AND state = 'uploading';

-- name: MarkDocumentDeleted :execrows
-- The row survives its object: the digest record is what a composed bundle
-- pinned, and destroying it would leave a session's provenance unanswerable.
UPDATE candidate.documents
SET state = 'deleted', deleted_at = now()
WHERE id = sqlc.arg(id)::uuid AND state = 'stored';

-- name: SetDocumentExtractionState :execrows
-- Moves extraction to its outcome. The guard admits pending - the normal
-- path - and the target state itself, so a workflow activity replayed after
-- its commit lands on success rather than on a refusal it must special-case.
UPDATE candidate.documents
SET extraction_state = sqlc.arg(state)::text
WHERE id = sqlc.arg(id)::uuid
  AND extraction_state IN ('pending', sqlc.arg(state)::text);

-- name: DeleteProposedFacts :exec
-- The idempotency half of storing an extraction: proposals for this document
-- are replaced wholesale, so a replayed StoreFacts converges instead of
-- duplicating. Rows the candidate has already acted on keep their status and
-- are left alone.
DELETE FROM candidate.extracted_facts
WHERE document_id = sqlc.arg(document_id)::uuid AND status = 'proposed';

-- name: InsertFact :exec
INSERT INTO candidate.extracted_facts
    (id, user_id, document_id, kind, value, span_start, span_end,
     confidence, extractor_version)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(user_id)::uuid, sqlc.arg(document_id)::uuid,
        sqlc.arg(kind)::text, sqlc.arg(value)::jsonb,
        sqlc.arg(span_start)::integer, sqlc.arg(span_end)::integer,
        sqlc.arg(confidence)::float8, sqlc.arg(extractor_version)::text);

-- name: ListFactsByDocument :many
-- In span order, because the facts are read beside the document they came
-- from and the span is the join.
SELECT id::text AS id, document_id::text AS document_id, kind, value,
       span_start, span_end, confidence::float8 AS confidence,
       extractor_version, status, created_at
FROM candidate.extracted_facts
WHERE document_id = sqlc.arg(document_id)::uuid
ORDER BY span_start, span_end, kind;
