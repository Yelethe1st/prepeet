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
- [x] Invalid transitions are rejected with a stable error code, not a 500.
- [x] Optimistic concurrency rejects a stale write rather than silently overwriting.
- [x] Every transition records authorised actor, emitted events, and audit entry.

**Done.** The machine holds the spec's exact edge set - every edge in the diagram allowed, every
edge outside it refused, self-transitions refused, terminals terminal except the authorized
retries - and an invalid transition is a typed TransitionError carrying SESSION_TRANSITION_INVALID,
proven against the real store. Optimistic concurrency is the schema's own version guard: a stale
write is ErrStaleVersion and the world it tried to overwrite is proven unchanged. The third box
is walked edge by edge: each transition writes exactly one audit row in its own transaction,
the row carries the acting user AND whether a person or automation carried their authority
(actor_type survives the distinction), catalogued events publish in the same transaction, and a
transition the catalogue defines no event for audits without inventing one. The restart proof
holds the exactly-once shape across a worker death.

One boundary note: no HTTP endpoint issues user-driven transitions yet - creation transitions
internally and the workflow transitions as a service. When SES-02's start and the cancel
endpoints land, TransitionError maps to its stable code at the wire the way every other typed
refusal here does.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

---

### SES-02 · Implement session start with quota reservation and scoped realtime authorization

**Depends on** SES-01, TEN-08, DEC-06 · **Blocks** RTC-01

Start checks readiness, expiry, authorization and quota, reserves capacity, and issues short-lived
provider credentials scoped to this session and this attempt.

**Done when**
- [x] Start is refused for an expired, unauthorized, already-started or over-quota session, each distinctly.
- [x] Realtime credentials are short-lived, session-scoped and non-reusable.
- [x] A quota change after start never interrupts an interview already running.

**Done.** POST /interviews/{id}/start runs one ordered command: quota reserved first (screening
only; practice has no tenant quota per ADR-0014), the ready-to-connecting transition second, the
room grant minted last, so a refusal happens before anything spends, a token never exists for a
session that did not move, and a crash between reserve and transition converges on the retried
start through the ledger's already-metered answer rather than double-billing. Each refusal is
its own code on the wire - SESSION_EXPIRED, SESSION_ALREADY_STARTED, SESSION_NOT_READY,
QUOTA_EXHAUSTED, all 409 - and unauthorized is this product's not-found, because existence is
not answered across owners. Six concurrent starts on one session admit exactly one through the
machine's version guard.

The grant is ADR-0012's, minted by platform/realtime as a stdlib HS256 JWT whose claims are
verified from the outside in tests: one room (the session id), one identity, roomJoin and
nothing wider - no create, no admin, no list - with a two-minute join window. Non-reusable
across attempts because reconnection mints a fresh grant through its own flow (RTC-03), and the
refused quota path leaves the session ready, so quota arriving later makes it startable without
repair. The third box is structure plus proof: nothing after start consults the ledger, and the
test collapses the quota behind a running interview and watches its transitions keep working.

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
- [x] Completion is idempotent; a duplicate request produces the same receipt and no second evaluation.
- [x] Sequence gaps are recorded rather than silently closed.
- [x] Missing optional media continues to evaluation with an explicit warning attached.

**Done.** Completion freezes the record as one immutable seal per session: the final cursor, the
gaps standing under it, the digest of the effective transcript (deterministic from the log
alone), the bundle digest it ran under, and the media status with its warnings. Idempotency is
the seal's primary key: a duplicate completion converges on the row and answers the identical
receipt - five concurrent completions proven to agree - with exactly one session_completed event
and no second pass through the states; a different final cursor is SEAL_CONFLICT, not a retry.

Gaps are the second box's words exactly: recorded, never closed - the missing sequence appears
in the receipt as its precise range plus a SEQUENCE_GAPS_RECORDED warning, and evaluation will
read it as coverage. Media follows the session's own recorded choice: transcript-only completes
as none_by_choice with no warning, because reporting a person's own decision back to them as a
warning would be noise; an audio session with nothing produced completes to evaluating with
MEDIA_MISSING attached and nothing blocked. The bounded media wait becomes a real timer when
RTC-05's egress exists to wait for; today the status is known at the seal, which is what
evaluating's entry condition requires.

The seal also ends the conversation: ingest refuses conversational events after it
(EVENT_AFTER_SEAL) while a leave or connection event still lands, because a goodbye is not
testimony, and the sealed digest is proven untouched by the attempt. POST
/interviews/{id}/complete serves the receipt.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

---

### SES-05 · Implement active-time accounting and timing policy

**Depends on** SES-01 · **Blocks** SES-06

Duration counts active interview time. Connecting, reconnecting, finalization and evaluation do not
count against the candidate, and accommodations may change pacing without changing anchors.

**Done when**
- [x] Reconnection provably does not consume candidate time.
- [x] Grace and maximum duration are versioned policy values, not constants in the client.
- [x] Response latency is excluded from every scoring path, verified by test.

**Done.** Duration is now active time by arithmetic over the durable timeline: per connection
epoch, the span from its first conversational segment to its last; in total, the sum. The
room-clock gap a reconnection leaves between epochs cannot appear in the sum by construction,
and the end-to-end proof completes a session whose two epochs bracket ten dead minutes and reads
180 seconds off the completed event, not 780. That proof also caught its own fixture being
silently refused at ingest (final segments require word timing), so it now asserts every ack.

