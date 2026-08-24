# Epic PLT — Platform foundation

**Phase 1** · **Workstream** Platform

The repository, the three deployables, the database, the workflow engine, storage, secrets and
telemetry. Phase 1 exits when an authenticated request can be traced end to end and a durable
Go→Python workflow can be deployed, rolled back and recovered.

---

### PLT-01 · Stand up the monorepo with Next.js, Go and Python deployables

**Depends on** DEC-01, DEC-03 · **Blocks** everything

One repository, three deployables, shared contract definitions, per-language toolchains, and a task
runner that works the same on a laptop and in CI.

**Done when**
- [x] `web`, `api` and `intelligence` build reproducibly from a clean checkout.
- [x] Formatting, linting and type checking run identically locally and in CI.
- [x] A new engineer can run the whole stack locally from a documented single command.

**Ticked late, and two of the three were not true when they were written.**

`make lint` had never passed. The web package's lint script ran `next lint`, which Next 16 removed, and
no ESLint configuration existed anywhere in the repository. Nothing reported it because CI reimplemented
the checks rather than invoking the make targets, and its version of the web job omitted lint entirely.
Two copies of a check are two chances for one to stop running, and the one that stops is the one nobody
notices. CI now runs `make lint-go`, `make lint-py`, `make lint-web` and `make lint-contracts`, which is
what "identically" has to mean to be checkable.

`make dev` now starts the infrastructure, applies migrations and runs all three deployables together,
stopping them together on Ctrl-C. Before this there was no single command: `local-up` started the
containers and the deployables were three more commands. The connection strings read the same
`infrastructure/local/.env` the compose file does, so moving a port moves it for both rather than leaving
the application dialling a port nothing published.

Verified by running it: api and web answer on 8080 and 3000, the worker connects to Temporal, an
organisation registers and `/me` reports its workspace, and Ctrl-C leaves no process holding a port.

**Spec** [architecture-and-implementation-brief.md](../../architecture/architecture-and-implementation-brief.md)

---

### PLT-02 · Build the CI pipeline with contract, boundary and security gates

**Depends on** PLT-01 · **Blocks** CTR-04, REL-01

CI is where the invariants become enforceable. Build, test, contract lint, drift check, module-boundary
check, dependency audit and container scan, on every change.

**Done when**
- [ ] Pipeline fails on contract drift, forbidden imports, or a failing migration.
- [ ] Build artifacts are immutable and digest-addressed.
- [ ] Pipeline duration is fast enough that engineers do not route around it.

**Spec** [release-criteria.md](../release-criteria.md)

---

### PLT-03 · Provision PostgreSQL with row-level security and least-privilege roles

**Depends on** DEC-05 · **Blocks** IAM-03, SEC-02 · **In progress**

Tenant isolation defended twice: in application authorization and again in the database. The
application's connection role must not be able to bypass RLS.

Built in `services/platform/platform/database` against
[ADR-0002](../../architecture/decisions/0002-postgresql-schema-rls-and-connection-roles.md), accepted
2026-08-24. The schema currently covers tenancy, identity and membership; later contexts add their own
tables under the same rules.

**Done when**
- [x] Every tenant-scoped table carries an RLS policy keyed to the active tenant, forced so the owner cannot bypass it.
- [x] A test proves cross-tenant `SELECT`, `INSERT`, `UPDATE`, `DELETE` and listing all fail under the app role.
- [x] A test proves a query with no tenant context returns nothing rather than everything.
- [x] A test proves tenant context does not survive the transaction on a reused connection.
- [x] A test fails the build if any table carrying `tenant_id` lacks forced row-level security.
- [x] Migration tooling runs forward from empty, is idempotent, and refuses an edited applied migration.
- [ ] Migrations run forward from the previous release, which needs a previous release to exist.
- [x] A person can read their own memberships across tenants, which no single tenant's scope can answer, without that read becoming a way to write one.
- [ ] Query plans are reviewed at volume, since every policy adds a predicate.

**Spec** [data-architecture.md](../../architecture/data-architecture.md)

---

### PLT-04 · Enforce module boundaries and forbidden imports in the Go control plane

