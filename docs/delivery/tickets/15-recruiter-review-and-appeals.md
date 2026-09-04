# Epic REV — Recruiter review, decisions and appeals

**Phase 5–6** · **Workstream** Web, Go, Security/privacy

Evidence first, decision second, and the decision belongs to a named person. Nothing here advances or
rejects a candidate automatically, and every sensitive read is audited.

---

### REV-01 · Build the candidate roster with campaign-scoped access

**Depends on** SCR-01, IAM-06 · **Blocks** REV-02

Candidates for the campaigns a reviewer is scoped to, with filters, states and pending-review counts —
never a bare leaderboard.

**Done when**
- [x] The roster is filtered server-side by campaign scope, not hidden in the browser.
- [x] Insufficient-evidence candidates are shown as such, not sorted to the bottom as low scorers.
- [x] No ranking or sort presents candidates as ordered by quality.

**Done, and the first recruiter surface in the product.** GET /campaigns/{id}/candidates
composes in cmd, the one place recruiting, interview and evaluation may meet: recruiting's
list reads back the candidate each acceptance resolved to (its own recorded join key),
interview answers a campaign's sessions as narrow rows under the tenant's scope, and
evaluation is asked one question about one reviewable session, could the record support
assessment at all. Access is the campaign join, answered as absence across campaigns; the
standing filter is a query parameter the server serves; and the pending-review count includes
the insufficient-evidence screenings, because absence of evidence is still a decision a
person has to make. On the web, /campaigns and /campaigns/{id} land behind the navigation
entries that already pointed at them, and the roster deliberately drops the one thing the
prototype offered that the ticket forbids: the sortable competency-signal column. Rows keep
invitation recency, no header sorts, no row carries a band, score, ratio or confidence, the
insufficient-evidence standing is words in place, and both the wire tests and the screen
tests pin each of those by name. What the row deliberately does not carry - coverage numbers,
confidence, evidence links - is REV-02's audited evidence screen, behind evaluation.read.

**Spec** [screen-mode.md](../../product/screen-mode.md)

---

### REV-02 · Build the evidence-first candidate review screen

**Depends on** EVL-05, EVL-06, REV-01 · **Blocks** REV-03

Pinned configuration and artifact versions, evidence summary framed as evidence rather than a verdict,
competencies with anchors and sufficiency, transcript spans, audio, coverage, contradictions, unverified
claims, job-requirement evidence, missing evidence and suggested follow-ups.

**Done when**
- [ ] Every material conclusion links to the evidence behind it.
- [ ] Uncertainty and coverage appear beside every score, not in a footnote.
- [ ] The screen states that the decision belongs to the reviewer, and never presents a recommendation as one.

**Spec** [screen-mode.md](../../product/screen-mode.md)

---

### REV-03 · Implement human decisions with override rationale and append-only history

**Depends on** REV-02, IAM-04 · **Blocks** REV-04

Advance, hold, decline or request re-review, each with a named actor and a reason, and an override
rationale required when the reviewer disagrees with the suggested band.

**Done when**
- [ ] No code path can produce an outcome without a named human actor.
- [ ] Override requires a rationale and records the band it disagreed with.
- [ ] Decision history is append-only and preserves the true actor and evidence version.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md)

---

### REV-04 · Audit sensitive transcript, audio and evaluation reads

**Depends on** REV-02, OPS-06 · **Blocks** REL-03

Every open of a transcript, recording or evaluation is recorded against a named person and is visible to
the tenant.

**Done when**
- [x] Reads are audited even when nothing is changed.
- [ ] The tenant can see who accessed a candidate's evidence and when.
- [ ] Unusual access volume raises an alert rather than sitting in a log nobody reads.

**One of three, built ahead of the epic because the mechanism is easier to add now than to retrofit
under the screens that will depend on it.**

Reading a transcript is an event rather than a query. The obligation is declared in the API contract,
`x-prepeet-sensitive-read` on the operation, rather than decided in a handler: a handler choosing for
itself would be a second source of truth, and the copy that drifts is the one nobody re-reads. When
REV-02's evidence screen and the audio endpoint arrive they are audited by declaring themselves,
which is the whole reason this landed early.

The recording sits between the handler and the response, deliberately. Recording first would log
reads that never happened and could not say how they ended; recording after the response would let
restricted content reach somebody with no record of it, which is the one outcome the obligation
exists to prevent. In between, the handler has an answer and nothing has been sent, so **a failure to
record refuses the read** rather than serving it unrecorded. An advisory audit is missing exactly when
somebody is doing something they should not.

A refusal is recorded as such. Somebody signed in reaching for a transcript that is not theirs is the
attempt a reviewer searches this table for, and recording only successes would hide it.

**A request with no resolvable session is deliberately not recorded here.** It never reached the
content, the request log and the rate limiter already describe it, and a row naming no actor cannot be
admitted by any policy on `audit.events` without one that decides nothing about who is asking.
PostgreSQL ORs permissive policies, so such a policy re-opens the table however well the others are
written. The first attempt at this added exactly that policy and `internal/isolation` refused it,
which is the gate SEC-02 built doing its job on work written after it.

The remaining two boxes are somebody else's. Tenant visibility is OPS-06's audit viewer, which does
not exist, and the alert on unusual volume needs the monitoring OPS-02 has not built. The rows are
being written and nothing reads them yet, which is worth saying plainly.

**Spec** [observability.md](../../operations/observability.md)

---

### REV-05 · Implement constrained candidate comparison

**Depends on** DEC-17, REV-02 · **Blocks** nothing

Only if DEC-17 approves it. Two to four candidates, same role, comparable rubric, uncertainty and
coverage shown, indistinguishable differences stated, no ranking, and individual review required first.

**Done when**
- [ ] Every constraint is enforced server-side, not by the interface.
- [ ] Comparison is unavailable until each candidate in it has been individually reviewed.
- [ ] The feature is behind a flag that is off unless a tenant is explicitly approved.

**Spec** [screen-mode.md](../../product/screen-mode.md) · [responsible-hiring.md](../../security/responsible-hiring.md)

---

### REV-06 · Build the appeals and re-review workflow

**Depends on** DEC-11, REV-03 · **Blocks** REV-07

Eligibility, request reason, frozen original inputs, assigned reviewer, independence where required,
SLA, outcome, permitted disclosure and append-only history.

**Done when**
- [ ] The original evidence and configuration are frozen at the moment the appeal is raised.
- [ ] Independent assignment is enforced where policy requires it — the original reviewer cannot self-review.
- [ ] Outcome, rationale and disclosure are recorded and delivered to the permitted parties.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md)

---

### REV-07 · Build the candidate-facing appeal request and status

**Depends on** REV-06, DEC-11 · **Blocks** nothing

*Gap found against the prototype: appeals exist only as a recruiter queue. A candidate has no way to
raise one or to see what happened to it.*

**Done when**
- [ ] An eligible candidate can raise an appeal and see its status, owner and SLA.
- [ ] Eligibility rules are explained rather than expressed as a missing button.
- [ ] The outcome is disclosed to the candidate to the extent DEC-11 permits.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md) · [product-requirements.md](../../product/product-requirements.md)

---

### REV-08 · Implement system-flagged low-confidence review

**Depends on** EVL-05, REV-06 · **Blocks** nothing

Some evaluations should reach a human without a candidate having to ask — low confidence, low coverage,
a short session, or an assessability warning.

**Done when**
- [ ] Flag criteria are configurable policy, not thresholds hardcoded in a handler.
- [ ] A flagged evaluation cannot be actioned without acknowledging the flag.
- [ ] Flag rates are monitored as a fairness signal in QUA-05.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)
