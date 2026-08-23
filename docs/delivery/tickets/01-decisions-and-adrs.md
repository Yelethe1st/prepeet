# Epic DEC — Decisions and ADRs

**Phase 0** · **Workstream** Product/design, security/privacy, Principal Engineer

Nothing in this epic writes production code. It exists because [dependency-map.md](../dependency-map.md)
names these as blockers: each one, left open, can invalidate work already built. Every ticket here ends
in an accepted ADR or an approved written policy with an owner, a decision date, a review date, and a
recorded fallback scope.

A ticket in this epic is done when the decision is *made and recorded*, not when it is discussed.

---

### DEC-01 · Choose the hosting platform and regional topology

**Depends on** nothing · **Blocks** PLT-01, PLT-05, SEC-08 · **Done** 2026-08-23

Pick the cloud, the first region, and the data-residency position. Residency drives where recordings and
transcripts may live, which the retention schedule and every provider contract then inherit.

Decided in [ADR-0001](../../architecture/decisions/0001-hosting-platform-and-regional-topology.md):
AWS with ECS on Fargate, `eu-west-2` London, in-region storage with disclosed AI sub-processors, single
region with point-in-time recovery, and MinIO standing in for S3 in local development.

**Done when**
- [x] ADR accepted covering platform, first region, expansion path, and residency commitments.
- [x] Fallback scope recorded if the chosen region cannot host a required provider.
- [x] Cost assumptions handed to [cost-and-capacity-model.md](../../operations/cost-and-capacity-model.md).

**Spec** [deployment-topology.md](../../operations/deployment-topology.md)

---

### DEC-02 · Decide whether identity is built or bought

**Depends on** DEC-01 · **Blocks** IAM-01 · **Done** 2026-08-24

Password, OAuth, magic link, OTP and recovery are required for the first release; SSO and SCIM are
deferred but must not be designed out. Decide build versus vendor, and how multi-tenant membership maps
onto whichever is chosen.

Decided in [ADR-0003](../../architecture/decisions/0003-identity-built-in-go.md): built in Go, with
argon2id passwords and revocable opaque sessions rather than JWTs. Enterprise federation is deferred
behind an adapter and adopted for tenant members when a buyer requires it.

**Done when**
- [x] ADR accepted naming the provider or the build, with the enterprise-federation path.
- [x] Cost and lock-in consequences recorded, including that password security becomes a standing obligation.
- [x] Account-enumeration and invitation-confusion positions stated: identical answers for known and unknown addresses, and a dummy verification so timing does not leak what the body does not.

**Spec** [authorization-model.md](../../architecture/authorization-model.md) · [threat-model.md](../../security/threat-model.md)

---

### DEC-03 · Fix the Go modular-monolith boundary and extraction criteria

**Depends on** nothing · **Blocks** CTR-01, PLT-02

Three deployables, not a service mesh. Write down the module boundaries inside the Go control plane and
the evidence required before any module is extracted, so extraction is a decision rather than a drift.

**Done when**
- [ ] ADR accepted listing modules, their owned tables, and forbidden imports.
- [ ] Extraction criteria stated: stable module API, telemetry, load and cost evidence, ownership, migration plan.
- [ ] Enforcement mechanism named, to be implemented in PLT-04.

**Spec** [architecture-and-implementation-brief.md](../../architecture/architecture-and-implementation-brief.md)

---

### DEC-04 · Decide Temporal hosting and workflow ownership

**Depends on** DEC-01 · **Blocks** PLT-06, SES-04

Cloud or self-hosted, who operates it, and which team owns each workflow. Composition, evaluation,
deletion and outbox delivery all sit on this.

**Done when**
- [ ] ADR accepted covering hosting, namespaces, retention of workflow history, and on-call ownership.
- [ ] Disaster-recovery implications passed to [disaster-recovery.md](../../operations/disaster-recovery.md).

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

---

### DEC-05 · Decide PostgreSQL schema layout, RLS strategy and connection roles

**Depends on** DEC-01, DEC-03 · **Blocks** PLT-03, PLT-05 · **Done** 2026-08-24

Shared schema with row-level security is the proposal. Confirm it, or replace it, and define the
connection roles, including which role, if any, may bypass RLS and under what audit.

Decided in [ADR-0002](../../architecture/decisions/0002-postgresql-schema-rls-and-connection-roles.md):
one database, a schema per module, forced row-level security on every tenant-owned table, tenant context
set per transaction with `SET LOCAL`, and three roles none of which can bypass the policy.