**Depends on** DEC-03, PLT-02 · **Blocks** nothing, but protects everything · **In progress**

The modular monolith only stays modular if the boundary is machine-checked. A module reaching into
another module's tables is a build failure, not a review comment.

Implemented as a test in `services/platform/internal/architecture`, so it runs with the suite and needs
no separate command. Each rule was verified by introducing a violation deliberately, which is how a bug
in the check itself was found: it was reading only its own package and would have passed forever.

**Done when**
- [x] Import graph rules are enforced in CI and name the offending import when they fail.
- [x] No context imports another, and infrastructure does not import a context.
- [x] The AWS SDK stays in the adapter layer, which is what ADR-0001 relies on to keep the cloud reversible.
- [x] Each rule is verified by a deliberate violation rather than assumed to work.
- [ ] Table ownership is declared per module and violations fail the build. Schemas exist per ADR-0002; the check does not yet read them.

**Spec** [domain-model.md](../../architecture/domain-model.md)

---

### PLT-05 · Provision object storage with scoped upload and playback authorization

**Depends on** DEC-01, DEC-05 · **Blocks** RTC-05, SEC-04 · **In progress**

Recordings and derived media live here. The browser never gets a durable credential; every upload and
every playback is short-lived, scoped and reconciled against a manifest.

The adapter is built in `services/platform/platform/objectstore`, with keys derived rather than accepted,
presigned lifetimes clamped, and integration tests against LocalStack. What remains is the deployed side:
Terraform-created buckets with versioning, encryption and lifecycle rules, and the per-actor half of
playback authorization, which belongs to the policy layer in IAM-04 rather than to storage.

**Done when**
- [x] Upload initiation, part upload and finalization work with short-lived scoped authorization.
- [x] Finalization verifies what was stored against what the client said it sent.
- [ ] Playback authorization is per-object, per-actor and time-bound. Per-object and time-bound are done; per-actor waits on IAM-04.
- [ ] Orphaned-object reconciliation runs and reports. Discovery and abort work; the scheduled job that reports is not built.
- [ ] Buckets are created by Terraform with versioning, encryption and lifecycle rules, not by application code.

**Spec** [data-architecture.md](../../architecture/data-architecture.md)

---

### PLT-06 · Stand up Temporal with restart-safe workers and a deployment story

**Depends on** DEC-04 · **Blocks** SES-04, EVL-02, SEC-06

INT-02 was listed here and is removed: the outbox dispatcher deliberately does not run in a workflow, so
outbox delivery never depended on this ticket.

Durable orchestration for composition, evaluation, deletion and outbox delivery. Workers must survive
restart and replay without duplicating side effects.

**Done when**
- [ ] Worker restart mid-workflow replays without duplicating state, usage or notification.
- [x] Workflow versioning strategy is in place before the first workflow ships.
- [x] Namespaces separate environments and history retention matches DEC-04.

**In progress.** `platform/temporal` builds the client in one place with the data converter, the
OpenTelemetry interceptor and the scrubbing logger attached, so no call site can opt out of any of them.
The namespace is derived from the environment rather than configured, so a preview process pointing at
the production namespace is not expressible; an override is allowed only as a suffix, which is what
Temporal Cloud's account qualifier needs. Locally Temporal runs on its own PostgreSQL with the
`prepeet-local` namespace and seven-day retention, matching the deployed shape from ADR-0007 rather than
approximating it.

ADR-0007's payload rule is enforced by the converter and asserted against a real server: a workflow
started with a transcript-sized argument is refused before anything is stored, and one carrying
identifiers is accepted, so the refusal is not passing for some other reason.

The module boundary test now distinguishes the Temporal client from the workflow package. A bounded
context starting its own client would undo the reversibility ADR-0007 rests on; a context defining a
workflow with `go.temporal.io/sdk/workflow` is doing exactly what it should. Both directions verified by
planting each.

**Remaining.** The first box needs a workflow to restart in the middle of, and there is none: the first
lands with SES-04. Production and staging namespaces and their thirty-day retention are infrastructure,
so they land with PLT-09. Worker registration and task queue naming land with the first workflow.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md) · [ADR-0007](../../architecture/decisions/0007-durable-execution-with-self-hosted-temporal.md)

