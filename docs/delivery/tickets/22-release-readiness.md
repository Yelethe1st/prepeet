# Epic REL — Release readiness and operational proof

**Phase 3, 5 and 6 gates** · **Workstream** Platform, Security/privacy, Product

The gates in [release-criteria.md](../release-criteria.md) turned into work. A gate is not passed by
asserting it; it is passed by producing the evidence and recording who accepted it.

---

### REL-01 · Pass the foundation release gate

**Depends on** PLT-09, CTR-04, SEC-08 · **Blocks** REL-02

Reproducible builds, contract gates, enforced module boundaries, RLS proof, least-privilege identities,
Temporal replay safety, complete traces, demonstrated rollback and restore, artifact publication and
rollback, and a clean telemetry scan.

**Done when**
- [ ] Every foundation checklist item in [release-criteria.md](../release-criteria.md) has recorded evidence.
- [ ] Rollback and database restore have been performed, not described.
- [ ] The release record for the foundation milestone is complete and approved.

**Spec** [release-criteria.md](../release-criteria.md)

---

### REL-02 · Pass the practice release gate

**Depends on** REL-01, PRC-01, ART-05, A11Y-04, QUA-04 · **Blocks** REL-03

The whole candidate journey, its documented states, evidence-citing coaching, assessability handling,
realtime resilience, SLOs under load, AI quality gates, accessibility, privacy rights and measured cost.

**Done when**
- [ ] Every practice checklist item has recorded evidence, including the accessibility and AI gates.
- [ ] Journey SLOs are measured under representative and burst load, not estimated.
- [ ] Cost per created, started, completed and review-ready session is measured.

**Spec** [release-criteria.md](../release-criteria.md)

---

### REL-03 · Pass the screening release gate for a named pilot

**Depends on** REL-02, SEC-09, SCR-07, REV-06, DEC-11 · **Blocks** nothing

Named jurisdictions, approved disclosure, proven isolation, enforced disclosure policy, audited access,
human-owned decisions, operational appeals and accommodations, published supported-matrix limits, and
independent testing.

**Done when**
- [ ] Every screening checklist item has recorded evidence with a named approver.
- [ ] Legal, security, privacy, accessibility and responsible-hiring each sign off explicitly.
- [ ] The pilot is limited to named tenants, roles and jurisdictions, and that limit is enforced technically.

**Spec** [release-criteria.md](../release-criteria.md) · [responsible-hiring.md](../../security/responsible-hiring.md)

---

### REL-04 · Establish SLOs, error budgets, dashboards and alerts

**Depends on** PLT-08, OPS-02 · **Blocks** REL-02

Journey indicators with proposed targets, measured against real traffic, with error budgets that
actually change behaviour when they are spent.

**Done when**
- [ ] Each journey indicator has a measured baseline before a target is committed.
- [ ] Alerts are actionable and map to a runbook.
- [ ] Error-budget policy states what happens when the budget is exhausted.

**Spec** [service-level-objectives.md](../../operations/service-level-objectives.md)

---

### REL-05 · Write and exercise the operational runbooks

**Depends on** OPS-03, PLT-09 · **Blocks** REL-02

Provider outage, evaluation backlog, workflow recovery, key rotation, restore, deletion failure and
integrity freeze — each written and then actually rehearsed.

**Done when**
- [ ] Each runbook has been executed in a drill by someone who did not write it.
- [ ] On-call ownership is assigned for every deployable and workflow.
- [ ] Gaps found in the drill are fixed before the gate they support.

**Spec** [disaster-recovery.md](../../operations/disaster-recovery.md)

---

### REL-06 · Exercise the integrity freeze and re-review procedure

**Depends on** REV-06, OPS-06 · **Blocks** REL-03

If an evaluation defect is discovered after decisions were made, the affected population must be
identifiable, freezable, and re-reviewable — with the affected candidates told.

**Done when**
- [ ] An affected population can be identified by artifact version and frozen.
- [ ] Impact assessment and re-review run end to end in a rehearsal.
- [ ] Candidate and tenant communication templates exist and are legally approved.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md) · [disaster-recovery.md](../../operations/disaster-recovery.md)

---

### REL-07 · Produce the release record for each gated milestone

**Depends on** REL-01 · **Blocks** nothing

Scope, tenants, modes, image and migration digests, artifact and contract versions, flags, test and AI
reports, approvals, SLO and capacity evidence, risks and exceptions, rollback plan, approvers and time.

**Done when**
- [ ] The record is produced automatically from the pipeline wherever possible, not assembled by hand.
- [ ] Exceptions are recorded with an owner and an expiry, never left implicit.
- [ ] Past release records are retrievable and immutable.

**Spec** [release-criteria.md](../release-criteria.md)
