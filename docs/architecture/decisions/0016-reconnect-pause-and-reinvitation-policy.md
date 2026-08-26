# ADR-0016: Reconnect grace, pause, restart and re-invitation

**Status:** Accepted (practice); screening clauses provisional pending DEC-11  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-26  
**Review date:** 2027-02-26  
**Supersedes:** None  
**Superseded by:** None

Closes DEC-14 for practice mode and pre-agrees the screening values. The
screening clauses take effect only when DEC-11's jurisdictional
determination lands; if that determination contradicts them, it wins and
this ADR is revised.

## Context

SES-05 shipped the mechanism: timing rules are versioned immutable rows in
`interview.timing_policies`, stamped on each session at start
first-write-wins, served to the client in the start response, and active
time is summed per connection epoch so reconnection can never bill the
candidate. What remained was the policy: the numbers, who may pause, what
expiry does, and who may re-invite.

## Decision

### The numbers (timing policy v1)

- **Reconnect grace: 120 seconds.** Long enough for a network flap, a
  browser refresh and a rejoin; short enough that an interviewer-shaped
  product does not leave a candidate staring at a dead room. Matches the
  2-minute join window ADR-0012 set, so the system has one notion of "how
  long we wait for you".
- **Maximum overrun: 300 seconds** past the configured interview length.
  Bounds a closing answer without letting a session run open-ended.

Both are policy rows, not constants; changing either is a new version and
touches no running session.

### Pause

- **Practice: candidate-initiated pause is allowed.** Practice exists for
  the candidate; their kitchen timer going off is not a failure mode.
  Pause suspends active time by the same epoch arithmetic as
  reconnection.
- **Screening: no candidate-initiated pause.** A pausable assessment is a
  different assessment; fairness across candidates requires the same
  clock. Interruptions are handled by reconnect-within-grace, and an
  accommodation that needs different pacing is granted before the session
  as configuration, per the accommodations line in
  [session-lifecycle.md](../session-lifecycle.md), not improvised during
  it.

### Grace expiry and restart

Grace expiry finalizes the session as **interrupted, with the reason
recorded. There is no silent restart, ever.** The sealed partial evidence
stands, and the interruption reaches evaluation as reduced coverage,
which EVL-03 renders as `not_discussed` competencies, never as a low
band. SES-05's active-time accounting already guarantees the dead clock
is not billed.

### Re-invitation (screening, provisional)

- Re-invitation is **recruiter-authorized**, per candidate, per campaign.
  The platform never re-invites on its own initiative.
- A re-invitation is **always a new session** with its own attempt
  history. The interrupted session's evidence is retained, never merged
  and never overwritten; reviewers see both, labelled.
- ADR-0014's billing stance applies unchanged: a platform-fault
  interruption is credited, so re-inviting after one costs the tenant
  nothing.

## Consequences

- SES-06 can build against fixed semantics; RTC-03's recovery chain
  inherits one grace number from the policy row it already receives.
- The practice/screening pause asymmetry must be visible in copy before a
  screening candidate starts, which lands with SCR's disclosure surfaces.

## Revisit when

DEC-11's determination lands (screening clauses); support data shows 120
seconds strands real candidates on real networks; or accommodations
require a per-session grace override, which would become a policy field,
not an exception.
