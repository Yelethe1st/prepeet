-- Tenant administration's queries. sqlc generates the Go beside this file;
-- ADR-0010 records why no SQL lives in Go source.
--
-- Tables this module owns: tenancy.tenant_configuration,
-- tenancy.access_reviews, tenancy.access_review_items. It reads audit.events,
-- which no context owns, and writes its own rows there. It reads nothing else
-- belonging to another module: the roster of members arrives through a port,
-- because who belongs to a workspace is identity's answer and not this
-- module's query.

-- ───────────────────────────────────────────────── settings (TEN-01)

-- name: CurrentConfiguration :one
-- The highest version is the current one. There is no "is_current" flag to
-- get out of step with the rows.
SELECT version, settings, changed_by::text AS changed_by, changed_at
FROM tenancy.tenant_configuration
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
ORDER BY version DESC
LIMIT 1;

-- name: ConfigurationAtVersion :one
-- The pin re-read: what a campaign created at this version was created
-- under. Append-only storage is what makes this answer stable.
SELECT version, settings, changed_by::text AS changed_by, changed_at
FROM tenancy.tenant_configuration
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND version = sqlc.arg(version)::integer;

-- name: InsertConfiguration :exec
-- A save is an insert. Two administrators saving the same version collide on
-- the primary key, so the second is refused rather than quietly winning.
INSERT INTO tenancy.tenant_configuration (tenant_id, version, settings, changed_by)
VALUES (sqlc.arg(tenant_id)::uuid, sqlc.arg(version)::integer,
        sqlc.arg(settings)::jsonb, sqlc.arg(changed_by)::uuid);

-- name: ConfigurationHistory :many
-- Every version, newest first, with who saved it. The document is returned
-- too: "what was it before" is answered by reading the row, not by trusting
-- a description of the change.
SELECT version, settings, changed_by::text AS changed_by, changed_at
FROM tenancy.tenant_configuration
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
ORDER BY version DESC;

-- ───────────────────────────────────────────── access review (TEN-03)

-- name: InsertAccessReview :exec
INSERT INTO tenancy.access_reviews
    (id, tenant_id, opened_by, due_at, dormant_after_days)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(tenant_id)::uuid, sqlc.arg(opened_by)::uuid,
        sqlc.arg(due_at)::timestamptz, sqlc.arg(dormant_after_days)::integer);

-- name: FindOpenAccessReview :one
SELECT id::text AS id, status, opened_at, coalesce(opened_by::text, '')::text AS opened_by,
       due_at, dormant_after_days, completed_at,
       coalesce(completed_by::text, '')::text AS completed_by
FROM tenancy.access_reviews
WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND status = 'open';

-- name: GetAccessReview :one
SELECT id::text AS id, status, opened_at, coalesce(opened_by::text, '')::text AS opened_by,
       due_at, dormant_after_days, completed_at,
       coalesce(completed_by::text, '')::text AS completed_by
FROM tenancy.access_reviews
WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND id = sqlc.arg(id)::uuid;

-- name: LatestCompletedAccessReview :one
-- What "due" is measured from. A workspace that has never completed one is
-- due immediately, which is the absence of a row rather than a special case.
SELECT id::text AS id, status, opened_at, coalesce(opened_by::text, '')::text AS opened_by,
       due_at, dormant_after_days, completed_at,
       coalesce(completed_by::text, '')::text AS completed_by
FROM tenancy.access_reviews
WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND status = 'completed'
ORDER BY completed_at DESC
LIMIT 1;

-- name: InsertAccessReviewItem :exec
INSERT INTO tenancy.access_review_items
    (id, tenant_id, review_id, membership_id, user_id, role, last_active_at, dormant)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(tenant_id)::uuid, sqlc.arg(review_id)::uuid,
        sqlc.arg(membership_id)::uuid, sqlc.arg(user_id)::uuid, sqlc.arg(role)::text,
        sqlc.narg(last_active_at)::timestamptz, sqlc.arg(dormant)::boolean);

-- name: ListAccessReviewItems :many
-- Dormant first, because the prompt exists to put the access nobody has used
-- in front of the person who can remove it.
SELECT id::text AS id, review_id::text AS review_id, membership_id::text AS membership_id,
       user_id::text AS user_id, role, last_active_at, dormant, decision,
       decided_at, coalesce(decided_by::text, '')::text AS decided_by, note
FROM tenancy.access_review_items
WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND review_id = sqlc.arg(review_id)::uuid
ORDER BY dormant DESC, last_active_at ASC NULLS FIRST, id;

-- name: GetAccessReviewItem :one
SELECT id::text AS id, review_id::text AS review_id, membership_id::text AS membership_id,
       user_id::text AS user_id, role, last_active_at, dormant, decision,
       decided_at, coalesce(decided_by::text, '')::text AS decided_by, note
FROM tenancy.access_review_items
WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND id = sqlc.arg(id)::uuid;

-- name: DecideAccessReviewItem :execrows
-- Guarded on still being pending, so a second reviewer deciding the same
-- item loses on rows affected rather than overwriting the first decision.
UPDATE tenancy.access_review_items
SET decision = sqlc.arg(decision)::text,
    decided_at = now(),
    decided_by = sqlc.arg(decided_by)::uuid,
    note = sqlc.arg(note)::text
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND id = sqlc.arg(id)::uuid
  AND decision = 'pending';

-- name: CountPendingAccessReviewItems :one
SELECT count(*) AS pending
FROM tenancy.access_review_items
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND review_id = sqlc.arg(review_id)::uuid
  AND decision = 'pending';

-- name: CompleteAccessReview :execrows
-- Guarded on still being open for the same reason the item decision is.
UPDATE tenancy.access_reviews
SET status = 'completed', completed_at = now(), completed_by = sqlc.arg(completed_by)::uuid
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
  AND id = sqlc.arg(id)::uuid
  AND status = 'open';

-- name: LastActivityByActor :many
-- When each person was last seen acting in this workspace.
--
-- Read from the audit trail because that is where a workspace's record of
-- what people did lives, and it is the only source that is tenant-scoped:
-- a session belongs to a person across every workspace they belong to, so it
-- cannot answer "dormant here". The consequence is stated where it is used:
-- somebody who only ever read pages has no audited act and reads as dormant,
-- which prompts a confirmation rather than hiding them.
SELECT actor_id::text AS actor_id, max(occurred_at)::timestamptz AS last_active_at
FROM audit.events
WHERE tenant_id = sqlc.arg(tenant_id)::uuid AND actor_id IS NOT NULL
GROUP BY actor_id;

-- ─────────────────────────────────────────────────────────── audit

-- name: InsertTenantAuditEvent :exec
-- This module's audit append. Settings versions and review completions are
-- the decisions worth reviewing later.
INSERT INTO audit.events
    (id, tenant_id, actor_id, actor_type, action, subject_type, subject_id, outcome, detail)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(tenant_id)::uuid, sqlc.arg(actor_id)::uuid, 'user',
        sqlc.arg(action)::text, sqlc.arg(subject_type)::text, sqlc.arg(subject_id)::text,
        'allowed', sqlc.arg(detail)::jsonb);
