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
- [x] Every trust boundary in the architecture has an owner and a control set.
- [x] Abuse cases include candidate-directed harms, not only platform-directed ones.
- [ ] The review cadence is scheduled and the first review has happened. The first review has happened
  and the cadence is defined; nothing schedules it, which is the document's own R10.

**Two of three, and the third is the one the ticket's word "maintain" was actually asking about.**

The previous model listed fourteen threats against generic controls: "Go resource policy, repository
tenant predicates, RLS, adversarial list/batch tests" for cross-tenant leakage, and so on for the rest.
Every line of it was defensible and none of it was checkable, because nothing in it named a file, and a
control nobody can locate is indistinguishable from one nobody wrote. The rewrite traces each threat to
the code, migration or trigger that mitigates it, or says plainly that nothing does.

Each entry now carries two states rather than one, because conflating them is how these documents
overstate themselves. A control state, what the code refuses. An assurance state, what has tried to
break it. Nothing anywhere reads "attacked" by an independent party, and the section that says what has
actually been attacked leads with SEC-02's own limits rather than its coverage: three layers, two
request paths, and no objects, workflows, caches, analytics or telemetry.

**Reading the code rather than the architecture found five places the documents were ahead of it**, and
they are recorded as open risks rather than fixed, because an undiscussed security change buried in a
documentation ticket is worse than the gap.

`recruiting.campaign` carries a tenant policy only. Migration 0043 says per-campaign scope is "the
database's rather than a handler's", and for `campaign_recruiter` that is true, but a query reading
`recruiting.campaign` under tenant scope without the join sees every campaign in the tenant, and
`authz.ScopeCampaign` is consulted by no production code at all. `evaluation.evidence_spans` and
`evaluation.contradictions` grant `DELETE` with `UPDATE`-only triggers, deliberately, so a retried
extraction converges, but `evaluation.results` holds no digest over the spans it cites and nothing
checks whether a result exists before evidence is replaced. Three of ADR-0019's four provider terms are
unenforceable from code, which that ADR already says. No read of a transcript, audio file or evaluation
writes an audit row, which `authorization-model.md` and `data-classification.md` both require.
`SameSite=Lax` is the entire CSRF defence: there is no token, no `Origin` check and no `Sec-Fetch-Site`
check anywhere, which `grep` confirms rather than infers.

**The third box is open on purpose.** The cadence is defined and event-driven, which is right, since a
calendar review of an unchanged system is theatre. But there is no CODEOWNERS file, no scheduled
review, and no CI gate that fails when a migration adds a table or an ADR moves a boundary without this
document moving. The one thing that does hold is indirect: `internal/isolation/registry_test.go` caps
its declaration lists, so widening what the database does not defend is a deliberate edit with a reason
in a commit message, and that edit is the signal a review is due. Until R10 closes, this is maintained
by whoever remembers, and ticking a box that says "scheduled" would be the first untrue line in a
document whose value is that it has none.

**Spec** [threat-model.md](../../security/threat-model.md)

---

### SEC-02 · Prove tenant isolation adversarially

**Depends on** PLT-03, IAM-06 · **Blocks** REL-03

Not a unit test that passes — a deliberate attempt to cross a tenant boundary through every layer:
API, SQL, objects, workflows, caches, analytics and telemetry.

**Done when**
- [ ] Each layer has an explicit cross-tenant attempt that fails. The HTTP handler, the bounded
  context and the database are attacked. Objects, workflows, caches, analytics and telemetry are not.
- [x] The suite runs in CI and a new table without RLS breaks the build.
- [ ] An independent tester repeats the exercise before screening pilot. Nobody but the author has run
  it, and the author cannot tick this one.

**In progress.** The suite is `services/platform/internal/isolation`, and what it does not cover
matters as much as what it does.

Three layers are attacked. The HTTP handler, given another workspace's membership identifier in the
path and another workspace's identifier in the body of the one endpoint that accepts one. The bounded
context, called with a tenant and a membership that do not belong together. The database, sent
statements that name a foreign row by primary key. The ticket also names objects, workflows, caches,
analytics and telemetry, and none of those is attacked at all, which is why the first box is not
ticked.

Every attack obeys three rules, because one that breaks any of them passes whether or not the guard
exists. It names a row that exists under the other tenant at that moment. It is otherwise valid: the
version guard satisfied, the capability held, the session live, and the target deliberately not an
owner, since member administration refuses to touch an owner row for reasons that have nothing to do
with tenancy. And each has a control, the same operation on the same row with the same arguments from
the tenant that owns it, which succeeds. Without the control, a refusal is indistinguishable from an
attack that missed, and an attack that misses is the mistake this repository has already made once.

**Proven by removing each guard and watching a named test fail.** Replacing the memberships tenant
policy with `USING (true)` failed the three database-layer attacks by name. It did not fail the
handler or the context attacks, which is the honest finding rather than a gap: those are defended
twice, by the policy and by the tenant predicate in the query, and removing the predicate as well is
what fails them. Opening `audit.events` the same way fails the audit-trail attack, and making the
membership check in tenant selection always answer yes fails the tenant-selection attack. Each edit
asserted that it had changed the file, and the tree was checked clean again afterwards.

