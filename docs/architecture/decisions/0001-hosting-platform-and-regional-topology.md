# ADR-0001: Hosting platform and regional topology

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-23  
**Review date:** 2027-02-23  
**Supersedes:** None  
**Superseded by:** None

Implements [DEC-01](../../delivery/tickets/01-decisions-and-adrs.md).

## Context

Prepeet stores voice recordings, transcripts and evaluations of named people taking part in hiring
processes. Under [data-classification.md](../../security/data-classification.md) that content is
Restricted, and under [responsible-hiring.md](../../security/responsible-hiring.md) it can affect
whether someone gets a job. Where it is processed is therefore a legal question before it is an
engineering one.

Three constraints shape the answer.

The product is en-GB first, and the worked example throughout the specification and the prototype is a
UK healthcare provider. UK health buyers commonly require UK data residency, and discovering that after
building in another region is a migration rather than a configuration change.

The realtime voice and model providers this product depends on may not offer processing in every region
we would like. A residency promise that the provider layer cannot keep is worse than a narrower promise
kept honestly, because the promise ends up in candidate-facing disclosure under
[DEC-11](../../delivery/tickets/01-decisions-and-adrs.md).

There are no paying tenants yet. Infrastructure that assumes enterprise availability before a pilot has
run spends money and operational attention that the evaluation quality work needs more.

## Decision

**Platform: AWS**, using managed containers rather than Kubernetes. ECS on Fargate for the web, API,
worker and Python services. Kubernetes remains out of scope until the triggers in
[deployment-topology.md](../../operations/deployment-topology.md) are met and a further ADR records the
migration and operating cost.

**First region: `eu-west-2`, London.** All production compute, PostgreSQL, object storage, backups and
telemetry for the first deployment live there.

**Expansion path:** a second region is added as a peer, not as a shard of the first. EU, most likely
`eu-central-1`, is the expected second region and is added when a tenant requires EU residency rather
than in anticipation. US expansion is a separate decision because it changes the legal work rather than
the topology: state level AI hiring rules replace GDPR as the governing regime and
[DEC-11](../../delivery/tickets/01-decisions-and-adrs.md) is re-answered for it.

**Residency commitment:** candidate recordings, transcripts, evaluations, derived artifacts, database
contents and backups are stored and processed in the tenant's region and do not leave it. Model,
transcription and realtime providers are named sub-processors and may process content in another
region under contract. This distinction is stated in plain language in candidate disclosure rather than
buried in a sub-processor annex.

**Region loss: single region with point-in-time recovery.** Multi-zone managed PostgreSQL with PITR,
versioned object storage, and a restore procedure rehearsed under
[REL-05](../../delivery/tickets/22-release-readiness.md). The recovery objectives in
[disaster-recovery.md](../../operations/disaster-recovery.md) stand as proposals until measured by a
real restore.

**Local development: containerised, with MinIO standing in for S3.** The local stack runs PostgreSQL,
MinIO and Temporal in containers so an engineer needs no cloud account to run the product. MinIO is
S3-compatible, so the object storage adapter in `services/platform/platform/objectstore` is written
once against the S3 API and differs only by endpoint and credentials between local and deployed
environments. Local buckets hold synthetic data only, per
[deployment-topology.md](../../operations/deployment-topology.md).

Concrete service choices that follow, each reversible without revisiting this ADR:

| Concern | Choice | Note |
|---|---|---|
| Compute | ECS on Fargate | App Runner is simpler but gives less control over networking and private egress |
| Database | RDS for PostgreSQL, multi-AZ, PITR | Aurora is reconsidered when connection or throughput limits are measured, not before |
| Object storage | S3 in `eu-west-2` | MinIO locally |
| Secrets | AWS Secrets Manager with KMS | Feeds [PLT-07](../../delivery/tickets/02-platform-foundation.md) |
| Edge | CloudFront with WAF | Public exposure only at edge, web and API |
| Temporal | Deferred to [DEC-04](../../delivery/tickets/01-decisions-and-adrs.md) | Region availability is now a constraint on that decision |
| Observability | Deferred | Vendor choice must satisfy the restricted-content rules in [SEC-08](../../delivery/tickets/19-security-and-privacy.md) |

## Alternatives considered

**GCP with Cloud Run.** Better managed container ergonomics and a simpler managed PostgreSQL. Rejected
because UK region coverage and buyer familiarity are weaker, and more vendors default to AWS for data
processing agreements.

**Azure.** The strongest fit if NHS or UK public sector procurement is the first route to market, where
existing agreements and assurance paths carry real weight. Rejected for now on developer ergonomics and
weaker Temporal and AI provider support. This is the alternative most likely to be revisited, and the
trigger is a public sector buyer requiring it.

**Managed PaaS such as Fly.io with Neon.** Fastest and cheapest route to a running pilot. Rejected
because residency guarantees and audit evidence are weaker, and both are required at the screening
release gate in [REL-03](../../delivery/tickets/22-release-readiness.md). Choosing it would mean
migrating precisely when legal review is under way.