---

### PLT-07 · Establish secret management and workload identity

**Depends on** DEC-01 · **Blocks** SEC-03

Every deployable and every workflow worker gets its own least-privileged identity. No shared
credentials, and rotation is a routine exercise rather than an incident.

**Done when**
- [ ] Secrets are never in the repository, the image, or an environment dump.
- [ ] Rotation is demonstrated outside production without downtime.
- [ ] Each workload identity is scoped to only the resources it uses.

**Spec** [threat-model.md](../../security/threat-model.md)

---

### PLT-08 · Instrument distributed tracing, metrics and structured logging

**Depends on** PLT-01 · **Blocks** OPS-02, SEC-07, REL-02

One trace should cross browser, Go, workflow, Python, provider, database and object storage. It must
carry correlation without carrying transcript content.

**Done when**
- [ ] A single trace spans the full journey with no broken links.
- [x] Telemetry conventions are documented and shared across the three languages.
- [x] SEC-08's restricted-content scanner passes against real telemetry output.

**In progress.** The Go half is built: `platform/telemetry` with the attribute allowlist and scrubber, a
span and a latency histogram per request, panic recovery, trace-correlated structured logging, and the
scanner running against real recorded spans and real log output. Conventions are written down in
[telemetry-conventions.md](../../operations/telemetry-conventions.md) for the other two languages to
implement.

**Remaining.** The first box cannot be ticked until the web and intelligence services exist, since a
single trace has nowhere else to reach yet. Database pool and outbox instrumentation land with their
adapters. Cardinality budgets need a real series count to be set against.

**Spec** [observability.md](../../operations/observability.md) · [telemetry-conventions.md](../../operations/telemetry-conventions.md)

---

### PLT-09 · Build environment provisioning and immutable deploy with rollback

**Depends on** PLT-02, DEC-01 · **Blocks** REL-01

Local, preview, staging and production from the same definitions. Deploys are immutable and rollback is
a demonstrated procedure, not a hope.

**Done when**
- [ ] Environments are reproducible from declarative configuration.
- [ ] Rollback of a bad deploy is demonstrated end to end.
- [ ] Database restore is demonstrated and timed against the RPO/RTO in DEC-01's region choice.

**Spec** [deployment-topology.md](../../operations/deployment-topology.md) · [disaster-recovery.md](../../operations/disaster-recovery.md)

---

### PLT-10 · Build the test harness and coverage gates for all three deployables

**Depends on** PLT-01, PLT-02 · **Blocks** every implementation ticket

The project is built test first, so the harness has to exist before the first feature does. Web, Go and
Python each need a runner, fixtures, real dependencies for integration tests, and a coverage gate that
fails the build rather than producing a report nobody reads.

**Done when**
- [ ] Each deployable has a fast unit runner and an integration runner using real PostgreSQL, object storage and Temporal.
- [ ] Frontend testing covers component behaviour, routing, state, realtime and accessibility, not only pure functions.
- [ ] Coverage thresholds are enforced in CI for the frontend and the backend alike, and a drop fails the build.
- [ ] Writing a failing test first is the documented default, and the harness makes it the easy path.
- [ ] Test data builders exist so a test states what matters and inherits sensible defaults for the rest.

**Spec** [architecture-and-implementation-brief.md](../../architecture/architecture-and-implementation-brief.md)

---

### PLT-11 · Establish and enforce code documentation standards

**Depends on** PLT-02 · **Blocks** nothing, applies to everything

Every exported type, function, endpoint, workflow and component carries documentation that says which
rule it enforces and which invariant it protects. Documentation completeness is a build gate, not a
review preference.

**Done when**
- [ ] Doc comment requirements are enforced by the linter for Go, Python and shared TypeScript.
- [ ] Each module and feature directory has a README stating what it owns and what it must never do.
- [ ] Code that enforces a specification rule names the rule, so removing the rule requires noticing it.
- [ ] Generated API reference builds from the source and is published with each release.
- [ ] The standard says to document why rather than restating what the code already says.

**Spec** [architecture-and-implementation-brief.md](../../architecture/architecture-and-implementation-brief.md)
