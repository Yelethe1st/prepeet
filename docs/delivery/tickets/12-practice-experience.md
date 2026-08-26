# Epic PRC — Practice results, review and coaching

**Phase 3** · **Workstream** Web, Python

What the candidate actually gets back. Results answers what happened and what evidence supports it;
review answers what to improve; delivery answers how it landed. The split is a hypothesis — PRC-06
exists to test it rather than defend it.

---

### PRC-01 · Build the outcome and evidence screen

**Depends on** EVL-05, WEB-04 · **Blocks** PRC-02

Overall evidence summary, competency results with sufficiency, the evidence behind each score, coverage
and gaps, transcript and synchronised audio replay.

**Done when**
- [x] Every score can be expanded to the exact sentences that produced it.
- [x] Insufficient competencies read as insufficient, never as zero or as a low band.
- [ ] Audio replay jumps to the evidence timestamp and works by keyboard.

**Built at /session/[id]/results, ported from candidate-session-results with two recorded
deviations.** First, no score ring: the prototype shows 74/100, and ADR-0015 forbids numeric
display until QUA-03 calibrates, so the outcome is per-competency bands and confidence labels
with the server's framing sentence, and a test fails if anything numeric-looking appears.
Second, no audio player yet: RTC-05 has not shipped, so replay is an honest absence rather than
the prototype's simulated playhead, which is why the third box stays open. The jump mechanism
that box describes is built and keyboard-proven - every evidence timestamp and both sides of
each contradiction move focus to the exact transcript segment - and the same jump will drive
the recording when it exists.

The server now serves the evidence spans on the results response, so each competency expands to
its exact quoted sentences with kind labels, joined by evidence_ids. Insufficient renders
through the WEB-04 InsufficientEvidenceState with the two reasons kept distinct: "this never
came up" (the plan's problem) versus "insufficient evidence" (thin answers), never band
styling. RESULT_NOT_READY renders as the processing state and polls; failures render the
ErrorState with the request reference. The transcript shows effective text with evidence-carrying
turns marked; the prototype's search, PDF export and share dialog are deferred to their own
tickets rather than shipped as decoration.

**Spec** [practice-mode.md](../../product/practice-mode.md)

---

### PRC-02 · Build the coaching review screen

**Depends on** PRC-01 · **Blocks** PRC-03

Per-answer strengths, gaps, rationale and next action, a fact-preserving model rewrite of each answer,
patterns across sessions, and the drills that follow from them.

**Done when**
- [x] Every coaching statement traces to something the candidate said.
- [x] A rewrite never adds a fact; missing information appears as a placeholder or question.
- [x] Coaching failure leaves the evaluation intact and says so.

**Done at the coaching-1 floor, where fact preservation is structural.** Coaching is derived,
never stored: a pure function (coaching-1) of the sealed input and the stored evidence spans,
served by GET /interviews/{id}/review, so it re-derives identically forever and a model coach
replaces it behind the same shape. Every statement carries the exact quote it is about;
strengths come from supporting spans, gaps from unverified claims, acknowledged limits and
contradictions, each with its own neutral wording. The rewrite is typed parts on the wire -
quote or placeholder - assembled ONLY from the candidate's own sentences plus bracketed
questions, and a strong answer earns silence, never filler.

ValidateCoaching is the gate that outlives the floor: every quote must be an exact substring of
its own candidate turn, and a placeholder must be a bracketed question containing no digits,
because a number inside brackets is a fact wearing them. Attacked three ways (invented sentence,
smuggled fact, foreign quote), each refused wholesale. A gate refusal or a missing input serves
coaching_available false with the note that the evaluation is complete and unaffected - a 200,
never an error - and the screen renders that as its own state linking back to results. The
screen marks placeholders in the DOM (data-part), not just by styling, so a placeholder can
never render as a fact; per-session patterns and drills are PRC-04's with progression.

**Spec** [practice-mode.md](../../product/practice-mode.md)

---

### PRC-03 · Implement answer redo with preserved history

**Depends on** PRC-02, SES-01 · **Blocks** ART-06

One retake per question. The redone answer is scored; both versions stay in the transcript with their
provenance.

**Done when**
- [ ] The original answer, its evidence and its timing all survive.
- [ ] Redo is available only in practice and only where the session configuration allowed it.
- [ ] The transcript makes clear which version was scored.

**Spec** [practice-mode.md](../../product/practice-mode.md)

---

### PRC-04 · Implement drills and drill-to-goal linkage

**Depends on** PRC-02, PRG-03 · **Blocks** nothing

Content drills from the coaching review and delivery drills from ART-04, each short, spoken, and able to
become a goal.

**Done when**
- [ ] Drills selected for a session are traceable to the observation that produced them.
- [ ] Adding a drill to goals creates a real goal rather than a label.
- [ ] The drill catalogue is server-provided data, not a hardcoded list.

**Spec** [practice-mode.md](../../product/practice-mode.md)

---

### PRC-05 · Implement practice notifications for ready results

**Depends on** PRC-01, INT-01 · **Blocks** nothing

The candidate is told when results and review are ready, honouring their notification preferences and
quiet hours.

**Done when**
- [ ] Notification fires once per session, even across workflow retries.
- [ ] Preferences and quiet hours from PRO-01 are respected.
- [ ] Screening sessions never trigger a results notification to the candidate.

**Spec** [observability.md](../../operations/observability.md) · [practice-mode.md](../../product/practice-mode.md)

---

### PRC-06 · Validate the results, review and delivery split with candidates

**Depends on** PRC-01, PRC-02, ART-05 · **Blocks** nothing

The three-way split is proposed, not proven. Research whether candidates can predict what is on each
screen, and consolidate if they cannot.

**Done when**
- [ ] Research is run with candidates who did not build the product.
- [ ] The finding is recorded in [information-architecture.md](../../product/information-architecture.md) either way.
- [ ] If the split confuses people, consolidation is scheduled rather than argued about.

**Spec** [user-journeys.md](../../product/user-journeys.md)
