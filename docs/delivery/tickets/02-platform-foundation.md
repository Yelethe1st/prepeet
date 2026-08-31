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
- [x] Pipeline fails on contract drift, forbidden imports, or a failing migration.
- [ ] Build artifacts are immutable and digest-addressed.
- [ ] Pipeline duration is fast enough that engineers do not route around it.

**One of three, and the first was already true.** Contract drift fails in the contracts job:
`make check-generated` regenerates everything from the contracts and fails on a diff, which is what
makes ADR-0004's "the contract is the source" enforceable rather than aspirational. Forbidden
imports fail in the Go job, because `internal/architecture` is an ordinary test package and runs
with the suite; PLT-04 added table ownership to it, so a module reading another module's schema
now fails there too. A failing migration fails in the same job: every integration suite calls
`database.Migrate` against a real PostgreSQL before it asserts anything, so a migration that does
not apply takes the build down before a single test runs.

The compatibility gates joined them with CTR-04: events, RPC and now the HTTP contract each refuse
a change that would break a deployed consumer.

**Two of those three failures now have their own name, and every one of them can be run on a
laptop.** A forbidden import failed inside a ten minute suite among container logs, and a broken
migration failed as whichever integration test happened to run first. `make check-boundaries` and
`make check-migrations` are named steps ahead of and beside the suite: the first answers in under
two seconds and needs nothing running, the second in about eight against a real PostgreSQL in a
container. Both were verified by planting the violation. An import of `internal/identity` into
`internal/candidate` fails `TestNoContextImportsAnother` by name, and a migration referencing a
table that does not exist fails with the version and the name of the migration that broke.

**The dependency audit from this ticket's description exists now; the container scan still does
not.** `make audit` gates all three languages against what deploys: govulncheck over the Go module,
pip-audit over the resolved Python runtime set, and `pnpm audit --prod` over the web production
dependencies. Each was proved by feeding it something vulnerable rather than by watching it pass:
a rewritten govulncheck report with a reachable advisory in pgx, a requirements file pinning a
known-bad `requests`, and the development dependency set, which fails today. The Go half needed a
policy, so `tools/vulncheck` reads govulncheck's JSON and decides. It refuses an empty report,
because a scanner that could not reach the vulnerability database prints nothing and nothing reads
as clean. Writing it found a fault in itself before it found one in the repository: govulncheck
reports each advisory first as a fact about the dependency graph and then once per traced call, so
taking the first report classified all 24 reachable advisories as unreachable. A gate that passes
what it exists to catch is worse than no gate, which is why the count is now checked against
govulncheck's own.

**The Go coverage floor was not the only number written twice.** The Python job carried its own
`--cov-fail-under=80` beside `PY_COVERAGE_MIN`, and the Makefile carried a `WEB_COVERAGE_MIN` that
no recipe read and that said 80 against the 95 Vitest actually enforces: a copy that was not merely
free to drift but already wrong, and would have been believed by whoever read it first. Both are
gone, and so are the last hand written commands in the workflow, which now names a make target for
every build, suite and check. The one exception is the browser job, and it is on the line: the
Playwright container ships no `make`, which was found by running the image rather than by pushing.

**What this pipeline does not check.** No container image is built, published or scanned, so
nothing is digest addressed and there is no image to scan. Standard library advisories are reported
and not gated: 24 are reachable today, cleared by Go 1.26.6 against the 1.26.0 pinned in
`services/platform/go.mod`, and gating a toolchain bump this ticket cannot make would only teach
people to skip the gate. Development toolchains are not audited either, and there are findings
there now: one critical in vitest and one high in vite, neither reachable from anything a user
talks to. `cmd/` remains outside the coverage measurement, and measuring it separately puts it at
11.0% without the integration tag, so the 81.3% the floor reads is a statement about `platform/` and
`internal/` and not about the wiring that starts them. And documentation link checking, which was written out inside
the workflow file and could only be run by pushing, is `make check-docs`, which is the same fix as
the coverage floor for the same reason.

The two remaining boxes are genuinely undone rather than unticked. Nothing builds a container at
all, so there is no artifact to address by digest, and that arrives with PLT-09's deployment work.
Pipeline duration still has no number that came from a hosted runner: the gates were timed on a
laptop, the jobs are parallel and the longest is the Go suite, but a box that says "fast enough"
closes on a measurement rather than on an argument.

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
- [x] Table ownership is declared per module and violations fail the build.

**Done.** `internal/architecture/ownership_test.go` declares which schemas each module may name and
fails the build for any other. A declaration rather than an inference: deriving ownership from what
the queries already do would make every existing crossing legal by definition and the check
vacuous. `audit` is everybody's and named as the one exception, because a module that could not
record what it did would be a module whose decisions are unreviewable, and migration 0008 makes it
append-only by grant so sharing it cannot mean editing it. Tenancy is identity's, because IAM-03
makes identity the one place that decides who may act under which tenant.

