# Event Catalog

**Status:** Implemented  
**Owner:** Go platform and integration architecture  
**Last updated:** 2026-08-24

The machine-readable contract is `packages/contracts/events/`, per
[ADR-0004](../architecture/decisions/0004-contract-conventions-and-code-generation.md).
This document is the reasoning and the producer/consumer map; a test fails if
the table below and the schemas disagree in either direction, so neither can be
edited alone. Delivered by CTR-03.

## Principles

- Events describe completed facts in past tense.
- External delivery is at-least-once and may be out of order.
- Producer writes event through a PostgreSQL transactional outbox in the same transaction as authoritative state.
- Consumers deduplicate by event ID and are idempotent.
- PII is minimized; payload is not a database row dump.

## Envelope

```json
{
  "event_id": "evt_uuidv7",
  "event_type": "evaluation.completed.v1",
  "schema_version": "1.0",
  "tenant_id": "tenant_uuidv7",
  "occurred_at": "2026-08-23T12:34:56Z",
  "producer": "evaluation",
  "actor": {"type": "service", "id": "evaluation-worker"},
  "purpose": "screening",
  "correlation_id": "...",
  "causation_id": "...",
  "payload": {}
}
```

## Initial events

| Event | Producer | Consumers |
|---|---|---|
| `identity.user_registered.v1` | Identity | Notification, analytics |
| `tenant.membership_changed.v1` | Tenancy | Audit, session invalidation |
| `candidate.document_uploaded.v1` | Candidate | Extraction workflow |
| `interview.session_created.v1` | Interview | Composition, usage |
| `interview.session_ready.v1` | Interview | Notification, projection |
| `interview.session_started.v1` | Interview | Usage, operations |
| `interview.session_interrupted.v1` | Interview | Operations/incident policy |
| `interview.session_completed.v1` | Interview | Finalization/evaluation |
| `media.manifest_finalized.v1` | Media | Evaluation/articulation |
| `evaluation.requested.v1` | Evaluation | Evaluation workflow |
| `evaluation.completed.v1` | Evaluation | Review, progression, notification |
| `evaluation.insufficient_evidence.v1` | Evaluation | Review, quality monitoring |
| `evaluation.failed.v1` | Evaluation | Operations/notification policy |
| `review.decision_recorded.v1` | Recruiting | Audit, integration |
| `appeal.requested.v1` | Recruiting | Assignment/notification |
| `candidate.competency_observed.v1` | Evaluation | Practice progression |
| `integration.delivery_requested.v1` | Integration | Dispatcher |
| `privacy.deletion_requested.v1` | Privacy | Deletion workflow |

## Ownership

Only the context owning the authoritative state emits its event. Consumers do not infer authoritative lifecycle from telemetry. Temporal internal workflow history is not the external event bus or audit log.

## Enforcement

The envelope and the payload are checked at publication rather than trusted. The
outbox refuses an event type that is not in the catalogue, a producer that is not
the owning context, a payload missing a declared field, and a payload carrying a
field nobody declared. `make check-events` refuses a change that would break a
consumer built against the previous release.

A consumer is asymmetric with a producer on purpose: it tolerates a field it does
not know, because refusing one is how an additive change becomes a breaking one.

## Compatibility and retention

Additive schema evolution within version; new version for semantic break. Consumers tolerate unknown additive fields/enums. Retention covers replay/support commitments and privacy minimization. Contract fixtures verify every producer/consumer pair.

