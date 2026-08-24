# ADR-0008: `net/http` routing and an in-repo migration runner, in place of `chi` and `goose`

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-24  
**Review date:** 2027-02-24  
**Supersedes:** None  
**Superseded by:** None

Records two departures from the Go row of the technology baseline in
[architecture-and-implementation-brief.md](../architecture-and-implementation-brief.md).
A third, `sqlc`, was left open here and has since been decided in [ADR-0010](0010-sqlc-generates-the-repositories-sql-access.md).

## Context

The brief names `chi`, `pgx`, `sqlc` and `goose`. `pgx` is used as written. The other three were
not, and until now that was drift rather than a decision, which is the failure this ADR exists to
correct: a baseline nobody follows and nobody amends stops being a baseline.

Both departures below were made while building something else, which is exactly how a stack ends up
unrecognisable one commit at a time. Each is re-argued here on its merits and would be reverted if
the argument did not hold.

## Decision

### Routing: `net/http.ServeMux`

**`chi` is not used. The standard library's `ServeMux` routes the control plane.**

The reason `chi` was worth a dependency was that `ServeMux` could not route by method or capture a
path segment, so every project wrote the same dispatch by hand. Go 1.22 put both in the standard
library: `mux.HandleFunc("POST /v1/sessions/{id}/start", ...)` and `r.PathValue("id")`. The gap
`chi` filled is closed, and the parts of it we would still use — a middleware chain and sub-routers —
are an `http.Handler` wrapping another `http.Handler`, which is the standard library's own shape.

Three things make the standard library the better side of this trade here rather than the merely
cheaper one:

The transport layer is generated. [ADR-0004](0004-contract-conventions-and-code-generation.md)
makes OpenAPI the source and `oapi-codegen` emits a strict server, so almost nothing is routed by
hand. `oapi-codegen` targets `net/http` and `chi` equally well, and the amount of code that would
differ between them is the handful of lines that mount the generated handler.

It is one fewer dependency on the path every authenticated request takes, which is the path where a
supply chain problem matters most.

`ServeMux` has one behaviour that had to be handled either way, and handling it is why
`platform/httpserver` exists: its built-in 404 and 405 write a plain text body, which would be the
one response in the API not shaped like the error envelope the contract promises. `httpserver`
converts both. `chi` would have needed the same wrapper for the same reason.

**Cost accepted.** `ServeMux` has no route groups, so a prefix applied to a set of routes is written
out rather than declared, and no built-in middleware, so anything `chi` ships — request ID, real IP,
timeout — is ours to write or vendor. We already write our own, because tracing, tenancy and
authorization are not generic.

### Migrations: the runner in `platform/database`

**`goose` is not used. Migrations are numbered SQL files embedded in the binary and applied by
`Migrate` in `platform/database`.**

This follows from a decision already accepted rather than being a fresh preference.
[ADR-0002](0002-postgresql-schema-rls-and-connection-roles.md) requires that migrations be forward
only, numbered, applied in a transaction, and **checksummed**, and lists "an applied migration is
edited" as a threat answered by "checksums are recorded and verified on every run".

`goose` records which versions have been applied. It does not record what they contained, so a
migration edited after it was applied is silently skipped as already done, and two environments then
hold different schemas while both report themselves up to date. That is precisely the failure
ADR-0002 named, and it is not a gap that can be closed by configuring `goose`.

The runner is small because it does less: read the embedded files, compare each against the recorded
checksum, refuse on any mismatch with a message naming the file, and apply the rest inside one
transaction. It also runs as the owner role rather than the application role, which
[ADR-0002](0002-postgresql-schema-rls-and-connection-roles.md) requires because the application role
is subject to the row-level policies the migration is creating.

**Cost accepted.** No down migrations and no `goose` ecosystem tooling. Down migrations are not a
loss, because forward-only was already the decision: a rollback that runs untested SQL against
production data during an incident is the worst moment to discover it is wrong.

**Verified by breaking it.** `migrate_test.go` applies a migration, edits its content, and asserts
the second run refuses rather than reporting success. Without the checksum comparison that test
passes silently, which is the whole point.

## Resolved: `sqlc`

This ADR originally left `sqlc` open, recording that it had never been evaluated and that inventing a
case against it after the fact would be the same drift wearing a decision's clothes. It was adopted
on 2026-08-24 and the reasoning is in
[ADR-0010](0010-sqlc-generates-the-repositories-sql-access.md).

## Consequences

- The brief's Go row now reads with a pointer to this ADR, so the baseline and the code agree.
- Adding `chi` later is a small change, because the generated handler mounts into either.
- Replacing the runner with `goose` would require re-opening ADR-0002's checksum requirement.
- `sqlc` was closed by ADR-0010 rather than left as a silent difference.

## What would change this

- `ServeMux` routing becoming a material share of hand-written transport code, which would mean the
  generated server is no longer carrying it.
- A migration need that genuinely requires a down path, such as an expand-and-contract step that
  must be abandoned mid-release.
- `goose` gaining checksum verification, which would remove the only argument above.