That exercise also found a hole in the gate itself. The first version of the policy rule asked whether
a table had at least one policy keyed to the caller, and the memberships table stayed green with its
tenant policy replaced by `USING (true)`, because two other policies on it still named `app.user_id`.
PostgreSQL ORs permissive policies together, so one that admits everyone re-opens the table however
well the others are written. The rule now demands that every policy on a table be keyed.

**The structural half is the more valuable one.** Every table the migrations create is in exactly one
of three states, and each has to be arrived at on purpose: row-level security keyed to the caller,
row-level security whose policy deliberately admits everyone, or none at all. The last two are
declared in `registry_test.go` with a reason each and a cap on how many there may be, because a list
that grows one justified entry at a time becomes a list of everything. A table carrying a `tenant_id`
can be declared away only through a field that says so in the open, which exactly one table uses:
`integration.outbox`, whose dispatcher acts for no tenant.

A policy counts as keyed when it names `app.tenant_id` or `app.user_id`, or when it delegates to a
table that does, which is how `interview.session_bundles` is scoped through the session it belongs to
without carrying a tenant of its own. Demanding `app.tenant_id` in every policy would have been
simpler and wrong: it would have failed correct tables and taught whoever hit it to write the
predicate the gate wanted rather than the one the data needed.

The gate is static, so it needs no Docker and fails in the ordinary `go test` rather than four minutes
into CI, and it is checked against the live database in the integration run. The two must see exactly
the same set of tables. That equality is not tidiness: the scanner is a parser, and a parser that
stops recognising a statement reports a clean bill of health forever. It had already happened once,
against migration 0005's unqualified `CREATE UNLOGGED TABLE`.

Verified by adding a scratch migration and watching the build break by name, then removing it. A
tenant-scoped table with no policy fails `TestEveryTableIsIsolatedOrDeclaredNotToBe`. A table whose
policy admits everyone fails the same test with a different message and passes once declared, which is
how migration 0043's jurisdiction determination is handled. A table carrying `tenant_id` added to the
declaration without the field that admits it fails
`TestATableCarryingTenantIDCannotBeDeclaredAwayQuietly`, and the cap fails alongside it.

**What this does not cover.** Object storage, Temporal workflows, caches, analytics and telemetry are
not attacked, and neither is any export, webhook or signed URL: the suite exercises one instance's own
request path and says nothing about a leak through something it hands out. Only member administration
and tenant selection are attacked at the handler and context layers, so the interview, evaluation,
billing, candidate, content, progression and recruiting contexts have their tables checked by the
structural gate and no request-level attack of their own. The practice and screening separation is
proven separately in `platform/database/practice_isolation_integration_test.go`. Identity's own tables
carry no row-level security by design, which is declared rather than defended: nothing in this suite
protects them, and what does is the query predicate and the policy layer.

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

Counting is in PostgreSQL rather than in memory. ADR-0001 runs more than one ECS task as the normal
shape for availability, so an in-memory counter is not a smaller version of this: it is wrong, and an
attacker gets the limit multiplied by the task count. PostgreSQL rather than Redis because the write
costs about a millisecond against the hundred argon2id already spends, and because a counter in the
same store as the credentials it protects cannot be down while they are up.

Built in `services/platform/platform/ratelimit` with migration 0005, in two implementations behind one
interface: PostgreSQL for anything deployed, in-memory for tests and local development. The behaviour
every counter must have is written once as a shared contract that each implementation runs, so a third
one, Redis or otherwise, inherits the assertions rather than copying them. What remains is applying it
to the authentication routes, which needs the handlers in IAM-01.

**Done when**
- [x] The limiter counts per key with a moving window, and forgets keys it no longer needs.
- [x] The limiter cannot distinguish a registered address from an unknown one, because it never looks either up.
- [x] An empty key is refused rather than collapsing every anonymous caller into one bucket.
- [x] An unusable rule fails at construction rather than locking out every user at runtime.
- [x] Authentication endpoints are limited per address and per network, not per address alone, since one attacker with many addresses is the ordinary case.
- [x] A limited response is `429` with `Retry-After`, per ADR-0004.
- [x] Limits are configuration rather than constants, so they can be tightened during an incident.
- [x] Two instances share one count, so the limit does not multiply by the task count.
- [x] Concurrent attempts are counted exactly, using one atomic statement rather than a read followed by a write.
- [x] The limiter fails open when the database is unreachable, and says so, because authentication cannot happen without that database anyway.
- [x] Old windows are swept, since the keys are email and network addresses and a counter nobody will read again is only personal data somebody has to store.

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

**In progress.** The Go scanner runs in the test suite and so already fails the build: it records spans
and log output from real requests and asserts no value matches an address, token, connection string or
password hash. The assertions it must satisfy are listed in
[telemetry-conventions.md](../../operations/telemetry-conventions.md) so the web and Python
implementations inherit them rather than reimplementing them.

**Remaining.** Scanning what the collector actually received, rather than what the process produced, and
covering third-party error reporting once one is chosen.

**Spec** [observability.md](../../operations/observability.md) · [telemetry-conventions.md](../../operations/telemetry-conventions.md)

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
