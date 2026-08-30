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
| [0010](0010-sqlc-generates-the-repositories-sql-access.md) | sqlc generates every query from SQL beside each module, checked against the real migrations; what stays hand-written is only what cannot be generated | 2026-08-24 |
| [0011](0011-artifact-registry-review-publication-and-rollback.md) | Artifacts: git authors, the database registry publishes immutably by trigger, sessions pin digests, rollback is a pointer move, and the publisher must not be the drafter | 2026-08-26 |
| [0012](0012-livekit-carries-live-voice.md) | LiveKit self-hosted in eu-west-2 carries live voice behind our own Python agent; Go mints session-scoped room grants; degradation pauses and resumes, never continues degraded into evaluation | 2026-08-26 |
| [0013](0013-recording-capture-format-alignment-retention.md) | Recording is server-side SFU egress: two Opus/WebM tracks on the room's single clock; transcript-only sessions never write audio at all; durations stay with the retention decision | 2026-08-26 |
| [0014](0014-billing-unit-and-quota-behaviour.md) | A started session is the billing unit, metered exactly-once on the first in_progress transition; the limit blocks new starts after a soft warning and never touches an interview in flight | 2026-08-26 |
| [0015](0015-confidence-is-qualitative-evidence-sufficiency.md) | Confidence is a qualitative per-competency evidence-sufficiency label derived from stored counts and pinned in the rubric; no numeric display until QUA-03 calibrates; five prohibited interpretations become content rules | 2026-08-26 |
| [0016](0016-reconnect-pause-and-reinvitation-policy.md) | Timing policy v1: 120s reconnect grace, 300s overrun; practice may pause, screening may not; grace expiry finalizes as interrupted with evidence standing; re-invitation is recruiter-authorized and always a new session (screening clauses provisional pending DEC-11) | 2026-08-26 |
| [0017](0017-candidate-comparison-is-deferred.md) | Candidate comparison is explicitly deferred with three reopen triggers: published calibration, closed DEC-11 in every jurisdiction, and a concrete tenant ask | 2026-08-26 |
| [0019](0019-model-providers-routing-and-budgets.md) | Deepgram and Cartesia for speech; any language model per deployment and stage (Anthropic, OpenAI, Hugging Face, or local open weights) behind two adapters; cloud providers admissible only with zero-retention, no-training, UK/EU terms; fallback per stage only after measured equivalence; deterministic floors are the terminal fallback; exhaustion never degrades a required result silently | 2026-08-27 |
| [0018](0018-one-brand-with-stated-isolation.md) | One brand across practice and screening; the structural isolation guarantee becomes required candidate-facing copy wherever the modes meet, validated with candidates in PRC-06 | 2026-08-26 |
| [0020](0020-screening-disclosure-access-and-appeal.md) | Screening disclosure, candidate result access and appeal are a stored per-jurisdiction determination read at run time, pinned into a campaign like a rubric; a jurisdiction with no recorded determination cannot open a campaign at all; consent is unbundled and disclosure versions immutable in every jurisdiction; appeal defaults to a right. The legal determination itself stays open and the code enforces that it is open | 2026-08-30 |

## Required initial ADRs

1. ~~Hosting platform and regional topology.~~ Accepted as [ADR-0001](0001-hosting-platform-and-regional-topology.md).
2. ~~Identity build/buy and enterprise federation.~~ Accepted as [ADR-0003](0003-identity-built-in-go.md).
3. ~~Go modular-monolith boundary and extraction criteria.~~ Accepted as [ADR-0005](0005-module-boundaries-and-extraction.md).
4. ~~Temporal hosting and workflow ownership.~~ Accepted as [ADR-0007](0007-durable-execution-with-self-hosted-temporal.md).
5. ~~PostgreSQL schemas, row-level security, and connection roles.~~ Accepted as [ADR-0002](0002-postgresql-schema-rls-and-connection-roles.md).
6. ~~Realtime provider, media topology, and outage fallback.~~ Accepted as [ADR-0012](0012-livekit-carries-live-voice.md).
7. ~~Recording source, format, alignment, and retention.~~ Accepted as [ADR-0013](0013-recording-capture-format-alignment-retention.md).
8. ~~REST, gRPC, event, and generated-contract conventions.~~ Accepted as [ADR-0004](0004-contract-conventions-and-code-generation.md).
9. ~~Artifact registry, review, publication, and rollback.~~ Accepted as [ADR-0011](0011-artifact-registry-review-publication-and-rollback.md).
10. ~~Model providers, routing, regional policy, fallback, and budgets.~~ Accepted as [ADR-0019](0019-model-providers-routing-and-budgets.md).
11. Screening disclosure, appeals, and human-decision semantics. The mechanism is accepted as [ADR-0020](0020-screening-disclosure-access-and-appeal.md); the per-jurisdiction legal determination it reads remains open under DEC-11.
12. Next.js deployment and strict Go backend boundary.

## ADR template

Copy [0000-template.md](0000-template.md). Number decisions sequentially. An ADR states context, decision, alternatives, consequences, risks, reversibility, owner, approval, and review date. Superseded ADRs remain in history and link to their replacement.