**Done when**
- [x] ADR accepted covering schema layout, RLS policy shape, connection roles, and migration tooling.
- [x] The bypass path is named, restricted, and audited, or explicitly does not exist. It does not exist: `prepeet_app` is created `NOSUPERUSER NOBYPASSRLS`, every tenant-owned table forces its policy, and a test fails the build if one does not.

**Spec** [data-architecture.md](../../architecture/data-architecture.md)

---

### DEC-06 · Choose the realtime provider, media topology and outage fallback

**Depends on** DEC-01 · **Blocks** RTC-01, RTC-07

Browser-direct WebRTC to a provider is the proposal. Decide the provider, what happens to an
in-progress interview when it degrades, and whether a fallback path exists at all.

**Done when**
- [ ] ADR accepted covering provider, topology, authorization model for media, and degradation behaviour.
- [ ] Position recorded on whether a degraded session continues, pauses, or ends.
- [ ] Provider terms reviewed against [data-classification.md](../../security/data-classification.md).

**Spec** [realtime-protocol.md](../../architecture/realtime-protocol.md)

---

### DEC-07 · Decide the recording source, format, alignment and retention

**Depends on** DEC-06 · **Blocks** RTC-05, ART-01

Word-level timing is what makes articulation measurable and evidence playable. Decide where audio is
captured, in what format, how it is aligned to the transcript, and how long each artifact lives.

**Done when**
- [ ] ADR accepted covering capture point, container/codec, alignment method, and per-artifact retention.
- [ ] Consequence recorded for candidates who decline audio retention.

**Spec** [articulation-system.md](../../architecture/articulation-system.md) · [retention-and-deletion.md](../../security/retention-and-deletion.md)

---

### DEC-08 · Fix REST, RPC, event and generated-contract conventions

**Depends on** DEC-03 · **Blocks** CTR-01, CTR-02, CTR-03

Error shape, idempotency, pagination, versioning, envelope fields and compatibility rules — agreed once,
before three languages implement them differently.

**Done when**
- [ ] ADR accepted covering REST conventions, Protobuf conventions, event envelope, and compatibility policy.
- [ ] Breaking-change and deprecation process stated.

**Spec** [public-api.md](../../contracts/public-api.md) · [internal-rpc.md](../../contracts/internal-rpc.md) · [event-catalog.md](../../contracts/event-catalog.md)

---

### DEC-09 · Decide the artifact registry, review, publication and rollback model

**Depends on** DEC-03 · **Blocks** CAT-01, QUA-04

Rubrics, calibrations, prompts, personas and interview blueprints are all versioned artifacts pinned into
sessions. Decide where they live, who approves publication, and how a bad publication is rolled back.

**Done when**
- [ ] ADR accepted covering storage, schema versioning, digest, approval roles, and rollback.
- [ ] Confirmed that publication never mutates an in-flight or historical session.

**Spec** [domain-model.md](../../architecture/domain-model.md)

---

### DEC-10 · Decide model providers, routing, fallback and budgets

**Depends on** DEC-01 · **Blocks** EVL-01, QUA-06

Which providers, in which regions, with what routing and fallback when one degrades, and what budget
exhaustion does to a session that is already running.

**Done when**
- [ ] ADR accepted covering providers, regional policy, routing, fallback, and per-stage budgets.
- [ ] Behaviour on budget exhaustion is defined and never silently degrades a candidate result.
- [ ] Provider data-processing terms reviewed and recorded.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md) · [cost-and-capacity-model.md](../../operations/cost-and-capacity-model.md)

---

### DEC-11 · Settle screening disclosure, candidate access and appeal rights per jurisdiction

**Depends on** nothing · **Blocks** the whole of epic SCR and epic REV

The highest-risk open decision in the specification. For each launch jurisdiction: what a candidate must
be told, what they may see of their own evaluation, whether appeal is a legal right or a product feature,
and who owns the answer.

**Done when**
- [ ] Legal determination recorded per launch jurisdiction, with named approvers.
- [ ] Disclosure text versioned and approved, and consent unbundled from optional processing.
- [ ] Appeal status decided: right, tenant option, or platform policy.
- [ ] Route guards and API policy requirements handed to SCR-02 and REV-06.

**Spec** [screen-mode.md](../../product/screen-mode.md) · [responsible-hiring.md](../../security/responsible-hiring.md)

