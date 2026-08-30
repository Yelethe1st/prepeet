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
- [x] Retrying failed work never duplicates evaluation, usage or notification.
- [ ] Backlog growth alerts before candidates notice a delay.
- [x] Every operator action here is audited.

**The backend is built, the way in is a command line, and the alert has nothing to ring.**

`services/platform/internal/operations` is the context. It measures the backlog, judges it against a
budget derived from the objectives, and performs the only two actions available on work that has failed
for good. `cmd/opsctl` is the way in, because the moment this gets used is an incident and a shell is
available before a deployment is. There is no web surface: nothing in `apps/web` was touched, so the
operator-facing console this epic implies is a command line, and the third box below is ticked for the
audit trail rather than for a screen.

Retry is a state transition and not a delivery. The console moves the item back to pending and the
ordinary dispatcher carries it, so a retried item travels the path a first attempt travels: the same
handler, the same workflow identity, the same duplicate rejection. Duplication is prevented twice over
and both halves are tested. `RecoverFailed` matches only work that is still dead lettered and returns
the row it changed, so a second retry of the same item changes nothing and is refused rather than
quietly repeated. And when a delivery genuinely does happen twice, which is what at-least-once means,
the handler starts a workflow whose id is derived from the session under
`WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE` and the second start is refused by Temporal.
`TestRetryingFailedWorkTwiceEvaluatesOnce` is the proof against real PostgreSQL: the first delivery
starts the workflow and then fails after it, the item fails its way back to an operator, the operator
retries it a second time, and one evaluation exists at the end. Changing the reuse policy to
`ALLOW_DUPLICATE` makes `TestARedeliveredCompletionStartsOneEvaluation` fail with two.

The claim is bounded, and the boundary is worth stating rather than leaving to be assumed. What is
proven end to end is the evaluation route. What the property rests on elsewhere is that each existing
handler is idempotent by construction: workflow starts keyed by entity id under the same policy, and
projections that converge on unique rows. Usage and notification are not on this path at all today,
since the ledger meters on session start and `notification.emails` is a separate queue with its own
sender, so that half of the criterion holds by the absence of those routes rather than by anything this
ticket added. A future route that is not idempotent would break it, and nothing here would catch that.

Discarding is a state transition and not a delete, for the reason migration 0047 gives at length: a
deleted row cannot answer later what happened to that work, and a DELETE under forced row-level
security with no matching policy removes nothing and raises nothing, which this project has already been
bitten by once. `discarded_at` with the operator's reason keeps the decision inspectable, and
`TestDiscardedWorkStaysInTheTableAndOutOfTheQueue` fails if somebody turns it back into a delete.

Every action lands in `audit.events` in the same transaction as the transition, refusals included. An
action that cannot be audited does not happen: `TestWorkIsNotRetriedWhenItCannotBeAudited` makes the
audit write fail and asserts the item is still waiting afterwards. The rows are untenanted, which is the
case migration 0008 describes as "a platform support action", with the item as the subject and the
reason in the detail. One consequence is recorded here rather than discovered later: the untenanted
policy binds a row to the actor who wrote it, so an operator can read their own actions and not their
colleague's. Reading the whole trail is OPS-06's privileged viewer and is not built here.

The alert box is not ticked, and what is missing is the alerting rather than the threshold. The
indicator is measured every seven and a half seconds by the monitor running in the worker, recorded as
three gauges, and logged at warning level with a summary naming what breached and by how much. The
threshold is thirty seconds of pending age, derived rather than picked: a completed session crosses the
outbox twice inside the three minute practice completion-to-review objective, so both hops sitting at
the threshold would spend a third of the budget, and thirty seconds is still above the twenty an
ordinary self-healing retry costs, so a provider blinking once pages nobody.
`TestTheBacklogAgeBudgetLeavesTheJourneyBudgetIntact` fails if the number is widened to quieten it. What
does not exist is anything that turns that warning into a page. There are no alert rules in this
repository and no deployed collector configuration to hold them, so a breach today is visible to
somebody reading logs or metrics rather than delivered to whoever is on call, and "alerts before
candidates notice" is therefore half true and stays unticked.

Also not done, and outside what was built rather than inside it: stuck workflows. The ticket's own text
says "queue depth, stuck workflows, failed activities", and this answers it for the outbox only. A
workflow that is running but wedged has no surface here, because that view belongs to Temporal's own
visibility rather than to the queue, and the spec does not settle whether it lands in this ticket or in
OPS-02.

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
