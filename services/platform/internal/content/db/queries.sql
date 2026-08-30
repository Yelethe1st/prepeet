-- The registry's queries. sqlc generates the Go beside this file; ADR-0010
-- records why no SQL lives in Go source.

-- name: InsertArtifact :exec
INSERT INTO content.artifacts
    (id, artifact_type, reference, version, schema_version, digest, body, tenant_id, created_by)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(artifact_type)::text, sqlc.arg(reference)::text,
        sqlc.arg(version)::text, sqlc.arg(schema_version)::text, sqlc.arg(digest)::text,
        sqlc.arg(body)::jsonb, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(created_by)::uuid);

-- name: GetArtifact :one
SELECT id::text AS id, artifact_type, reference, version, schema_version, digest, body,
       status, coalesce(tenant_id::text, '')::text AS tenant_id,
       created_by::text AS created_by, created_at,
       coalesce(published_by::text, '')::text AS published_by, published_at
FROM content.artifacts
WHERE id = sqlc.arg(id)::uuid;

-- name: GetArtifactByVersion :one
SELECT id::text AS id, artifact_type, reference, version, schema_version, digest, body,
       status, coalesce(tenant_id::text, '')::text AS tenant_id,
       created_by::text AS created_by, created_at,
       coalesce(published_by::text, '')::text AS published_by, published_at
FROM content.artifacts
WHERE reference = sqlc.arg(reference)::text
  AND version = sqlc.arg(version)::text
  AND tenant_id IS NOT DISTINCT FROM nullif(sqlc.arg(tenant_id)::text, '')::uuid;

-- name: GetArtifactByDigest :one
-- The pin-resolution path: anything holding a digest reads by it, and what it
-- reads can never differ from what was pinned, because published rows are
-- immutable and never deleted.
SELECT id::text AS id, artifact_type, reference, version, schema_version, digest, body,
       status, coalesce(tenant_id::text, '')::text AS tenant_id,
       created_by::text AS created_by, created_at,
       coalesce(published_by::text, '')::text AS published_by, published_at
FROM content.artifacts
WHERE digest = sqlc.arg(digest)::text;

-- name: TransitionArtifact :execrows
-- Guarded by the current status, so a concurrent transition loses on rows
-- affected rather than overwriting.
UPDATE content.artifacts
SET status = sqlc.arg(to_status)::text
WHERE id = sqlc.arg(id)::uuid AND status = sqlc.arg(from_status)::text;

-- name: MarkPublished :execrows
-- Publication stamps provenance in the same statement that moves the status,
-- so a published row without a publisher cannot exist even transiently.
UPDATE content.artifacts
SET status = 'published',
    published_by = sqlc.arg(published_by)::uuid,
    published_at = now()
WHERE id = sqlc.arg(id)::uuid AND status = 'approved';

-- name: UpsertPointer :exec
-- The one mutable surface. Publication and rollback both land here, and the
-- audit trail is where the history of moves lives.
INSERT INTO content.artifact_pointers (reference, tenant_id, artifact_id, updated_by)
VALUES (sqlc.arg(reference)::text, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(artifact_id)::uuid, sqlc.arg(updated_by)::uuid)
ON CONFLICT (tenant_id, reference)
DO UPDATE SET artifact_id = excluded.artifact_id,
              updated_by = excluded.updated_by,
              updated_at = now();

-- name: ResolvePointer :one
-- What a composition uses next: the pointer's target, joined so one round
-- trip answers both "which version" and "with what content".
--
-- A tenant resolving a reference gets its own pointer where one exists and
-- the platform's otherwise - a tenant's calibration of a rubric overrides
-- the platform default without hiding the catalogue from everyone else. The
-- ORDER BY is that precedence; row-level security has already removed other
-- tenants' pointers before it runs.
SELECT a.id::text AS id, a.artifact_type, a.reference, a.version, a.schema_version,
       a.digest, a.body, a.status, coalesce(a.tenant_id::text, '')::text AS tenant_id,
       a.created_by::text AS created_by, a.created_at,
       coalesce(a.published_by::text, '')::text AS published_by, a.published_at
FROM content.artifact_pointers p
JOIN content.artifacts a ON a.id = p.artifact_id
WHERE p.reference = sqlc.arg(reference)::text
  AND (p.tenant_id IS NULL
       OR p.tenant_id = nullif(sqlc.arg(tenant_id)::text, '')::uuid)
ORDER BY (p.tenant_id IS NOT NULL) DESC
LIMIT 1;

-- name: InsertAuditEvent :exec
-- Content's own audit append; pointer moves are the decisions worth reviewing.
INSERT INTO audit.events
    (id, tenant_id, actor_id, actor_type, action, subject_type, subject_id, outcome)
VALUES (sqlc.arg(id)::uuid, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(actor_id)::uuid, 'user', sqlc.arg(action)::text,
        'artifact', sqlc.arg(subject_id)::text, sqlc.arg(outcome)::text);

-- name: ListArtifactVersions :many
-- Every version of one reference, newest first, with who published it and
-- when. The registry already holds this; nothing else needs to keep a second
-- copy of what a version is, which is what TEN-04's library is built on.
SELECT id::text AS id, artifact_type, reference, version, schema_version, digest, body,
       status, coalesce(tenant_id::text, '')::text AS tenant_id,
       created_by::text AS created_by, created_at,
       coalesce(published_by::text, '')::text AS published_by, published_at
FROM content.artifacts
WHERE reference = sqlc.arg(reference)::text
  AND tenant_id IS NOT DISTINCT FROM nullif(sqlc.arg(tenant_id)::text, '')::uuid
ORDER BY created_at DESC;

-- name: ListArtifactsByType :many
-- Everything of one type this caller may see: the tenant's own and the
-- platform catalogue's, which row-level security has already narrowed to
-- exactly those two. The ordering puts a tenant's own first, because a
-- workspace's library is its own work with the platform's templates beneath.
SELECT id::text AS id, artifact_type, reference, version, schema_version, digest, body,
       status, coalesce(tenant_id::text, '')::text AS tenant_id,
       created_by::text AS created_by, created_at,
       coalesce(published_by::text, '')::text AS published_by, published_at
FROM content.artifacts
WHERE artifact_type = sqlc.arg(artifact_type)::text
ORDER BY (tenant_id IS NOT NULL) DESC, reference, created_at DESC;

-- name: DeleteDraftArtifact :execrows
-- Only a draft, and the trigger in migration 0013 refuses anything else in
-- any case. The status predicate is here so the refusal is a row count the
-- caller can act on rather than an exception it has to parse.
DELETE FROM content.artifacts
WHERE id = sqlc.arg(id)::uuid AND status = 'draft';
