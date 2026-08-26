-- The billing ledger's queries. sqlc generates the Go beside this file;
-- ADR-0010 records why no SQL lives in Go source.

-- name: LockQuota :one
-- The reservation's serialisation point: concurrent starts at the boundary
-- queue on this row lock rather than both passing at limit minus one.
-- Absent row means no quota is configured, which is unlimited.
SELECT session_limit, warn_threshold::float8 AS warn_threshold
FROM billing.quotas
WHERE tenant_id = sqlc.arg(tenant_id)::uuid
FOR UPDATE;

-- name: GetQuota :one
SELECT session_limit, warn_threshold::float8 AS warn_threshold, updated_at
FROM billing.quotas
WHERE tenant_id = sqlc.arg(tenant_id)::uuid;

-- name: UpsertQuota :exec
-- Configuration, not ledger: the quota row may change; the entries never do.
INSERT INTO billing.quotas (tenant_id, session_limit, warn_threshold, updated_at)
VALUES (sqlc.arg(tenant_id)::uuid,
        nullif(sqlc.arg(session_limit)::integer, -1),
        sqlc.arg(warn_threshold)::float8, now())
ON CONFLICT (tenant_id) DO UPDATE SET
    session_limit = excluded.session_limit,
    warn_threshold = excluded.warn_threshold,
    updated_at = now();

-- name: CountBillableStarts :one
-- What counts against the quota: starts minus credits. Computed from the
-- ledger every time rather than kept as a counter, because a counter is a
-- second copy of the truth and this query is one indexed aggregate.
SELECT (count(*) FILTER (WHERE kind = 'session_started')
      - count(*) FILTER (WHERE kind = 'start_credited'))::integer AS billable
FROM billing.usage_entries
WHERE tenant_id = sqlc.arg(tenant_id)::uuid;

-- name: InsertUsageEntry :exec
INSERT INTO billing.usage_entries (id, tenant_id, session_id, kind, reason, mode)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(tenant_id)::uuid, sqlc.arg(session_id)::uuid,
        sqlc.arg(kind)::text, sqlc.arg(reason)::text, sqlc.arg(mode)::text);

-- name: HasEntry :one
SELECT EXISTS (
    SELECT 1 FROM billing.usage_entries
    WHERE session_id = sqlc.arg(session_id)::uuid AND kind = sqlc.arg(kind)::text
) AS present;

-- name: UsageSummary :one
SELECT count(*) FILTER (WHERE kind = 'session_started')::integer AS started,
       count(*) FILTER (WHERE kind = 'start_credited')::integer  AS credited
FROM billing.usage_entries
WHERE tenant_id = sqlc.arg(tenant_id)::uuid;