---

### DEC-12 · Define what confidence and coverage mean, and what they may not imply

**Depends on** nothing · **Blocks** EVL-05, REV-02

Confidence appears on candidate and recruiter surfaces. Decide what the number is derived from, what it
licenses a reader to conclude, and how it is prevented from reading as a probability of job success.

**Done when**
- [ ] Written definition approved by product, responsible-AI and legal.
- [ ] Numeric thresholds set by QUA-03 rather than guessed here.
- [ ] Prohibited interpretations listed and turned into content rules for A11Y-06.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### DEC-13 · Publish the supported language, accent and audio-quality matrix

**Depends on** DEC-06, DEC-07 · **Blocks** QUA-05, SCR-09

State where transcription is good enough to evaluate against, and what happens outside that boundary.
Silence here becomes a fairness problem the moment screening launches.

**Done when**
- [ ] Supported matrix published with measured word-error evidence per QUA-05.
- [ ] Out-of-matrix behaviour defined: warn, mark unassessable, or refuse to start.
- [ ] Accommodation path exists for every unsupported case.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md) · [articulation-system.md](../../architecture/articulation-system.md)

---

### DEC-14 · Decide reconnect, pause, restart and re-invitation policy for screening

**Depends on** DEC-06, DEC-11 · **Blocks** SES-06, SCR-08

A dropped connection in a screening interview is a fairness event. Decide the grace period, whether the
candidate's time is paused, who authorises a re-invitation, and how an interruption is recorded so it
never reads as poor performance.

**Done when**
- [ ] Policy approved covering grace, pause, maximum duration, and re-invitation authority.
- [ ] Policy is versioned configuration, not a UI constant.
- [ ] Interruption is represented in evidence as coverage, not as a low score.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md) · [screen-mode.md](../../product/screen-mode.md)

---

### DEC-15 · Decide retention schedules, legal hold precedence and deletion exceptions

**Depends on** DEC-01, DEC-07, DEC-11 · **Blocks** SEC-05, SEC-06, TEN-07

No duration in the specification is approved. Set them per purpose, mode, jurisdiction and data class,
and decide what happens when a hiring-record duty and a deletion request collide.

**Done when**
- [ ] Schedules approved per data category with legal basis recorded.
- [ ] Legal-hold precedence and candidate-facing explanation approved.
- [ ] Backup expiry treated as documented lag rather than selective editing.

**Spec** [retention-and-deletion.md](../../security/retention-and-deletion.md)

---

### DEC-16 · Decide the billing unit, quota behaviour and overage messaging

**Depends on** DEC-01 · **Blocks** TEN-08, OPS-05, INT-06

What a tenant is charged for — created, started, or completed sessions — and what happens at the limit.
An insufficient-evidence session is the awkward case: decide whether it is billable.

**Done when**
- [ ] Billing unit decided, including the insufficient-evidence and abandoned-session cases.
- [ ] Quota behaviour at limit decided: block, bill overage, or notify only.
- [ ] Candidate-facing consequence of a tenant hitting its limit is defined and never mid-interview.

**Spec** [cost-and-capacity-model.md](../../operations/cost-and-capacity-model.md)

---

### DEC-17 · Decide whether candidate comparison ships, and under what constraints

**Depends on** DEC-11 · **Blocks** REV-05

Comparison is off by default. Decide whether it is approved at all, and if so record the constraints —
same role, comparable rubric, two to four candidates, no ranking, mandatory individual review first.

**Done when**
- [ ] Decision recorded with responsible-hiring and legal approval, or comparison is explicitly deferred.
- [ ] If approved, constraints are written as enforceable server rules for REV-05.

**Spec** [screen-mode.md](../../product/screen-mode.md) · [responsible-hiring.md](../../security/responsible-hiring.md)

---

### DEC-18 · Decide the shared-brand question for practice and screening

**Depends on** nothing · **Blocks** WEB-02, PRC-01

One brand across practice and screening is convenient and may cost candidate trust: a candidate who
practises on Prepeet may reasonably fear their practice history reaches the employer. Decide, then make
the answer visible in the product.

**Done when**
- [ ] Decision recorded, supported by candidate research rather than internal preference.
- [ ] The isolation guarantee is stated in candidate-facing copy wherever the two modes meet.

**Spec** [product-requirements.md](../../product/product-requirements.md) · [user-journeys.md](../../product/user-journeys.md)
