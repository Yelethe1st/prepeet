# ADR-0007: Durable execution runs on self-hosted Temporal, on its own database

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-24  
**Review date:** 2027-02-24  
**Supersedes:** None  
**Superseded by:** None

Closes [DEC-04](../../delivery/tickets/01-decisions-and-adrs.md) and unblocks
[PLT-06](../../delivery/tickets/02-platform-foundation.md).

## Context

Four things in this system are long-running, must survive a process restart, and must not repeat their
side effects when they resume: session composition, evaluation, deletion, and media finalization.
[session-lifecycle.md](../session-lifecycle.md) makes completion an eight-step sequence that seals a
transcript, waits a bounded period for media, persists digests, runs evaluation and only then publishes.
[retention-and-deletion.md](../../security/retention-and-deletion.md) requires deletion to be "a durable,
observable workflow" with per-system status.

None of that is a request. A candidate finishes an interview and closes the tab; the work continues for
minutes across several systems, some of which will fail and be retried.

That Temporal is the right tool for this is not what is being decided here. Writing the equivalent by
hand means building timers, retries, idempotency, versioning and visibility, and the result is a worse
Temporal owned by us. What is open is where it runs, what it stores its state in, and what may be put
into it.

## Decision

**Self-hosted Temporal, on ECS Fargate in `eu-west-2`, on a database instance of its own.**

### Self-hosted rather than Temporal Cloud

Temporal Cloud is the easier operational answer and was rejected on residency rather than on cost.

Workflow history is not metadata. It durably records every workflow input, every activity input and
every activity result, which for this system means session identifiers, tenant identifiers and
evaluation references. [ADR-0001](0001-hosting-platform-and-regional-topology.md) commits to storage in
`eu-west-2` London. Temporal Cloud's region list would have to be checked against that, and if it does
not include London then choosing it means workflow state leaves the UK and ADR-0001 needs amending
rather than quietly contradicting itself.

It would also become a sub-processor holding candidate-linked state, which means an entry in the data
inventory, the retention schedule, and the disclosure candidates are shown before they consent. That
machinery exists, because ADR-0001 already uses it for the AI providers. The point is that using it is a
decision to be made deliberately, and there is no forcing reason to make it now.

The cost is stated plainly rather than minimised: this is a stateful service to provision, secure, patch
and upgrade, and Temporal upgrades are real work rather than background work.

**The swap is kept cheap deliberately.** Self-hosted and Cloud differ in address, namespace and
credentials, and nothing else, so all three live in configuration and the client is built in one place.
Moving is a configuration change plus the paperwork above, not a rewrite. This is the same shape as
`ratelimit.Counter` and `broadcast.Broadcaster`, and for the same reason: a deferral is only defensible
while it stays reversible.

### Its own database instance, not the product primary

Temporal supports PostgreSQL for persistence, and the tempting economy is to point it at the instance
already running. That is rejected, and the strongest reason is the one this project has already written
down about something else.

[ADR-0006](0006-postgresql-serves-cache-coordination-and-rate-limiting.md) argued that many small writes
to few rows is the pattern PostgreSQL handles worst, and that moving high-frequency, low-value writes off
the primary protects the store that actually matters. Temporal is exactly that pattern and at far higher
volume than the rate limiter that argument was about: every workflow task, activity start, activity
completion, timer and heartbeat is a write. Applying that reasoning to the rate limiter and not to
Temporal would be inconsistent.

Three more, each independently sufficient:

**The role model does not fit.** [ADR-0002](0002-postgresql-schema-rls-and-connection-roles.md) forces
row-level security on every table and gives the application a `NOSUPERUSER NOBYPASSRLS` role. Temporal
manages its own schema with its own migration tool and expects ordinary ownership of its tables. Putting
it in that instance means either exempting its schemas from the RLS guard, which weakens a check that has
already caught a real omission, or fighting its tooling.

**Upgrades couple.** Temporal pins schema versions to server versions and migrates on upgrade. Sharing an
instance means its upgrade window is our upgrade window.

**Blast radius.** Workflow persistence bloating or contending should not slow request serving, and a
product incident should not take workflow execution with it.

Locally this is a second container. Deployed it is a second RDS instance, smaller than the primary.

### Namespaces separate environments

One namespace per environment: `prepeet-local`, `prepeet-preview`, `prepeet-staging`,
`prepeet-production`. A worker connects to exactly one and a namespace is never shared, so a preview
worker cannot pick up a production task by having the wrong task queue name, which is otherwise a
one-typo mistake with an unpleasant blast radius.

### Workflow history retention

**Thirty days in production, seven elsewhere**, set per namespace.

Long enough that an incident review can replay what actually happened, which is most of the operational
value. Short enough that history is not a second, undocumented copy of candidate-linked data outliving
the retention schedule it is supposed to obey.

