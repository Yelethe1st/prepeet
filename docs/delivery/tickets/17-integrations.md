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
- [x] Templates are versioned and previewable before send.
- [x] No transcript or evaluation content appears in an email body.

**In progress.** `internal/notification` owns the queue, the versioned templates and the sender;
`platform/email` speaks SMTP, which every provider and the local Mailpit accept, so the vendor
question ADR-0001 confines never arises. Emails are enqueued in the caller's transaction for the
outbox's reason and drained by `cmd/worker`; the rendered content is erased by the same statement
that records the send, because a delivered verification link is a secret with no reason to stay
readable at rest. Previews are the golden files in `internal/notification/testdata`, so a wording
change is a reviewable diff rather than a surprise in an inbox. The content rule is structural:
a template accepts only its declared typed variables and rendering fails on anything undeclared.

The first box is half done: delivery and dead-letter status are recorded and queryable, but bounce
and complaint ingestion needs provider feedback and lands with the production provider decision.
The columns already exist so the status has one home.

The dependency on PLT-06 is unused: delivery rides the queue-and-drain shape INT-02 established,
which deliberately does not run in a workflow, so nothing here waits on Temporal.

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

A third decision surfaced while wiring the dispatcher, and is settled the same way.

**An event type nobody has decided about fails rather than being dropped.** `outbox.Router` refuses an
unregistered type, so it retries and eventually dead letters, which somebody sees. The tempting
alternative, treating unknown as nothing to do, loses data in the quietest possible way: the day somebody
adds a producer and forgets the consumer, every event of that type is marked delivered and gone, and
nothing about the system looks wrong. A type with no consumer is a legitimate state, so `Ignore` records
it along with the reason, which is required by the signature rather than by convention.

Built in `services/platform/platform/outbox` with migration 0004, and running in `cmd/worker`. Every
load-bearing guarantee was verified by removing it and watching exactly the intended test fail. The
wake-up is emitted by `Publish` inside the caller's transaction, so the signal becomes visible exactly
when the row does; PostgreSQL drops it on rollback, which is a guarantee no external transport can make
and the one place being tied to PostgreSQL is an advantage.

**Done when**
- [x] Publishing happens in the same transaction as the state change, or not at all, so a rolled back change takes its event with it.
- [x] Two dispatchers running at once do not claim the same event, using `FOR UPDATE SKIP LOCKED`.
- [x] Retries are backed off and bounded, with a dead-letter state an operator can see.
- [x] An event type without a contract version is refused, since that version is what consumers subscribe against.
- [x] The dispatcher runs in `cmd/worker`, polling on an interval.
- [x] A notification shortens the wait without being relied on for delivery.
- [ ] Replay of a delivery does not duplicate its effect for a well-behaved consumer.

**Remaining.** The last box needs a consumer to be idempotent against, and there is none yet: `routes()`
in `cmd/worker` is deliberately empty because nothing publishes an event. It closes with INT-03, the
first real handler.

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
