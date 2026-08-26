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
- [ ] Duplicate and out-of-order events are handled without corrupting session state.
- [ ] A stale epoch cannot mutate a session that has already moved on.
- [ ] Replay from a cursor produces the same client state.

**Spec** [realtime-protocol.md](../../architecture/realtime-protocol.md)

---

### RTC-03 · Implement reconnection and recovery in the browser

**Depends on** RTC-02, SES-06 · **Blocks** RTC-06

Refresh, duplicate tab, sleep and wake, network handover, device removal and stale credentials — each
recovered into the same session rather than into a new one.

**Done when**
- [ ] All six interruptions are tested and recover into the same session.
- [ ] A duplicate tab is detected and refused rather than producing two live connections.
- [ ] The reconnection overlay announces state changes to assistive technology.

**Spec** [realtime-protocol.md](../../architecture/realtime-protocol.md)

---

### RTC-04 · Implement transcript capture, correction and provenance

**Depends on** RTC-02 · **Blocks** SES-04, EVL-01

Partial transcripts arrive, get corrected, and both versions retain provenance and sequence so evidence
can always be traced to what was actually said.

**Done when**
- [ ] Original and corrected text are both retained with their sequence and timing.
- [ ] Word-level timing is preserved for the alignment ART-01 depends on.
- [ ] Transcript confidence is carried per segment, not just per session.

**Spec** [realtime-protocol.md](../../architecture/realtime-protocol.md)

---

### RTC-05 · Implement media upload, finalization and reconciliation

**Depends on** PLT-05, RTC-01, DEC-07 · **Blocks** ART-01

Chunked upload during the session, finalization at the end, and reconciliation that proves the stored
object matches what the client sent.

**Done when**
- [ ] Upload survives a network interruption and resumes.
- [ ] Finalization verifies the manifest and digest before the session is marked complete.
- [ ] A failed or partial upload marks media as missing rather than pretending it arrived.

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