**Hard residency including all AI processing.** Nothing leaves the region under any circumstance.
Rejected because it eliminates most realtime voice and model providers or forces self-hosted models,
which changes the cost model and lowers the quality ceiling for a product whose value depends on
evaluation quality. It remains available as a per-tenant offering if a buyer requires it and will pay
for it.

**Multi-region from day one.** Rejected as complexity bought before it is needed. Tenant routing,
split-brain handling and cross-region consistency are real work for a system whose central promise is
reproducible pinned state.

## Consequences

Positive. Residency is decidable per tenant from the first line of code, because region is a property of
the tenant rather than an afterthought. The local stack needs no cloud account, so
[PLT-01](../../delivery/tickets/02-platform-foundation.md)'s single-command start stays true. The
narrower residency promise is one the provider layer can actually keep.

Negative. AWS carries more operational surface than a PaaS, and ECS task definitions and networking are
work that a smaller platform would not require. A single region means a regional outage is an outage:
RTO is measured in hours, and that has to be stated to tenants rather than implied away.

Security. Restricted content stays in region at rest. The exposure that remains is provider egress,
which is why [DEC-06](../../delivery/tickets/01-decisions-and-adrs.md) and
[DEC-10](../../delivery/tickets/01-decisions-and-adrs.md) must record each provider's processing region,
and why egress from the Python service and the webhook dispatcher is restricted by default.

Cost. Baseline is one region of managed services rather than two. The demand inputs in
[cost-and-capacity-model.md](../../operations/cost-and-capacity-model.md) are now priced against
`eu-west-2`, which is roughly 5 to 10 percent above `us-east-1` for equivalent resources. Model and
realtime provider spend is expected to dominate infrastructure spend, so this premium is not the number
to optimise first.

Organisational. One region and one cloud means one set of runbooks and one on-call surface, which suits
a team that does not yet have a platform specialism.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| No realtime or model provider offers UK processing | Recorded fallback below. Decided in DEC-06 and DEC-10 with the residency wording adjusted before launch, never after |
| A UK health buyer requires that no data reaches a non-UK sub-processor | Per-tenant hard-residency offering, priced separately, with a reduced capability set stated up front |
| Regional outage during a live screening interview | Interruption is recorded as coverage rather than as poor performance, per [SES-06](../../delivery/tickets/08-session-lifecycle.md), and re-invitation is a human decision under [SCR-08](../../delivery/tickets/14-screening-and-invitations.md) |
| Single region RTO proves unacceptable to a buyer | Warm standby in the second region is the pre-agreed next step, costed before it is needed |
| Local and deployed object storage behaviour diverges | The adapter is written against the S3 API only, and integration tests run against MinIO in CI so divergence surfaces there |

## Fallback scope if the region cannot host a required provider

Required by DEC-01. If a provider needed for realtime voice, transcription or evaluation has no UK
processing option, the order of preference is:

1. **Provider with EU processing, disclosed.** Content remains in the EEA. Disclosure names the
   provider and the region. This is expected to be the common case and needs no change to the storage
   commitment.
2. **Provider with non-EEA processing, disclosed, with transfer safeguards.** Requires an approved
   transfer mechanism and explicit legal review before a single real candidate is affected. Practice
   mode may adopt this ahead of screening because the data subject is the candidate themselves and the
   consent is theirs to give.
3. **Reduced capability in region.** Ship the capability without the provider rather than move the
   data. An articulation measure that cannot be computed in region is reported as unassessable, which
   the product already handles honestly, rather than being computed elsewhere quietly.

What is not available as a fallback: moving candidate content to another region without disclosure, or
treating a provider's default region as acceptable because it is convenient.

## Reversibility and migration

Region is the expensive part to reverse. Moving regions means migrating PostgreSQL, re-replicating
object storage, re-issuing provider configuration and re-papering residency commitments with every
tenant. Cost is measured in weeks, and it grows with stored recording volume.

Cloud is moderately expensive to reverse. Terraform, container images and the three deployables are
portable in principle; managed PostgreSQL, secrets and edge configuration are not, and observability
integration would be rebuilt.

The container and adapter boundaries are deliberately kept portable to keep this cost bounded: nothing
outside `services/platform/platform/` may call an AWS SDK directly.

## Validation

- Every tenant record carries a region, and no code path infers region from anything else.
- Integration tests run against MinIO, and the object storage adapter contains no MinIO-specific branch.
- A restore from PITR into an isolated environment is performed and timed before the practice release
  gate, and the measured RPO and RTO replace the proposed values in
  [disaster-recovery.md](../../operations/disaster-recovery.md).
- A dependency audit lists every sub-processor with its processing region, and it is reviewed whenever a
  provider is added or changed.
- No AWS SDK import appears outside `services/platform/platform/`, enforced by the module boundary check
  in [PLT-04](../../delivery/tickets/02-platform-foundation.md).
