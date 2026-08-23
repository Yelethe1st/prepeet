# Deployment Topology

**Status:** Proposed  
**Owner:** Platform engineering  
**Last updated:** 2026-08-23

## Initial topology

Use a managed container platform unless discovery proves Kubernetes is necessary.

```text
Public edge / WAF
├── Next.js web
└── Go API

Private network
├── Go worker
├── Python intelligence API/worker
├── Temporal workers
└── integration dispatcher

Managed state/services
├── PostgreSQL with PITR
├── Temporal service
├── Redis
├── S3-compatible object storage
├── secrets/KMS
└── metrics/logs/traces/error reporting
```

The browser connects to the realtime provider directly with ephemeral Go-issued authorization.

## Environments

Production, staging, and development use separate accounts/projects, networks, databases, buckets, credentials, and provider keys. Production data never seeds lower environments. Ephemeral preview environments use synthetic data.

## Network and identity

- Public exposure only at edge/web/API and explicitly approved webhook egress.
- Databases, Redis, workers, and Temporal private.
- Workload identity and short-lived credentials.
- Restricted egress for Python/model calls and webhook dispatcher.
- Region selection follows tenant residency and provider availability.

## Release

- Terraform-managed infrastructure.
- Immutable images promoted by digest; provenance/signing where supported.
- Expand-and-contract database migrations.
- Artifact/prompt publication independently versioned and reversible.
- Feature flags separate deploy from exposure and include mode/tenant scope.
- Progressive rollout: synthetic/internal → practice → controlled screening.
- Maintain application/database compatibility windows for rollback.

## Scaling

Scale independently: web, Go API, Go workers, Python realtime/evaluation workers, integration dispatcher. Database connection pools and provider concurrency are explicit bottlenecks. Introduce new services only for independent scaling, ownership, security, availability, or release needs.

## Kubernetes triggers

Consider only when service count/traffic, specialized scheduling, multi-region control, policy, or platform-team maturity make managed containers materially inadequate. Record through ADR with migration/operating cost.

## Open decisions

Cloud and regions, managed container product, managed Temporal, PostgreSQL topology, Redis need, observability vendor, provider egress, and region-loss strategy.

