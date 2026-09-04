# Epic RTC — Realtime, media and transcript integrity

**Phase 2–4** · **Workstream** Web, Go, Platform

The riskiest technical surface in the product, and the reason the walking skeleton exists in Phase 2.
Browser-direct WebRTC, an explicit event protocol with epochs and cursors, and media that either
arrives intact or is honestly marked as missing.

---

### RTC-01 · Implement browser-to-provider WebRTC connection and teardown

**Depends on** DEC-06, SES-02 · **Blocks** RTC-02

Negotiation, connection, device selection, and a teardown that releases the microphone every time,
including when the tab is closed mid-answer.

**Done when**
- [ ] Connection succeeds across the supported browser and device matrix.
- [x] Teardown always releases the microphone, on navigation, close and error alike.
- [x] Failure to connect degrades to a named error with recovery steps, never a spinner.

**The shell is in; the matrix run remains.** LiveKit joined the local stack (single node per
ADR-0006's trigger table, keys agreeing with the api's signer, node-ip pinned to loopback so ICE
candidates point somewhere a local browser can reach). The prepare screen's start button is now
real: POST start, stash the one-use grant, navigate to /session/[id], where the connection shell
joins the room and opens the microphone.

The second box is one idempotent teardown that every exit funnels into - the end button,
component unmount, the pagehide handler for a tab closed mid-answer, connect failure, microphone
refusal, and the server dropping us - each pinned by a test that constructs a way for cleanup to
be forgotten and asserts it was not. A connection nobody can speak into is torn down rather than
left looking alive. The third box: a missing or expired pass, an unauthorized token, a refused
microphone and an unreachable SFU are each a named explanation with their own recovery steps and
a way back to the prepare screen, and "nothing was recorded" said where it is true. The grant
hand-off is consume-on-read so a stale URL cannot silently reuse a token, with a per-page-load
memory so development StrictMode's double effects cannot eat the pass.

The first box stays open honestly: it is a browser-and-device matrix run, which belongs to the
e2e harness against the real stack, not to jsdom. The interview surface itself - the agent in
the room, captions, the protocol - is RTC-02 onward on top of this shell.

**Spec** [realtime-protocol.md](../../architecture/realtime-protocol.md)

---

### RTC-02 · Implement the control event protocol with epochs and cursors

**Depends on** RTC-01, CTR-03 · **Blocks** RTC-03

Ordered, deduplicated, resumable events carrying a connection epoch and a conversational cursor, so a
reconnect can prove what the candidate already heard and said.

**Done when**
- [x] Duplicate and out-of-order events are handled without corrupting session state.
- [x] A stale epoch cannot mutate a session that has already moved on.
- [x] Replay from a cursor produces the same client state.

**Done.** One authoritative timeline per session: start opens epoch one (SES-02's Starter now
begins the attempt), events order by sequence within their epoch in an append-only log with the
session's own dual RLS shape, event ids deduplicate retries, and each durable insert runs in a
savepoint so one duplicate cannot poison the batch's transaction. The acknowledgment answers the
highest contiguous cursor plus the exact gaps still owed, the cursor persists on the session row
so recovery never relies on browser memory, and a different event claiming an occupied sequence
slot is refused as SEQUENCE_CONFLICT rather than resolved: two events in one slot is corruption,
not a race to win.

The proofs are the boxes. A shuffled, gapped, doubled batch lands with the cursor stopped at the
gap and the gap named exactly; the full batch retried converges to all-duplicates with nothing
doubled; filling the gap advances the cursor past it. A takeover opens epoch two, resets the
cursor, and the stale tab's next batch is refused whole with EPOCH_STALE, its events provably
absent from the timeline. Replay from any cursor answers identically twice, in timeline order
whatever order ingestion saw, with ephemeral partials acknowledged but absent because they were
never stored. The first accepted connection.established moves connecting to in_progress through
the machine's own guard - ADR-0014's metering moment. POST and GET /interviews/{id}/events carry
the protocol; the browser's resend buffer and resume flow are RTC-03's, built on the ack and
replay this provides.

**Spec** [realtime-protocol.md](../../architecture/realtime-protocol.md)

---

### RTC-03 · Implement reconnection and recovery in the browser

**Depends on** RTC-02, SES-06 · **Blocks** RTC-06

Refresh, duplicate tab, sleep and wake, network handover, device removal and stale credentials — each
recovered into the same session rather than into a new one.

**Done when**
- [x] All six interruptions are tested and recover into the same session.
- [x] A duplicate tab is detected and refused rather than producing two live connections.
- [x] The reconnection overlay announces state changes to assistive technology.

**Done, at the protocol level; the real-stack matrix rides RTC-06.** The browser's recovery is
three lib pieces and their wiring. The resend buffer (lib/rtc/timeline.ts) holds only what the
server has not confirmed: identity survives retries and takeovers, a refusal is dropped by name
rather than resent forever, and a resume rebases the survivors into the new epoch keeping their
ids so anything that did land converges to a duplicate. The chain (lib/rtc/recovery.ts) makes
one attempt the whole story - resume, rejoin, rebase, then the interruption report and the new
epoch's established event - and any link failing retries the whole chain, because a
half-recovered state would leave the grace timer running toward a finalization the candidate
cannot see. The server's refusals end it by name: GRACE_EXPIRED, EPOCH_STALE and
SESSION_NOT_RESUMABLE each land on their own explanation with a way forward.