This interacts with deletion and is called out in [retention-and-deletion.md](../../security/retention-and-deletion.md):
a deletion request must not leave the deleted subject's identifiers sitting in workflow history for
another month. The payload rule below is what keeps that tractable, because identifiers in history
resolve to records that deletion has already removed.

### What may be put into a workflow

**Identifiers and small control values. Never transcript text, evaluation prose, CV content, model
output or candidate contact details.**

This is the same rule as telemetry in [telemetry-conventions.md](../../operations/telemetry-conventions.md)
and for the same reason: workflow history is durable storage on its own retention schedule, outside the
deletion machinery that governs the tables. An activity taking a transcript as an argument silently
creates a second copy of it, in a store nobody classified.

Activities take identifiers and read what they need from the database, under the row-level security that
decides whether they may.

**Enforced by a data converter, not by review.** The converter sits on the payload path in every worker
and client, so a payload that breaks the rule cannot be encoded. A convention here would fail the same
way it would in telemetry, and the failure would be discovered in a store with a month's retention.

### Workflow versioning

**In-flight changes use `workflow.GetVersion`; incompatible changes get a new workflow type.**

Decided now rather than at the first change, because the first change is exactly when the pressure to
just edit the workflow is highest and the consequence, a non-deterministic replay of every in-flight
execution, is invisible until it happens.

A workflow whose shape changes enough to need several stacked version gates is a workflow to replace
rather than patch. Version gates are removable once no execution older than the retention window can
still be running, which is one of the things the retention decision above is doing.

### Ownership

One on-call rotation covering the platform, including Temporal, because there is one team. Recorded so
that the question is answered rather than assumed, and so that splitting it later is a change to this
document.

## Alternatives considered

**Temporal Cloud.** Above. The residency question is the blocker, not the price.

**A workflow-shaped thing on the outbox and a state machine.** The outbox already gives durable
at-least-once delivery, and a state column would give sequencing. Rejected: it does not give timers,
bounded waits, retries with per-step policy, replay-safe branching, or a visibility surface, and
[session-lifecycle.md](../session-lifecycle.md) needs all of them. Building them is building Temporal
badly.

**Sharing the product database.** Above. Cheapest now, and it contradicts reasoning this project has
already committed to.

**Deferring Temporal and building the seam only.** Rejected on the stated engineering position: this is
needed with certainty, so under-building it for cost is the thing to avoid rather than the thing to do.

## Consequences

Positive. Residency in ADR-0001 stands unchanged. Nothing is added to the data inventory, the retention
schedule or the sub-processor disclosure. Workflow write volume never touches the product primary. The
RLS guard stays as strict as it is.

Negative, stated plainly. A fourth stateful service, with its own upgrade and patch cadence and its own
backup story. A second database instance to run and pay for. Temporal's own observability is a thing to
learn and wire up. And self-hosting means a Temporal outage is ours to resolve at whatever hour it
happens.

Security. Workflow history is candidate-linked storage, so it is in scope for the threat model and for
deletion. The payload rule and the retention window are what keep that bounded, and both are enforced
rather than documented.

Disaster recovery. [disaster-recovery.md](../../operations/disaster-recovery.md) lists "Temporal
outage/history loss" as a scenario and leaves the recovery model open. Self-hosting settles it as
"backed-up persistence" rather than "managed guarantees": Temporal's database gets the same
point-in-time recovery treatment as the product database, and the replay-and-duplicate-delivery drill
already listed becomes runnable. Product state and workflow state can be restored to different points,
so the drill must cover that, which is precisely why workflows must be idempotent against product state
rather than assuming it.

## Reversibility and migration

**To Temporal Cloud: configuration, credentials and paperwork.** Address, namespace and TLS material are
configuration; the client is constructed in one place; workflow and activity code is unchanged, because
it is the same Temporal. What is not cheap is the paperwork, which is the point of doing it deliberately:
data inventory, retention schedule, sub-processor disclosure, and an amendment to ADR-0001 if the region
does not include London. In-flight executions do not migrate, so the cutover drains rather than moves
them.

**Away from Temporal entirely: expensive, and correctly so.** Workflow code is written against the
Temporal SDK and no interface hides that honestly. An abstraction over durable execution would be a worse
Temporal with a smaller test suite. The seam that exists is the one that pays for itself.

[ADR-0005](0005-module-boundaries-and-extraction.md) provides the structural insurance: the Temporal
*client* lives in `platform/` and is reached through a consumer-defined interface, so a bounded context
starts a workflow without importing the SDK. Workflow *definitions* legitimately import the SDK, since
that is what a workflow is, and the boundary test distinguishes the two rather than banning both.

## Validation

- A worker killed mid-workflow replays without duplicating state, usage or notification.
- The data converter refuses a payload carrying restricted content, asserted by a test that tries.
- A workflow started against a preview namespace is invisible to a production worker.
- History retention is set per namespace and verified, not assumed from a default.
- A version gate is exercised by a test that replays a history recorded before the change.
- Temporal's database is included in the backup and restore drill, and the drill covers product state and
  workflow state restored to different points.
