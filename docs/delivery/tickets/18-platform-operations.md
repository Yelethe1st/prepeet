# Epic OPS — Platform operations and internal consoles

**Phase 3–6** · **Workstream** Platform, Go, Web

What the people running the service need in order to answer a support question or an incident without
reading a candidate's transcript. Cross-tenant access is exceptional, not ordinary navigation.

---

### OPS-01 · Build privacy-controlled aggregate analytics

**Depends on** PLT-08, IAM-06 · **Blocks** nothing

Cross-tenant volumes, funnels and outcomes at an aggregate level, with small-population suppression so
an aggregate cannot identify one person.

**Done when**
- [ ] Aggregates suppress or refuse to render below a documented population threshold.
- [ ] No aggregate view exposes a transcript, a recording or an identifiable candidate.
- [ ] Every chart has a text summary and a table alternative.

**Spec** [observability.md](../../operations/observability.md) · [data-classification.md](../../security/data-classification.md)

---

### OPS-02 · Build session and realtime health monitoring

**Depends on** PLT-08, RTC-02 · **Blocks** REL-02

Connection success, reconnection rate, media integrity, provider latency and session-state distribution
— the signals that say the live product is healthy.

**Done when**
- [ ] Realtime failures are visible before support tickets arrive.
- [ ] A single session can be inspected by state and timing without exposing its content.
- [ ] Alerts fire on the journey indicators in [service-level-objectives.md](../../operations/service-level-objectives.md).

**Spec** [observability.md](../../operations/observability.md)

---

### OPS-03 · Build workflow backlog and failed-work recovery

**Depends on** PLT-06 · **Blocks** REL-02

Queue depth, stuck workflows, failed activities and a retry path that is safe to use under pressure.

**Done when**
- [ ] Retrying failed work never duplicates evaluation, usage or notification.
- [ ] Backlog growth alerts before candidates notice a delay.
- [ ] Every operator action here is audited.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

---

### OPS-04 · Build evaluation quality and artifact version monitoring

**Depends on** QUA-06, CAT-01 · **Blocks** REL-02

Which model, prompt and rubric versions are live, and how quality metrics are moving under them.

**Done when**
- [ ] Live artifact and model versions are visible in one place.
- [ ] Quality regressions raise an alert tied to the version that introduced them.
- [ ] Rollback from this screen is possible and audited.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### OPS-05 · Build provider usage, cost and quota control

**Depends on** DEC-10, DEC-16, TEN-08 · **Blocks** REL-04

Token and audio spend by provider, cost per completed session, and the controls to change a tenant's
quota with a recorded reason.

**Done when**
- [ ] Cost per created, started, completed and review-ready session is measured and visible.
- [ ] Quota changes require a reason and are audited.
- [ ] Spend anomalies alert before the invoice does.

**Spec** [cost-and-capacity-model.md](../../operations/cost-and-capacity-model.md)

---

### OPS-06 · Build the privileged append-only audit viewer

**Depends on** IAM-07, PLT-03 · **Blocks** REV-04, REL-03

Every privileged and sensitive action, immutable, searchable, and itself audited when read.

**Done when**
- [ ] Audit records cannot be edited or deleted by any application path.
- [ ] Reading the audit log is itself an audited action.
- [ ] Records carry actor, tenant, purpose, correlation identifier and time.

**Spec** [observability.md](../../operations/observability.md)

---

### OPS-07 · Build the restricted super-administrator console

**Depends on** IAM-07 · **Blocks** nothing

Break-glass operations behind elevation, with every action reason-bound, time-bound and visible.

**Done when**
- [ ] The console is unreachable without an active elevation.
- [ ] Each destructive operation states its blast radius before it runs.
- [ ] An active elevation is visible to the whole operations team, not just its holder.

**Spec** [authorization-model.md](../../architecture/authorization-model.md)
