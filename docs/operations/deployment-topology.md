# Deployment Topology

**Status:** Proposed  
**Owner:** Platform engineering  
**Last updated:** 2026-08-30

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

### The control plane to intelligence plane hop

This connection carries interview briefs outward and transcripts back, so it
carries candidate speech and is in scope for the data classification. It ran as
plaintext gRPC in both directions, with a source comment asserting that deployed
environments got TLS; no code on either end offered a way to do so.

Outside `local` and `preview`, a process configured with
`PREPEET_INTELLIGENCE_ADDRESS` now refuses to start unless the transport is
settled one way or the other:

| Variable | Meaning |
| --- | --- |
| `PREPEET_INTELLIGENCE_TLS_CA_FILE` | Authority the intelligence plane's certificate is verified against. Set this for an internal endpoint; leaving it empty verifies against the public roots, which an internal name will not satisfy. |
| `PREPEET_INTELLIGENCE_TLS_CERT_FILE`, `PREPEET_INTELLIGENCE_TLS_KEY_FILE` | The worker's own certificate, when the intelligence plane requires one. Set together or not at all. |
| `PREPEET_INTELLIGENCE_TLS_INSECURE` | `true` declares plaintext deliberately, for a mesh that encrypts the hop in a sidecar. It is a declaration rather than a default so that forgetting to configure the transport is not the same as choosing to go without it. |

The serving half reads `PREPEET_RPC_TLS_CERT_FILE` and
`PREPEET_RPC_TLS_KEY_FILE`; adding `PREPEET_RPC_TLS_CLIENT_CA_FILE` requires a
client certificate rather than merely accepting one. The server logs which of
plaintext, TLS, or mutual TLS it bound, because the failure worth catching is a
deployment that intended to be encrypted and silently was not.

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

## Decided

Cloud, first region, managed container product, PostgreSQL topology and region-loss strategy are settled in [ADR-0001](../architecture/decisions/0001-hosting-platform-and-regional-topology.md): AWS, `eu-west-2` London, ECS on Fargate, multi-AZ RDS for PostgreSQL with PITR, and a single region.

Local development runs PostgreSQL, LocalStack and Temporal in containers. LocalStack provides S3, Secrets Manager and KMS, so adapters are written once against the AWS APIs and differ only by endpoint and credentials between local and deployed environments. LocalStack was chosen over MinIO by measurement; see [ADR-0001](../architecture/decisions/0001-hosting-platform-and-regional-topology.md).

### Redis is not provisioned

Decided in [ADR-0006](../architecture/decisions/0006-postgresql-serves-cache-coordination-and-rate-limiting.md).
Coordination is served by `FOR UPDATE SKIP LOCKED` on the outbox, rate limiting by a counter in
PostgreSQL, and there is nothing yet to cache. Two of those three are better answers rather than
cheaper ones.

That ADR also records what Redis would genuinely bring, the four triggers that would make it necessary
with what to measure for each, and what integrating it would actually cost. The most likely first
trigger is fanning live interview progress out across instances, which is not one of the three uses
Redis was originally proposed for.

## Open decisions

Managed Temporal, observability vendor, and provider egress.

