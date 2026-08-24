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

## Decided

Cloud, first region, managed container product, PostgreSQL topology and region-loss strategy are settled in [ADR-0001](../architecture/decisions/0001-hosting-platform-and-regional-topology.md): AWS, `eu-west-2` London, ECS on Fargate, multi-AZ RDS for PostgreSQL with PITR, and a single region.

Local development runs PostgreSQL, LocalStack and Temporal in containers. LocalStack provides S3, Secrets Manager and KMS, so adapters are written once against the AWS APIs and differ only by endpoint and credentials between local and deployed environments. LocalStack was chosen over MinIO by measurement; see [ADR-0001](../architecture/decisions/0001-hosting-platform-and-regional-topology.md).

### Redis is not provisioned

Its three proposed uses are each already served, so adding it would be a fourth stateful service to
provision, secure, monitor and fail over for no capability we lack.

Coordination is served by `FOR UPDATE SKIP LOCKED` on the outbox, which is better here than a lock
service rather than merely cheaper: the lock and the work live in one transactional scope, so they
cannot disagree, and there is no lease to tune or split-brain window.

Rate limiting is served by a counter in PostgreSQL. The cost is about a millisecond against the hundred
that argon2id already spends on the same request. It also removes a decision a separate store would
force: Redis can be down while the database is up, and somebody would then have to choose between
locking every user out and letting every attacker through. With one store there is no such state, since
authentication cannot happen without the database either.

Caching has nothing to cache. The one candidate is session lookup, and ADR-0003 requires that to be
measured before it is cached. It is also the cache with the worst failure mode, since a cached session
outliving a revocation is precisely what opaque tokens were chosen to prevent.

Redis becomes the right answer when limiting is needed on every request across the whole API at rates
where a database write per call matters, or when something needs coordination that is not about rows in
this database. Neither is close.

## Open decisions

Managed Temporal, observability vendor, and provider egress.

