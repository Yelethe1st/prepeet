# Architecture Decisions

This directory contains approved Architecture Decision Records (ADRs). Proposed architecture elsewhere in `docs/` does not substitute for a decision where alternatives materially affect cost, risk, security, or reversibility.

## Accepted

| ADR | Decision | Date |
|---|---|---|
| [0001](0001-hosting-platform-and-regional-topology.md) | Hosting platform and regional topology: AWS, `eu-west-2` London, in-region storage with disclosed AI sub-processors, single region with PITR | 2026-08-23 |
| [0002](0002-postgresql-schema-rls-and-connection-roles.md) | PostgreSQL: one database, a schema per module, forced row-level security on every tenant-owned table, tenant context set per transaction | 2026-08-24 |
| [0003](0003-identity-built-in-go.md) | Authentication built in Go with argon2id and revocable opaque sessions; enterprise federation deferred behind an adapter | 2026-08-24 |
| [0004](0004-contract-conventions-and-code-generation.md) | Contracts are hand-authored and everything else is generated from them; REST, RPC and event conventions, and what counts as a breaking change | 2026-08-24 |
| [0005](0005-module-boundaries-and-extraction.md) | No context imports another: consumer-defined interfaces for synchronous reads, events for state changes, enforced by a test | 2026-08-24 |
| [0006](0006-postgresql-serves-cache-coordination-and-rate-limiting.md) | PostgreSQL serves cache, coordination and rate limiting; Redis is deferred behind four named triggers | 2026-08-24 |
| [0007](0007-durable-execution-with-self-hosted-temporal.md) | Durable execution on self-hosted Temporal with its own PostgreSQL, and a data converter that enforces what a payload may carry | 2026-08-24 |
| [0008](0008-go-library-baseline-net-http-and-an-in-repo-migration-runner.md) | `net/http` routing in place of `chi`, and an in-repo checksummed migration runner in place of `goose`; `sqlc` recorded as an open deviation | 2026-08-24 |
| [0009](0009-dark-is-the-default-theme.md) | Dark is the default theme against the prototype's light, and the operating system's preference is deliberately not consulted | 2026-08-24 |

## Required initial ADRs

1. ~~Hosting platform and regional topology.~~ Accepted as [ADR-0001](0001-hosting-platform-and-regional-topology.md).
2. ~~Identity build/buy and enterprise federation.~~ Accepted as [ADR-0003](0003-identity-built-in-go.md).
3. ~~Go modular-monolith boundary and extraction criteria.~~ Accepted as [ADR-0005](0005-module-boundaries-and-extraction.md).
4. ~~Temporal hosting and workflow ownership.~~ Accepted as [ADR-0007](0007-durable-execution-with-self-hosted-temporal.md).
5. ~~PostgreSQL schemas, row-level security, and connection roles.~~ Accepted as [ADR-0002](0002-postgresql-schema-rls-and-connection-roles.md).
6. Realtime provider, media topology, and outage fallback.
7. Recording source, format, alignment, and retention.
8. ~~REST, gRPC, event, and generated-contract conventions.~~ Accepted as [ADR-0004](0004-contract-conventions-and-code-generation.md).
9. Artifact registry, review, publication, and rollback.
10. Model providers, routing, regional policy, fallback, and budgets.
11. Screening disclosure, appeals, and human-decision semantics.
12. Next.js deployment and strict Go backend boundary.

## ADR template

Copy [0000-template.md](0000-template.md). Number decisions sequentially. An ADR states context, decision, alternatives, consequences, risks, reversibility, owner, approval, and review date. Superseded ADRs remain in history and link to their replacement.

