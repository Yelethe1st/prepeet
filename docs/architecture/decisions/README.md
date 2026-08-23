# Architecture Decisions

This directory contains approved Architecture Decision Records (ADRs). Proposed architecture elsewhere in `docs/` does not substitute for a decision where alternatives materially affect cost, risk, security, or reversibility.

## Accepted

| ADR | Decision | Date |
|---|---|---|
| [0001](0001-hosting-platform-and-regional-topology.md) | Hosting platform and regional topology: AWS, `eu-west-2` London, in-region storage with disclosed AI sub-processors, single region with PITR | 2026-08-23 |
| [0002](0002-postgresql-schema-rls-and-connection-roles.md) | PostgreSQL: one database, a schema per module, forced row-level security on every tenant-owned table, tenant context set per transaction | 2026-08-24 |
| [0003](0003-identity-built-in-go.md) | Authentication built in Go with argon2id and revocable opaque sessions; enterprise federation deferred behind an adapter | 2026-08-24 |

## Required initial ADRs

1. ~~Hosting platform and regional topology.~~ Accepted as [ADR-0001](0001-hosting-platform-and-regional-topology.md).
2. ~~Identity build/buy and enterprise federation.~~ Accepted as [ADR-0003](0003-identity-built-in-go.md).
3. Go modular-monolith boundary and extraction criteria.
4. Temporal hosting and workflow ownership.
5. ~~PostgreSQL schemas, row-level security, and connection roles.~~ Accepted as [ADR-0002](0002-postgresql-schema-rls-and-connection-roles.md).
6. Realtime provider, media topology, and outage fallback.
7. Recording source, format, alignment, and retention.
8. REST, gRPC, event, and generated-contract conventions.
9. Artifact registry, review, publication, and rollback.
10. Model providers, routing, regional policy, fallback, and budgets.
11. Screening disclosure, appeals, and human-decision semantics.
12. Next.js deployment and strict Go backend boundary.

## ADR template

Copy [0000-template.md](0000-template.md). Number decisions sequentially. An ADR states context, decision, alternatives, consequences, risks, reversibility, owner, approval, and review date. Superseded ADRs remain in history and link to their replacement.

