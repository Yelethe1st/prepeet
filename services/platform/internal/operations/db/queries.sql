-- The operations context's only SQL: the audit row behind every operator
-- action on failed work.
--
-- This context owns no tables. What it writes goes into audit.events, the one
-- table shared by every context, and deliberately so: an audit table per
-- feature is a trail nobody can read in one query, which is the only way a
-- trail is ever read.
--
-- The rows are untenanted. Working the backlog is a platform action rather than
-- a tenant one - the operator is not acting inside a workspace and may be
-- looking at work belonging to several - and migration 0008's own comment names
-- "a platform support action" as the case tenant_id is nullable for. The
-- affected tenant travels in detail, where it is an identifier rather than a
-- scope. The untenanted policy binds the row to the acting user, so the caller
-- must have set app.user_id in this transaction, which is also what stops this
-- becoming a way to write an audit row as somebody else.

-- name: InsertRecoveryAudit :exec
-- One row per operator decision about one item, successful or refused.
--
-- outcome carries the refusal: an operator who tried to retry work that had
-- already been retried is a fact worth keeping, because during an incident it
-- is usually the first sign that two people are working the same queue.
INSERT INTO audit.events
    (id, tenant_id, actor_id, actor_type, action, subject_type, subject_id,
     outcome, detail, request_id)
VALUES (sqlc.arg(id)::uuid, NULL, sqlc.arg(actor_id)::uuid, 'user',
        sqlc.arg(action)::text, 'outbox_event', sqlc.arg(subject_id)::text,
        sqlc.arg(outcome)::text, sqlc.arg(detail)::jsonb,
        nullif(sqlc.arg(request_id)::text, ''));
