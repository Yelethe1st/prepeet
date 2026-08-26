# ADR-0014: A started session is the billing unit; the limit blocks starts, never interviews

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-26  
**Review date:** 2027-02-26  
**Supersedes:** None  
**Superseded by:** None

Closes DEC-16: the billing unit, the awkward cases, quota behaviour at the
limit, and what a candidate is told. Price points and currency are
deliberately not decided here; this ADR fixes the unit and the behaviour,
which is what TEN-08 and SES-02 need to build against.

## Context

[cost-and-capacity-model.md](../../operations/cost-and-capacity-model.md)
requires immutable usage entries attributed per tenant, session, mode and
capability, a soft warning before any hard limit, in-flight interviews
completing safely after quota exhaustion, and candidate-facing messaging
that never exposes employer billing. Unit economics are tracked per
created, started, completed and insufficient-evidence session separately,
with the instruction not to optimise by hiding unsuccessful journeys. The
undecided part was which of those counts is the invoice.

## Decision

### The unit: a started session

**A session is billable when it first enters `in_progress`**: the moment
recording begins and provider spend starts, which is the moment the prepare
screen's gate opens into a live interview. One session bills once,
whatever happens after, keyed by session id so retries and reconnects
cannot double-bill.

- **Creation is free.** The wizard, composition and the prepare screen are
  exploration; billing creation would tax configuration and punish the
  candidate who walks up to the gate and decides not to start. Composition
  cost at volume is a platform cost of sale, tracked in the usage ledger
  but not invoiced per unit.
- **Completion is not the unit.** Billing only completed sessions makes an
  abandoned interview free to farm: most of the provider spend (realtime
  minutes, transcription, the interviewer loop) is incurred whether or not
  the candidate reaches the end.

### The awkward cases, decided

- **Insufficient evidence: billable.** The interview ran, the spend was
  incurred, and the spec forbids hiding unsuccessful journeys. An
  insufficient-evidence outcome is a fact about what was observable, not a
  defect to refund by default.
- **Abandoned early: not billable inside the first minute.** A session
  that ends within sixty seconds of starting - a wrong click, immediate
  device failure, cold feet at the first question - is recorded in usage
  but not invoiced. Past that, an abandoned session is a started session.
- **Interrupted by us: never billable.** ADR-0012 already promises it: a
  session that ends `interrupted` because our transport or agent failed is
  excluded from the invoice however long it ran. The usage entry records
  it with its reason, because the cost model tracks failure honestly even
  when nobody is charged for it.

### At the limit: block new starts, after warning, never mid-interview

- **Soft warning first.** Crossing a configured threshold (80 percent by
  default) surfaces to tenant administrators in usage and quota views and
  through notification. Nothing is blocked at the warning.
- **At the limit, new starts are refused.** The refusal happens at
  SES-02's start, before recording, before provider spend, with a stable
  code. Creation and composition still work: a configured session simply
  waits at the prepare gate until quota exists, which keeps the blocked
  state recoverable rather than destructive.
- **In-flight interviews always complete.** Quota is reserved at start,
  so a session that began under quota finishes whatever the counter says
  afterwards. A quota event during an interview changes nothing the
  candidate can see; the cost model's rule is honoured structurally by
  reserving before starting rather than checking during.
- **Overage billing is deferred, not decided against.** Block-at-limit is
  the launch behaviour because it is the only one that cannot surprise a
  tenant's finance team. A tenant-configurable overage allowance is a
  future commercial option; nothing in the unit or the ledger forecloses
  it.

### What the candidate is told

A candidate refused at start because the workspace is at its limit sees
that the workspace is at capacity and that the hiring team has been told -
in those words, with no seat counts, no prices and no billing vocabulary,
per the cost model's rule that new-start messaging never exposes employer
billing. Practice mode has no tenant and no quota in this sense; personal
fair-use limits, if ever needed, are a separate decision.

## Consequences

- TEN-08 builds the usage ledger (immutable entries keyed by session and
  attributed per the cost model), the quota configuration, the warning
  threshold and the two read endpoints; SES-02 calls reserve-at-start and
  refuses with the stable code when reservation fails.
- The first `in_progress` transition is the metering event, which the
  session state machine already makes exactly-once under its version
  guard: the billing unit inherits the machine's own discipline.
- The sixty-second early-abandon window and the 80 percent warning
  threshold are configuration with these defaults, not constants.
- Price points and currency remain open; when they are set, they attach to
  the unit defined here without touching the ledger's shape.

## Alternatives considered

- **Per-minute billing** (rejected for launch: bills the candidate's
  thinking time, makes extended-time accommodation a cost event, and turns
  the invoice into an argument; minutes stay in the ledger for cost
  attribution).
- **Billing created sessions** (taxes exploration; punishes composition
  failures that are ours).
- **Billing completed sessions** (makes abandonment free to farm while the
  spend is real).
- **Overage-by-default at the limit** (a finance surprise; deferred as a
  configurable option instead).
