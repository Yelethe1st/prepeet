# Epic SEC — Security, privacy and data rights

**Phase 0–5, continuous** · **Workstream** Security/privacy

Threat modelling starts in Phase 0 and never finishes. Candidate data rights are product features with
owners and SLAs, not a mailbox somebody checks.

---

### SEC-01 · Produce and maintain the threat model

**Depends on** DEC-01, DEC-02 · **Blocks** nothing, informs everything

Trust boundaries, threats, abuse cases and controls, reviewed on a defined cadence rather than once at
the start.

**Done when**
- [ ] Every trust boundary in the architecture has an owner and a control set.
- [ ] Abuse cases include candidate-directed harms, not only platform-directed ones.
- [ ] The review cadence is scheduled and the first review has happened.

**Spec** [threat-model.md](../../security/threat-model.md)

---

### SEC-02 · Prove tenant isolation adversarially

**Depends on** PLT-03, IAM-06 · **Blocks** REL-03

Not a unit test that passes — a deliberate attempt to cross a tenant boundary through every layer:
API, SQL, objects, workflows, caches, analytics and telemetry.

**Done when**
- [ ] Each layer has an explicit cross-tenant attempt that fails.
- [ ] The suite runs in CI and a new table without RLS breaks the build.
- [ ] An independent tester repeats the exercise before screening pilot.

**Spec** [threat-model.md](../../security/threat-model.md) · [release-criteria.md](../release-criteria.md)

---

### SEC-03 · Implement data classification controls across storage and transport

**Depends on** PLT-05, PLT-07 · **Blocks** SEC-07

Restricted, confidential, internal and public each get defined handling — where they may be stored,
logged, exported and processed.

**Done when**
- [ ] Every data category in the inventory has a classification and a handling rule.
- [ ] Restricted content never reaches a log, trace, metric or error report.
- [ ] Exports carry the classification of their most sensitive field.

**Spec** [data-classification.md](../../security/data-classification.md)

---

### SEC-04 · Implement consent lifecycle including withdrawal

**Depends on** SCR-02, CAT-05 · **Blocks** SEC-05

Purpose-specific, versioned consent that can be withdrawn — with an honest account of what withdrawal
can and cannot undo.

**Done when**
- [ ] Withdrawal stops future consent-dependent processing immediately.
- [ ] The candidate is told, in writing, what was stopped and what was retained and why.
- [ ] Consent records survive as evidence even after the data they governed is deleted.

**Spec** [retention-and-deletion.md](../../security/retention-and-deletion.md)

---

### SEC-05 · Implement candidate data export and correction

**Depends on** PRO-04, SEC-03 · **Blocks** nothing

A candidate can get a copy of what is held about them, in a usable format, and correct what is wrong.

**Done when**
- [ ] Export includes profile, documents, extracted facts, sessions, transcripts, evaluations and consents.
- [ ] Export is delivered securely and expires.
- [ ] Corrections propagate to the surfaces that used the incorrect fact.

**Spec** [retention-and-deletion.md](../../security/retention-and-deletion.md)

---

### SEC-06 · Build the durable deletion workflow with reconciliation

**Depends on** PLT-06, DEC-15, TEN-07 · **Blocks** SEC-07

Discover scope, evaluate holds, freeze new processing, delete across every store, request provider
deletion, reconcile, and report what could not be deleted and why.

**Done when**
- [ ] Deletion is idempotent and reports per-system status.
- [ ] No new derivative is created after the freeze.
- [ ] Reconciliation proves the objects are gone; backup expiry is documented rather than faked.

**Spec** [retention-and-deletion.md](../../security/retention-and-deletion.md)

---

### SEC-07 · Build the candidate data-request status surface

**Depends on** SEC-05, SEC-06 · **Blocks** nothing

*Gap found against the prototype: settings offer export and deletion, but there is nowhere to see a
request's status, its SLA, or why something was withheld.*

**Done when**
- [ ] A candidate can see each request's state, owner and approved SLA.
- [ ] Exceptions — legal hold, retained hiring record — are explained in plain language.
- [ ] Completion produces evidence the candidate can keep.

**Spec** [retention-and-deletion.md](../../security/retention-and-deletion.md)

---

### SEC-10 · Rate limit authentication and every other abusable endpoint

**Depends on** IAM-01 · **Blocks** REL-02

[ADR-0003](../../architecture/decisions/0003-identity-built-in-go.md) chose to build authentication
rather than buy it, and named credential stuffing as the standing obligation that comes with that
choice. A vendor would have been doing this work continuously; now it is ours.

Login, registration, password recovery, magic link and OTP are all endpoints where an attacker gets
unlimited attempts unless something stops them. Rate limiting is also the only defence that works
before a breached-password check exists.

Counting starts in memory, which is correct while there is one instance and wrong the moment there are
two: each would enforce its own share of the limit. Redis is the upgrade, and it is not worth
provisioning before the second instance exists.

The counter is built in `services/platform/platform/ratelimit`. What remains is applying it to the
authentication routes, which needs the handlers in IAM-01.

**Done when**
- [x] The limiter counts per key with a moving window, and forgets keys it no longer needs.
- [x] The limiter cannot distinguish a registered address from an unknown one, because it never looks either up.
- [x] An empty key is refused rather than collapsing every anonymous caller into one bucket.
- [x] An unusable rule fails at construction rather than locking out every user at runtime.
- [ ] Authentication endpoints are limited per address and per network, not per address alone, since one attacker with many addresses is the ordinary case.
- [ ] A limited response is `429` with `Retry-After`, per ADR-0004.
- [ ] Limits are configuration rather than constants, so they can be tightened during an incident.
- [ ] The counter moves to Redis before a second instance runs, and the ticket that adds the instance is what triggers it.

**Spec** [threat-model.md](../../security/threat-model.md)

---

### SEC-08 · Run restricted-content scanning across telemetry

**Depends on** PLT-08, SEC-03 · **Blocks** REL-01

An automated scanner that looks for transcript fragments, personal data and secrets in logs, traces,
metrics and error reports.

**Done when**
- [ ] The scanner runs continuously and fails the release gate on a finding.
- [ ] It covers third-party error reporting, not only first-party logs.
- [ ] A finding is treated as an incident, with the same review as a leak.

**Spec** [observability.md](../../operations/observability.md)

---

### SEC-09 · Commission independent penetration and isolation testing

**Depends on** SEC-02, SCR-07 · **Blocks** REL-03

An external party attempts what the team believes is impossible, before a real candidate's screening
interview depends on it.

**Done when**
- [ ] Scope covers tenant isolation, practice/screen separation, media authorization and privileged access.
- [ ] Findings are triaged against policy with owners and dates.
- [ ] Retest confirms the fixes before the screening pilot opens.

**Spec** [release-criteria.md](../release-criteria.md)
