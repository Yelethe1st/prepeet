# Architecture Decisions

This directory contains approved Architecture Decision Records (ADRs). Proposed architecture elsewhere in `docs/` does not substitute for a decision where alternatives materially affect cost, risk, security, or reversibility.

## Required initial ADRs

1. Hosting platform and regional topology.
2. Identity build/buy and enterprise federation.
3. Go modular-monolith boundary and extraction criteria.
4. Temporal hosting and workflow ownership.
5. PostgreSQL schemas, row-level security, and connection roles.
6. Realtime provider, media topology, and outage fallback.
7. Recording source, format, alignment, and retention.
8. REST, gRPC, event, and generated-contract conventions.
9. Artifact registry, review, publication, and rollback.
10. Model providers, routing, regional policy, fallback, and budgets.
11. Screening disclosure, appeals, and human-decision semantics.
12. Next.js deployment and strict Go backend boundary.

## ADR template

Copy [0000-template.md](0000-template.md). Number decisions sequentially. An ADR states context, decision, alternatives, consequences, risks, reversibility, owner, approval, and review date. Superseded ADRs remain in history and link to their replacement.

