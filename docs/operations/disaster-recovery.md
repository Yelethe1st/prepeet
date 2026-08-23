# Disaster Recovery

**Status:** Proposed baseline  
**Owner:** Platform/SRE and security  
**Last updated:** 2026-08-23

## Objectives

Proposed starting database RPO: 15 minutes or better. Proposed service RTO: 4 hours. Business and enterprise commitments must ratify these values.

[ADR-0001](../architecture/decisions/0001-hosting-platform-and-regional-topology.md) runs a single region with point-in-time recovery, so a regional outage is an outage rather than a failover: RTO for region loss is measured in hours and must be stated to tenants rather than implied away. These values stay proposals until a restore has been performed and timed, per the validation in that ADR. Warm standby in a second region is the pre-agreed next step if a buyer will not accept the measured figure.

## Recovery assets

- Managed multi-zone PostgreSQL with PITR.
- Object versioning/replication consistent with residency.
- Temporal managed guarantees or backed-up persistence.
- Infrastructure/config/contracts/artifacts recoverable from version control and registries.
- Secret/KMS emergency rotation and recovery.
- Provider configuration and webhook destinations backed up without exposing secrets.

## Scenarios

Database corruption/unavailability, region loss, object loss, Temporal outage/history loss, bad migration/deploy, bad artifact/model publication, key compromise, provider outage, webhook backlog, and evaluation-integrity incident.

## Recovery sequence

1. Declare incident and freeze unsafe writes/publications.
2. Establish recovery point and data-residency constraints.
3. Restore infrastructure/state into isolated validation environment.
4. Validate schemas, tenant isolation, session/evaluation references, bundle/artifact digests, object manifests, outbox cursors, workflow idempotency, and audit continuity.
5. Reconcile duplicate/missing deliveries and incomplete workflows.
6. Resume progressively and monitor journey SLOs.
7. Communicate and complete post-incident evidence.

## Exercises

- Quarterly PostgreSQL restore.
- Periodic full journey restore/recovery.
- Worker/Temporal replay and duplicate-delivery drill.
- Key rotation/compromise drill.
- Provider outage/fallback or degraded-mode drill.
- Bad artifact publication freeze/rollback and impacted-session query.

## Runbooks required

Database failover/restore, region outage, stuck/replayed workflow, interrupted session, partial media, provider outage, key rotation, leaked token/media, bad artifact/model, deletion failure, cross-tenant incident, and evaluation-integrity re-review.

## Open decisions

Final RPO/RTO, regional topology, object replication, Temporal recovery model, backup retention/encryption, enterprise communication, and provider exit/fallback.

