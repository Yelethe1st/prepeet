# Documentation Coverage Manifest

**Purpose:** Demonstrate how the consolidated greenfield specification was redistributed into `docs-new/` and identify artifacts that remain implementation or governance work.  
**Last audited:** 2026-08-23

## Audit rule

The split is considered complete when every material requirement, invariant, protocol, control, open decision, delivery gate, and operational expectation from the consolidated specification has a canonical home in `docs-new/`. Repeated explanations need not be duplicated merely to preserve word count.

## Source-to-target mapping

| Consolidated source subject | Canonical split target |
|---|---|
| Product definition, actors, principles, functional scope, success, open questions | `product/product-requirements.md` |
| Practice coaching, progression, articulation, privacy | `product/practice-mode.md` |
| Screening disclosure, review, decisions, appeals, safeguards | `product/screen-mode.md` |
| End-to-end candidate/recruiter/operator flows and state branches | `product/user-journeys.md` |
| Navigation, routes, page responsibilities, accessibility/content rules | `product/information-architecture.md` |
| Principal mandate, system context, deployables, frontend decision, tools, repo, quality/CI | `architecture/architecture-and-implementation-brief.md` |
| Bounded contexts, aggregates, invariants, ownership/transactions | `architecture/domain-model.md` |
| State transitions, completion, timing, mode lifecycle | `architecture/session-lifecycle.md` |
| WebRTC topology, epochs, events, reconnection, media, errors, latency/tests | `architecture/realtime-protocol.md` |
| PostgreSQL/S3/Redis, data products, ownership, read models, deletion | `architecture/data-architecture.md` |
| Roles/capabilities/scopes/matrix/elevation/services/enforcement/tests | `architecture/authorization-model.md` |
| Speech delivery dimensions, measures, result schema, coaching/drills/safeguards | `architecture/articulation-system.md` |
| Evidence pipeline, rubric/sufficiency/confidence, output/publication/evals/failures | `architecture/evaluation-system.md` |
| Browser REST/error/idempotency/pagination/route inventory | `contracts/public-api.md` |
| Go↔Python RPC envelopes, services, failures, governance | `contracts/internal-rpc.md` |
| Durable facts, envelope, ownership, compatibility | `contracts/event-catalog.md` |
| Signing, replay, retry, SSRF, lifecycle, compatibility | `contracts/webhook-protocol.md` |
| Trust boundaries, threats, abuse cases, controls, review cadence | `security/threat-model.md` |
| Restricted/confidential/internal/public handling and telemetry | `security/data-classification.md` |
| Data inventory, policy model, deletion, withdrawal, legal hold | `security/retention-and-deletion.md` |
| Human decisions, prohibited inference, comparison, fairness, appeals, launch gates | `security/responsible-hiring.md` |
| Managed-container/network/environment/release/scaling topology | `operations/deployment-topology.md` |
| Journey indicators, proposed targets, error budgets, measurement | `operations/service-level-objectives.md` |
| Telemetry safety, dashboards, alerts, quality monitoring | `operations/observability.md` |
| RPO/RTO, assets, recovery sequence, exercises, runbooks | `operations/disaster-recovery.md` |
| Demand, attribution, scenarios, controls, economics, scaling triggers | `operations/cost-and-capacity-model.md` |
| Phases, workstreams, first 90 days, ADRs, risks, exit criteria | `delivery/implementation-roadmap.md` |
| Critical path, work packages, parallel work, blockers | `delivery/dependency-map.md` |
| Foundation/practice/screen/operations gates and stop-ship | `delivery/release-criteria.md` |

## Preserved high-risk decisions

- Screening candidate disclosure/access and consent withdrawal.
- Bias-audit/demographic-data approach.
- Appeals rights and independence.
- Confidence meaning and score comparability.
- Historical re-evaluation after rubric change.
- Insufficient-evidence and billing policy.
- Screening reconnect/pause/restart/accommodation.
- Screening persona and supported language/accent policy.
- Evidence-first recruiter review and reviewer disagreement.
- Candidate comparison approval.
- Retention/legal-hold conflict.
- Billing unit/quota messaging.
- Shared practice/screen brand and review/results IA.
- Hosting, identity, Temporal, database, provider, recording, artifact, and contract decisions.

## Material boundaries preserved

- Go authoritative product state; Python typed intelligence; Next.js presentation/browser behavior.
- Immutable versioned session bundle.
- PostgreSQL source of truth and RLS defense in depth.
- Temporal durable orchestration and outbox external delivery.
- Browser-direct WebRTC and explicit reconnect/event protocol.
- Evidence-linked evaluation with insufficiency and publication validation.
- Articulation as practice coaching with deterministic measurement and no accent/personality inference.
- Candidate practice/employer screening isolation.
- Human-owned hiring decisions and append-only override/re-review history.
- Purpose-specific consent, retention, deletion, and privileged audit.
- SLO, recovery, cost, capacity, and release evidence requirements.

## Artifacts intentionally not fabricated by documentation

The following remain required work rather than omissions:

- accepted ADRs—the directory contains the backlog and template;
- generated OpenAPI and complete request/response examples;
- compiled Protobuf RPC/event schemas;
- physical PostgreSQL DDL, migrations, indexes, and query plans;
- approved legal retention/disclosure schedules;
- calibrated numeric confidence/quality thresholds;
- measured traffic, unit-cost, SLO, RPO, and RTO commitments;
- provider/cloud/vendor choices;
- final visual design tokens and production component implementation;
- production runbooks, dashboards, AI evaluation reports, and release records.

These are explicitly open implementation/governance deliverables and must not be silently guessed in an architecture brief.