The rule is only complete because ADR-0010 puts every statement through sqlc, so the query files
are the whole surface. That is asserted rather than assumed: a third rule refuses hand-written SQL
anywhere in a module, because without it the ownership check would pass while measuring nothing,
which is precisely how the first rule in this package was found broken.

A second rule refuses a module that has queries and no declared ownership, so the check cannot
silently skip whichever module is added next.

All three were verified by introducing the violation and watching them fail: a billing query
reading `interview.sessions`, billing's declaration deleted, and a raw statement in
`billing/ledger.go`. Each named the offender.

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
- [x] Worker restart mid-workflow replays without duplicating state, usage or notification.
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

**The first workflow arrived with SES-01's composition rather than SES-04, and the first box closed
with it.** The restart is proven the hard way in the interview package's integration suite: a composer
blocks inside its activity while the worker running it is stopped, a second worker picks up the retry,
and then everything is counted - composition ran at least twice, which is at-least-once being
exercised, while the session advanced exactly once, the catalogue event published exactly once, and
the audit trail holds exactly one ready row. The exactly-onces come from the aggregate's version
guards and idempotent activities, not from trusting delivery semantics Temporal does not offer.

Task queues are named by bounded context - the interview queue is `prepeet-interview` - so queue
ownership follows module ownership. The worker serves it only when the intelligence plane is also
configured, because a worker polling a queue without its composer would take tasks it can only fail.

**Remaining.** Production and staging namespaces and their thirty-day retention are infrastructure,
so they land with PLT-09.

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

**Three of the four breaks are closed. The box stays open because the browser is still one of them.**

The journey broke in four places, and three were where the work actually happens.

The **queue** was the first. A request that publishes an event and the worker that later delivers it
are one piece of work to everybody except the tracing system, which saw two: the trace ended at the
HTTP response and an unrelated one began when the dispatcher picked the row up. Migration 0054 carries
W3C trace context on the row, captured from the publisher's context rather than passed as a parameter,
so a caller cannot forget it. Delivery rejoins before its span starts, so it is a child of the request
rather than the root of a second trace.

The **language boundary** was the widest. The gRPC client sent no trace context at all, so extraction,
evidence and articulation, which are the slowest work in the product, could not be connected to the
request that caused them. The client injects it now and the Python server continues it.

The **Python plane** had no OpenTelemetry at all, so even once the context arrived there was nothing
to receive it.

Both rules that hold at every hop exist because the failure they prevent is invisible. Absent context
starts a fresh trace and is never invented, because a zero traceparent produces spans pointing at a
trace nobody can find. Malformed context is ignored rather than trusted, because a span attached to a
parent that cannot exist looks joined and leads nowhere.

Each propagator is named where it is used rather than read from the process-wide default, which is a
noop until something installs one. An attack proved that is not theoretical: switching the gRPC client
to the global propagator makes it propagate nothing while every other test still passes.

Nine guards proven by breaking them across Go and Python. One of them found a defect in my own test:
removing the interceptor from the Python server left all 347 tests green, because the test asserted
only that the call raised, which it does either way. It now reads the recorded spans and fails when
the interceptor is gone.

**What is left is the browser.** `apps/web` sends no `traceparent`, so a trace begins at the Go edge
rather than at the click. Everything server-side is joined; the first hop is not, and the criterion
says the full journey. Also unjoined: database spans, since pgx carries no tracer, and outbound
provider calls to LiveKit, the OIDC providers and the SMTP relay, none of which use an instrumented
transport. Those are visible as their caller's span rather than their own, which is a thinner trace
rather than a broken one.

**In progress.** The Go half is built: `platform/telemetry` with the attribute allowlist and scrubber, a
span and a latency histogram per request, panic recovery, trace-correlated structured logging, and the
scanner running against real recorded spans and real log output. Conventions are written down in
[telemetry-conventions.md](../../operations/telemetry-conventions.md) for the other two languages to
implement.

**Verified against a running collector, not only in tests.** The local stack now receives what the
services send, so the claims in this ticket are checkable. A request produces one span named for its
route template rather than its resolved path; a request arriving with a `traceparent` joins that trace
with the caller's span as its parent; the restricted-content scan finds nothing across the spans a real
session produced; and the latency histogram arrives with route, method and status and no unbounded
dimension, with three rejected logins aggregating into one series.

That last one only became checkable because the arrangement changed. Sending straight to Jaeger meant
metrics were exported and discarded, and logged a failure every thirty seconds while doing it.

**Remaining.** The first box needs the browser and the intelligence service. The Go tier continues an
inbound trace correctly, which is the half a browser SDK would hand to, but nothing in the browser emits
one yet and the Python service does not exist. Database pool and outbox instrumentation land with their
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
