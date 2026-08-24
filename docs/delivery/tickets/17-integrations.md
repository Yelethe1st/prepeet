# Epic INT — Email, webhooks and ATS integration

**Phase 3 and 5** · **Workstream** Integrations, Go, Security/privacy

Outbound communication and delivery of approved events. Everything leaves through the outbox so it can
be retried, deduplicated and replayed, and nothing leaves that the tenant has not approved.

---

### INT-01 · Implement transactional email delivery

**Depends on** PLT-06, CTR-03 · **Blocks** IAM-02, SCR-04, PRC-05

Verification, recovery, magic link, OTP, invitations, reminders and results-ready notifications, with
delivery status fed back into the product.

**Done when**
- [ ] Delivery, bounce and complaint status is recorded and visible where it matters.
- [ ] Templates are versioned and previewable before send.
- [ ] No transcript or evaluation content appears in an email body.

**Spec** [data-classification.md](../../security/data-classification.md)

---

### INT-02 · Build the transactional outbox and delivery workflow

**Depends on** CTR-03 · **Blocks** INT-03

Durable, ordered, at-least-once delivery of durable events, with deduplication at the consumer boundary.

[ADR-0005](../../architecture/decisions/0005-module-boundaries-and-extraction.md) makes this load
bearing: state changes other contexts care about travel as events, so the outbox is the only way one
context tells another that something happened.

Two decisions were hiding in this ticket and are settled here rather than drifted into.

**Polling is the guarantee, `LISTEN/NOTIFY` is the optimisation.** A notification is not durable: it is
delivered to whoever is listening at that moment and is gone. A dispatcher that relied on it alone would
strand any row published while it was restarting. So the dispatcher polls on an interval, which is what
makes delivery certain, and listens as well, which is what makes it fast.

**The dispatcher lives in `cmd/worker`, not in a Temporal workflow.** A workflow has a beginning and an
end; tailing a table does not. This also removes the dependency on PLT-06, so the outbox can ship before
Temporal does, which matters because ADR-0005 already depends on it.

Built in `services/platform/platform/outbox` with migration 0004. Both load-bearing guarantees were
verified by removing them and watching the tests fail. What remains is the dispatcher process itself
and the `LISTEN/NOTIFY` wake-up, which need `cmd/worker` wired.

**Done when**
- [x] Publishing happens in the same transaction as the state change, or not at all, so a rolled back change takes its event with it.
- [x] Two dispatchers running at once do not claim the same event, using `FOR UPDATE SKIP LOCKED`.
- [x] Retries are backed off and bounded, with a dead-letter state an operator can see.
- [x] An event type without a contract version is refused, since that version is what consumers subscribe against.
- [ ] The dispatcher runs in `cmd/worker`, polling on an interval.
- [ ] A notification shortens the wait without being relied on for delivery.
- [ ] Replay of a delivery does not duplicate its effect for a well-behaved consumer.

**Spec** [event-catalog.md](../../contracts/event-catalog.md)

---

### INT-03 · Implement signed webhooks with replay and SSRF defence

**Depends on** INT-02, PLT-07 · **Blocks** INT-04

Signed payloads, timestamp windows, secret rotation, retry with backoff, and outbound request controls
that stop a tenant-supplied URL reaching internal services.

**Done when**
- [ ] Forgery, replay and SSRF tests all fail to get through.
- [ ] Secrets rotate without breaking in-flight deliveries.
- [ ] Only approved event types can be subscribed to.

**Spec** [webhook-protocol.md](../../contracts/webhook-protocol.md)

---

### INT-04 · Build webhook delivery history, test and replay

**Depends on** INT-03, WEB-04 · **Blocks** nothing

The tenant can see what was sent, what failed, send a test, and replay a delivery.

**Done when**
- [ ] Delivery history shows request, response status and attempt count without leaking the secret.
- [ ] Replay is explicit, audited and rate-limited.
- [ ] Payload previews respect data classification.

**Spec** [webhook-protocol.md](../../contracts/webhook-protocol.md)

---

### INT-05 · Implement tenant API keys with scoped capabilities

**Depends on** IAM-04, PLT-07 · **Blocks** nothing

Keys scoped to capabilities, shown once, rotatable, revocable, and audited on use.

**Done when**
- [ ] A key carries capabilities, never a role name, and cannot exceed its creator's authority.
- [ ] Revocation takes effect immediately.
- [ ] Key usage is auditable per key.

**Spec** [authorization-model.md](../../architecture/authorization-model.md)

---

### INT-06 · Build the first ATS adapter as a pilot

**Depends on** INT-03, DEC-16 · **Blocks** nothing

One named ATS, delivering only approved events, with field mapping the tenant can inspect before it goes
live.

**Done when**
- [ ] Only events the tenant approved are delivered, verified end to end.
- [ ] The mapping is visible and reversible before activation.
- [ ] Failure to deliver to the ATS never blocks or alters the review record in Prepeet.

**Spec** [webhook-protocol.md](../../contracts/webhook-protocol.md)
