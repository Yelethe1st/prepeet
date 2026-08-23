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

**Depends on** PLT-06, CTR-03 · **Blocks** INT-03

Durable, ordered, at-least-once delivery of durable events, with deduplication at the consumer boundary.

**Done when**
- [ ] An event is never lost when the process dies between commit and publish.
- [ ] Retries are backed off and bounded, with a visible dead-letter path.
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
