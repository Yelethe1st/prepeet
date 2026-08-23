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
- [ ] `web`, `api` and `intelligence` build reproducibly from a clean checkout.
- [ ] Formatting, linting and type checking run identically locally and in CI.
- [ ] A new engineer can run the whole stack locally from a documented single command.

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

**Depends on** DEC-05 · **Blocks** IAM-03, SEC-02

Tenant isolation defended twice: in application authorization and again in the database. The
application's connection role must not be able to bypass RLS.

**Done when**
- [ ] Every tenant-scoped table carries an RLS policy keyed to the active tenant.
- [ ] A test proves cross-tenant `SELECT`, `INSERT`, `UPDATE` and `LIST` all fail under the app role.
- [ ] Migration tooling runs forward from empty and from the previous release.

**Spec** [data-architecture.md](../../architecture/data-architecture.md)

---

### PLT-04 · Enforce module boundaries and forbidden imports in the Go control plane

**Depends on** DEC-03, PLT-02 · **Blocks** nothing, but protects everything

The modular monolith only stays modular if the boundary is machine-checked. A module reaching into
another module's tables is a build failure, not a review comment.

**Done when**
- [ ] Import graph rules are declared as data and enforced in CI.
- [ ] Table ownership is declared per module and violations fail the build.
- [ ] The rule file is the same artifact the extraction criteria in DEC-03 refer to.

**Spec** [domain-model.md](../../architecture/domain-model.md)

---

### PLT-05 · Provision object storage with scoped upload and playback authorization

**Depends on** DEC-01, DEC-05 · **Blocks** RTC-05, SEC-04

Recordings and derived media live here. The browser never gets a durable credential; every upload and
every playback is short-lived, scoped and reconciled against a manifest.

**Done when**
- [ ] Upload initiation, part upload and finalization work with short-lived scoped authorization.
- [ ] Playback authorization is per-object, per-actor and time-bound.
- [ ] Orphaned-object reconciliation runs and reports.

**Spec** [data-architecture.md](../../architecture/data-architecture.md)

---

### PLT-06 · Stand up Temporal with restart-safe workers and a deployment story

**Depends on** DEC-04 · **Blocks** SES-04, EVL-02, SEC-06, INT-02

Durable orchestration for composition, evaluation, deletion and outbox delivery. Workers must survive
restart and replay without duplicating side effects.

**Done when**
- [ ] Worker restart mid-workflow replays without duplicating state, usage or notification.
- [ ] Workflow versioning strategy is in place before the first workflow ships.
- [ ] Namespaces separate environments and history retention matches DEC-04.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

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
- [ ] Telemetry conventions are documented and shared across the three languages.
- [ ] SEC-07's restricted-content scanner passes against real telemetry output.

**Spec** [observability.md](../../operations/observability.md)

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
