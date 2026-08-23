# Data Architecture

**Status:** Proposed  
**Owner:** Principal Engineer and data/platform teams  
**Last updated:** 2026-08-23

## Storage strategy

- PostgreSQL is the system of record.
- S3-compatible object storage holds CVs, audio, exports, and large immutable artifacts.
- Redis is restricted to ephemeral cache, rate limiting, and coordination.
- PostgreSQL full-text and `pgvector` precede dedicated search/vector infrastructure.
- An analytics warehouse is introduced only after measured operational/reporting limits.

## Logical PostgreSQL ownership

```text
identity · tenancy · candidate · content · interview · media
evaluation · recruiting · progression · billing · integration · audit
```

One cluster does not imply shared writes. Each module owns its tables. Cross-module reporting uses governed projections.

## Core conventions

- UUIDv7 primary IDs; external secrets use independent high-entropy values.
- `tenant_id` and supporting indexes on every tenant-owned record.
- PostgreSQL row-level security as defense in depth with production-equivalent tests.
- UTC timestamps; integer milliseconds for durations; monetary minor units plus currency.
- Optimistic version on mutable aggregates.
- Append-only evaluation evidence, competency history, usage, review decisions, consent, and audit.
- JSONB for immutable snapshots/provider payloads/evolving supplementary output; typed columns for contractual/query-critical fields.
- Unique idempotency constraints for retryable mutations.
- Expand-and-contract migrations and regularly exercised point-in-time restore.

## Data products

### Session bundle

Immutable, content-addressed bundle containing tenant/mode policy, candidate/job snapshots, persona, plan, rules, rubric/calibration, standard, readiness, prompts, models, experiments, schemas, versions, and digests.

### Transcript

Ordered segments with speaker, turn, sequence, text, timing, confidence, language, provider event, supersession, audio reference, redaction, and assessability.

### Media manifest

Authoritative database record for track object keys, type, bytes, checksum, timing, upload state, encryption/region, retention, and deletion. Bucket listing is never an application query.

### Evaluation

Input digests, attempts, pipeline/artifact/prompt/model/calculator versions, evidence, turn/competency/articulation output, quality warnings, usage, latency, publication, and visibility.

### Usage and audit

Usage is attributable by tenant, session, mode, capability, provider/model, unit, and price version. Audit records actor, authority, purpose, resource, action, outcome, correlation, and tamper evidence.

## Data ownership

| Data | Writer | Readers |
|---|---|---|
| Identity/memberships | Go | Go; web through API |
| Session lifecycle | Go | Go, scoped Python inputs, web projection |
| Session bundle | Python composes; Go validates/persists | Go/Python/authorized review |
| Transcript | Go ingestion | Python and authorized review |
| Audio | Browser/provider upload; Go manifest | Python feature extraction, authorized playback |
| AI result | Python produces; Go validates/persists | Review/progression projections |
| Human decision | Go recruiting | Authorized tenant/integration |
| Progression | Go projection from practice evaluation | Candidate only by default |

## Read models

Candidate dashboard/readiness, session list/progress, practice results/review, recruiter roster/detail, invitation lifecycle, platform health/quality, tenant usage/quota, and integration deliveries.

Projections publish freshness. Commands consult authoritative aggregates.

## Object storage

- Tenant/region-prefixed opaque keys.
- Encryption, checksums, retention, lifecycle, and short-lived signed access.
- No public buckets.
- Upload and playback authorization is resource- and purpose-scoped.
- Database/object reconciliation jobs report orphans and missing objects.

## Redis constraints

Permitted: rate limiting, short-lived projections, realtime coordination, and carefully bounded locks. Redis failure cannot destroy interview, evaluation, consent, billing, or audit state.

## Retention and deletion

Purpose-specific retention metadata follows each primary and derived record. Deletion is a durable workflow spanning PostgreSQL, objects, caches, search/vector, analytics, providers, and backup expiry. Legal hold and required hiring-record retention override deletion only under approved policy and remain visible to the requester where permitted.

## Open decisions

- Physical schema and partition thresholds.
- Regional database topology and tenant movement.
- Identity storage when using an external provider.
- Legal-hold representation.
- Audit immutability mechanism.
- Warehouse/search adoption triggers.
- Final retention schedules.

