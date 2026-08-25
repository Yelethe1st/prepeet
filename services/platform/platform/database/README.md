# platform/database

## What this owns

The PostgreSQL schema, the migration runner, and the tenant isolation that sits under every query.
Migrations live in `sql/`, are numbered, forward only, and applied by `cmd/migrate`.

Every tenant-owned table is created with its row-level security policy in the same migration that
creates the table, and the policy is FORCEd. See
[ADR-0002](../../../../docs/architecture/decisions/0002-postgresql-schema-rls-and-connection-roles.md).

## What this must never do

A migration never creates a tenant-owned table without a forced policy: the integration suite fails the
build if one appears. An applied migration is never edited in place, because two environments running
different SQL under the same version number is a difference nobody notices until it matters; the runner
verifies checksums and refuses.

Tenant context is set with `SET LOCAL` and never with `SET`. A pooled connection carrying a previous
request's tenant is exactly the bug row-level security exists to catch, so the mechanism must not
create it.

## Writing a policy

Use `NULLIF(current_setting('app.tenant_id', true), '')::uuid`, not the bare `current_setting`. A
setting that was never set reads as NULL, but one set with `SET LOCAL` reverts to the empty string when
the transaction ends, and `''::uuid` raises rather than matching nothing. Without `NULLIF` the first
unscoped query after any tenant-scoped transaction on a pooled connection fails with a cast error
instead of quietly returning no rows.

## The practice/screening separation

Migration 0011 establishes the shape every candidate-owned table carries:
owner-scoped row-level security with no tenant branch, no tenant_id column,
and a trigger that refuses any write inside a transaction carrying tenant
context. That trigger exists because it catches the one shape the policy
cannot: the owner's own row, written through a code path that also set tenant
context, which WITH CHECK happily passes.

The isolation suite enforces the shape structurally: a candidate table that
grows a tenant column, loses FORCE, or gains a policy consulting
app.tenant_id fails the suite, and so does any view or materialized view
whose definition joins a candidate table to anything carrying tenant_id. The
view detector proves itself against a planted offender on every run before
checking the real schema.

A refusal from the trigger says "stop-ship" in its message on purpose:
whoever meets it in a log is looking at the practice/screening separation
failing, not at an input error to retry.
