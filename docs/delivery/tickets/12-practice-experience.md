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
- [ ] Every score can be expanded to the exact sentences that produced it.
- [ ] Insufficient competencies read as insufficient, never as zero or as a low band.
- [ ] Audio replay jumps to the evidence timestamp and works by keyboard.

**Spec** [practice-mode.md](../../product/practice-mode.md)

---

### PRC-02 · Build the coaching review screen

**Depends on** PRC-01 · **Blocks** PRC-03

Per-answer strengths, gaps, rationale and next action, a fact-preserving model rewrite of each answer,
patterns across sessions, and the drills that follow from them.

**Done when**
- [ ] Every coaching statement traces to something the candidate said.
- [ ] A rewrite never adds a fact; missing information appears as a placeholder or question.
- [ ] Coaching failure leaves the evaluation intact and says so.

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