Timing rules live in interview.timing_policies: versioned immutable rows (new values are new
versions, the trigger refuses edits), stamped on the session at start first-write-wins so a
policy published mid-session never changes a running one - the same reconstructability the
rubric pin gives evaluation. The start response carries the stamped values, so the client
compiles in no grace constant. Seeded v1 (120s grace, 300s overrun) is explicitly marked
pending DEC-14; enforcement of grace expiry is SES-06's.

Latency exclusion is proven on both scoring paths: the Python extractor reads the same words
identically after a 90-second pause (only provenance clocks shift), and Go aggregation is
DeepEqual-identical across spans differing only in clock values. Timing is provenance for
replay, never an input to a score.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

---

### SES-06 · Implement reconnection, grace expiry and interruption recording

**Depends on** SES-05, RTC-03, DEC-14 · **Blocks** SCR-08

A dropped connection resumes where it left off, or expires into finalization with the interruption
recorded as coverage rather than as poor performance.

**Done when**
- [x] Resume restores the conversational cursor, the timer snapshot and the phase.
- [x] Grace expiry finalizes cleanly and records why.
- [x] The interruption appears in evidence as reduced coverage, never as a low score.

**Done.** The third box is proven at the rule that decides it: aggregation over a
truncated record keeps the earned band on everything answered before the drop, and lands
everything the drop cut off in unassessed with NOT_DISCUSSED and no band or confidence,
named in coverage's NotReached - a reviewer sees an interview that stopped, never a
candidate who failed. The expiry path composes into that by construction: the seal
carries the record only as far as it truly went, and no code path converts absence into
a score. The drop is the server's state: an
accepted connection.lost folds a running session into reconnecting and publishes the catalogue's
session_interrupted announcement, and recovery - connection.resumed for a same-epoch blip,
connection.established for a fresh attempt - folds it back, so the machine follows the timeline
rather than any tab's memory. POST /interviews/{id}/resume opens the next epoch and answers the
recovery cursor (what the superseded epoch durably holds, the exact gaps owed), a fresh room
grant, and the timing policy stamped at start, read by version so a policy published mid-session
never moves a window the candidate is inside; the superseded tab's next batch refuses whole with
EPOCH_STALE. Expiry is a Temporal timer per drop, started from the session_interrupted event
("grace-{session}-{attempt}", REJECT_DUPLICATE): when the stamped window lapses without a
resume, the interruption is recorded with grace_expired as its cause and how long the candidate
had been gone, and the session seals through the same completion path a candidate's own complete
runs, announced as expired rather than completed. What remains is proving the sealed-short
record reaches evaluation as coverage not reached rather than as a low score, which is an
assertion against the evaluation pipeline, and the browser recovery chain itself, which is
RTC-03's.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md) · [screen-mode.md](../../product/screen-mode.md)

---

### SES-07 · Build session history with complete lifecycle states

**Depends on** SES-01, WEB-04 · **Blocks** nothing

*Gap found against the prototype: the session list uses an "abandoned" state that is not in the state
machine, and shows no expired, cancelled, composition-failed or archived sessions.*

**Done when**
- [x] Every lifecycle state the machine can reach is representable in the list.
- [x] Each state offers the action that actually applies to it.
- [x] Filters persist in the URL and survive a refresh.

**Built at /practice, replacing the placeholder, with the recorded prototype gap made
structural.** The state vocabulary is data (states.ts): all sixteen states from the schema's own
CHECK, each with its candidate-facing label, filter group and the one action that applies -
rejoin through prepare for the live family, the processing page for the sealing and failed
family, results for the finished, the wizard for the dead ends, with failure codes shown inline.
The completeness test walks the machine's list against the vocabulary, so an invented
"abandoned" state or a hidden expired row - the gap the ticket recorded - fails the build. An
unknown future state renders as itself with the safest action rather than crashing history.

The server side is GET /me/sessions: every session the scope can see, newest first, failed and
expired rows included on purpose. Filters live in the URL (?filter=), read on load and written
on change, so refresh and shared links keep them; the empty history offers the wizard rather
than a blank table.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md) · [information-architecture.md](../../product/information-architecture.md)

---

### SES-08 · Build the completion receipt and processing status screen

**Depends on** SES-04, WEB-04 · **Blocks** nothing

A durable receipt and honest processing stages, distinguishing queued, delayed, partial, recoverable
failure, terminal failure, insufficient evidence and complete — without promising a completion time.

**Done when**
- [x] The receipt survives navigating away and returning later.
- [x] Delayed and failed are distinguishable, and failure states name a next action.
- [x] Screening candidates see confirmation and permitted status only.

**Built at /session/[id]/complete, durable by construction.** The session GET now carries the
timeline cursor and, once sealed, the seal itself (sealed_at, media status, warnings), so the
receipt is the server's own durable read: returning later answers the same, and there is no
client state worth keeping. Ending the interview now actually completes it - the live shell
reads the cursor, seals at it (idempotent to the receipt), and lands here; a session that never
established has nothing to seal and the page says the session is still live rather than
inventing a receipt.

Processing shows the state machine's own stages and, as a recorded deviation from the
prototype's "usually takes under a minute", promises no completion time - a test forbids the
copy. Delayed is a polling status region; failure is an alert naming the code, stating that the
transcript and evidence are safe, and giving the concrete next action (the session reference to
quote). The media line reads the choice as a choice: none_by_choice in the candidate's own
terms, missing stated plainly with what still works. The third box is structural today - the
API's practice-only enum means no screening candidate can reach any session surface - and the
screening confirmation view lands with the SCR epic behind DEC-11.

**Spec** [user-journeys.md](../../product/user-journeys.md)