The six interruptions map onto that machinery and each is tested recovering into the same
session: refresh and stale credentials resume from the mount path (a missing or expired grant
asks the server rather than bouncing to prepare), sleep/wake recovers on the first chain, a
network handover on a later one, a removed device once it is back, and a duplicate tab asks on
a per-session broadcast channel before joining and refuses on an answer - advisory, with the
server's epoch takeover as the authority. The overlay is the prototype's
(candidate-session-live.html), an alertdialog whose attempt counter is a live region, so each
state change is announced without the dialog re-announcing itself. The tests run against the
protocol with structural fakes for the room and the channel; driving the six through a real
SFU belongs to the live surface build-out, which is why RTC-06 stays open behind this.

**Spec** [realtime-protocol.md](../../architecture/realtime-protocol.md)

---

### RTC-04 · Implement transcript capture, correction and provenance

**Depends on** RTC-02 · **Blocks** SES-04, EVL-01

Partial transcripts arrive, get corrected, and both versions retain provenance and sequence so evidence
can always be traced to what was actually said.

**Done when**
- [x] Original and corrected text are both retained with their sequence and timing.
- [x] Word-level timing is preserved for the alignment ART-01 depends on.
- [x] Transcript confidence is carried per segment, not just per session.

**Done.** Transcript segments are control events with a shape worth retaining, enforced at
ingest: speaker, text, a span on the room's single clock (ADR-0013), confidence in [0,1], and
word-level timing required on every final segment - each word proven to sit inside its segment -
because a transcript row without timing is not a lesser record, it is one that cannot answer
what the product exists to answer. A malformed segment is refused as TRANSCRIPT_INVALID at the
door, never discovered by evaluation later. Corrections must name the sequence they supersede;
their word timing is optional because the original's remains the alignment source.

The read model assembles the timeline into provenance: a correction replaces a segment's
effective text while the original stands underneath with its own sequence, timing and words,
linked both ways; when corrections stack, the latest wins and every earlier version - original
and prior corrections alike - is retained, superseded, and linked forward to what stands now.
The proofs walk it: correct-once shows both versions with exact word timing surviving storage,
correct-twice shows one effective text and three retained versions, per-segment confidence
survives to the read, and a correction whose target never arrived is listed as an orphan rather
than hidden, because a dangling supersession is a resend the client still owes.
GET /interviews/{id}/transcript serves the assembled view with links and orphans intact.

**Spec** [realtime-protocol.md](../../architecture/realtime-protocol.md)

---

### RTC-05 · Implement media upload, finalization and reconciliation

**Depends on** PLT-05, RTC-01, DEC-07 · **Blocks** ART-01

Chunked upload during the session, finalization at the end, and reconciliation that proves the stored
object matches what the client sent.

**Done when**
- [x] Upload survives a network interruption and resumes.
- [x] Finalization verifies the manifest and digest before the session is marked complete.
- [x] A failed or partial upload marks media as missing rather than pretending it arrived.

**Done at the platform layer, per ADR-0013's reinterpretation of the ticket's upload language.**
There is no client upload to resume: recording is server-side SFU egress, so what must survive
interruption is the recording itself. Egress starts from interview.session_started.v1 (published
atomically with the in_progress transition, carrying exactly the fields its contract names) and
is idempotent by the UNIQUE (session, track) row: three starts or a reconnection begin exactly
one egress per track, proven. Transcript-only sessions return before any recorder call, so
durable audio never exists to discard.

Completion stops egress and reconciles BEFORE the seal: each track's artifact is read back from
the object store, its size and sha256 recorded on the track row, and only then does the seal say
finalized. One absent artifact makes the recording missing with the MEDIA_MISSING warning; the
absent track's row says missing while the landed one keeps its digest, and finalized or missing
rows are immutable by trigger. The Recorder and Prober are consumer-defined ports; the
reconciliation semantics are proven against fakes, and the real LiveKit egress client (Twirp
over HTTP with the same stdlib HS256 tokens the grants use, S3 output into the platform bucket)
is wired behind PREPEET_LIVEKIT_API_URL. End-to-end egress against a live room is exercised when
the voice agent lands, since a room with no publishing agent records nothing; the candidate
playback surface (presigned GET) arrives with the results screen's player when ART-01 needs the
same artifacts.

**Spec** [data-architecture.md](../../architecture/data-architecture.md)

---

### RTC-06 · Build the live interview screen

**Depends on** RTC-03, WEB-01 · **Blocks** nothing

*Implemented in the prototype; carry it into production.* Phase, progress, interviewer, speaking state,
microphone state, connection state, timer, captions, caption history, push-to-talk and an explicit,
mode-aware end confirmation — with no live scoring of any kind.

**Done when**
- [ ] Every state is conveyed as text as well as visually, and colour alone carries nothing.
- [ ] Push-to-talk works by pointer and by keyboard, and is announced.
- [ ] No articulation score, filler count or correction appears during an answer.

**Spec** [realtime-protocol.md](../../architecture/realtime-protocol.md) · [practice-mode.md](../../product/practice-mode.md)

---

### RTC-07 · Handle provider degradation and outage during a live interview

**Depends on** RTC-03, DEC-06 · **Blocks** REL-04

The provider slows, drops, or fails mid-answer. Decide and implement what the candidate experiences, and
make sure the session is recoverable or honestly terminated.

**Done when**
- [ ] Degradation is detected and surfaced before the candidate is left talking to nothing.
- [ ] A provider outage produces a recoverable session or a clean interruption record, never silent data loss.
- [ ] The behaviour is exercised in a fault-injection test, not only reasoned about.

**Spec** [realtime-protocol.md](../../architecture/realtime-protocol.md) · [disaster-recovery.md](../../operations/disaster-recovery.md)
