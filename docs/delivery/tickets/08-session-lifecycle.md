# Epic SES — Session lifecycle and orchestration

**Phase 2–3** · **Workstream** Go, Platform, Web

The session aggregate and its state machine. Every mutation carries tenant, actor, purpose, idempotency
key, expected version and correlation identifier. Retryable failures stay visible as workflow state
rather than being rewritten into a terminal one.

---

### SES-01 · Implement the session aggregate and its state machine

**Depends on** CAT-02, PLT-03 · **Blocks** SES-02, RTC-01

Draft, composing, ready, connecting, in progress, reconnecting, finalizing, evaluating, review ready,
archived, plus the exceptional terminals: cancelled, expired, composition failed, interrupted,
finalization failed, evaluation failed.

**Done when**
- [ ] Invalid transitions are rejected with a stable error code, not a 500.
- [ ] Optimistic concurrency rejects a stale write rather than silently overwriting.
- [ ] Every transition records authorised actor, emitted events, and audit entry.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

---

### SES-02 · Implement session start with quota reservation and scoped realtime authorization

**Depends on** SES-01, TEN-08, DEC-06 · **Blocks** RTC-01

Start checks readiness, expiry, authorization and quota, reserves capacity, and issues short-lived
provider credentials scoped to this session and this attempt.

**Done when**
- [ ] Start is refused for an expired, unauthorized, already-started or over-quota session, each distinctly.
- [ ] Realtime credentials are short-lived, session-scoped and non-reusable.
- [ ] A quota change after start never interrupts an interview already running.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

---

### SES-03 · Build the prepare screen with device checks and a blocking consent gate

**Depends on** SES-01, WEB-04, CAT-05 · **Blocks** SES-02

*Implemented in the prototype; carry it into production.* Brief, interviewer preview, microphone,
speaker, connection and browser checks, accommodations, and consent that actually blocks start.

**Done when**
- [x] Start is disabled until the microphone check passes and required consent is given.
- [x] The blocked state names which requirement is missing and moves focus to it.
- [x] Optional model-improvement consent is separate, off by default, and never bundled.

**Done, holding the gate for SES-02.** /session/[id]/prepare carries the prototype whole: the
brief and interviewer from the session's own config named through the catalogue, four device
checks (microphone and browser required, speaker and connection recommended and never gating),
the accessibility switches, the consent card and the gated start. The gate's rules are pure and
pinned: an unsupported browser outranks everything because nothing else can be fixed in one, a
failed microphone reads differently from an unrun one, and consent blocks last so the choice
gets full attention. "Take me to what is missing" moves focus to the one named problem - the
mic's test button or the consent checkbox - and the fix-me hand hides when the browser itself is
the problem, exactly as the prototype decides it.

The microphone check opens the microphone, measures level and closes it; no audio leaves the
browser, and the page says nothing is recorded until start wherever it matters. The optional
model-improvement consent is its own checkbox, off, with the required consent proven not to drag
it along and the optional one alone proven to open nothing. The accessibility switches arrive
pre-set from the profile - captions and extended thinking time honoured by default, PRO-01's
promise kept for the prepare half. A composing session says so and polls itself to readiness; a
failed composition is an honest error with its taxonomy code, not a dead page. Pressing start is
SES-02's: this screen readies everything, holds the gate, and will hand SES-02 the session
toggles and consents it collected.

**Spec** [practice-mode.md](../../product/practice-mode.md) · [responsible-hiring.md](../../security/responsible-hiring.md)

---

### SES-04 · Implement idempotent completion and transcript sealing

**Depends on** SES-01, PLT-06, RTC-04 · **Blocks** EVL-01

Accept the final cursor, reject later conversational events, reconcile sequence gaps, seal the
transcript, await media for a bounded period, then persist evaluation input digests.

**Done when**
- [ ] Completion is idempotent; a duplicate request produces the same receipt and no second evaluation.
- [ ] Sequence gaps are recorded rather than silently closed.
- [ ] Missing optional media continues to evaluation with an explicit warning attached.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

---

### SES-05 · Implement active-time accounting and timing policy

**Depends on** SES-01 · **Blocks** SES-06

Duration counts active interview time. Connecting, reconnecting, finalization and evaluation do not
count against the candidate, and accommodations may change pacing without changing anchors.

**Done when**
- [ ] Reconnection provably does not consume candidate time.
- [ ] Grace and maximum duration are versioned policy values, not constants in the client.
- [ ] Response latency is excluded from every scoring path, verified by test.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

---

### SES-06 · Implement reconnection, grace expiry and interruption recording

**Depends on** SES-05, RTC-03, DEC-14 · **Blocks** SCR-08

A dropped connection resumes where it left off, or expires into finalization with the interruption
recorded as coverage rather than as poor performance.

**Done when**
- [ ] Resume restores the conversational cursor, the timer snapshot and the phase.
- [ ] Grace expiry finalizes cleanly and records why.
- [ ] The interruption appears in evidence as reduced coverage, never as a low score.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md) · [screen-mode.md](../../product/screen-mode.md)

---

### SES-07 · Build session history with complete lifecycle states

**Depends on** SES-01, WEB-04 · **Blocks** nothing

*Gap found against the prototype: the session list uses an "abandoned" state that is not in the state
machine, and shows no expired, cancelled, composition-failed or archived sessions.*

**Done when**
- [ ] Every lifecycle state the machine can reach is representable in the list.
- [ ] Each state offers the action that actually applies to it.
- [ ] Filters persist in the URL and survive a refresh.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md) · [information-architecture.md](../../product/information-architecture.md)

---

### SES-08 · Build the completion receipt and processing status screen

**Depends on** SES-04, WEB-04 · **Blocks** nothing

A durable receipt and honest processing stages, distinguishing queued, delayed, partial, recoverable
failure, terminal failure, insufficient evidence and complete — without promising a completion time.

**Done when**
- [ ] The receipt survives navigating away and returning later.
- [ ] Delayed and failed are distinguishable, and failure states name a next action.
- [ ] Screening candidates see confirmation and permitted status only.

**Spec** [user-journeys.md](../../product/user-journeys.md)
