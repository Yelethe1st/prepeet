# Epic A11Y — Accessibility and inclusive content

**Phase 1–4, continuous** · **Workstream** Product/design, Web

Accessibility is a correctness requirement in this product, not a polish phase. A candidate who cannot
complete an interview by keyboard and screen reader has been excluded from a hiring process, which is
the harm the whole responsible-hiring position exists to prevent.

---

### A11Y-01 · Establish the accessibility baseline in the design system

**Depends on** WEB-01 · **Blocks** every screen ticket

Focus management, contrast, target sizes, motion preferences and semantic structure, built into the
components rather than retrofitted per page.

**Done when**
- [x] Every interactive component is operable by keyboard with a visible focus state.
- [x] Minimum 24px targets throughout, 44px for primary candidate controls.
- [x] Reduced-motion preference is honoured by every animation.

**Done, as global rules rather than per-component effort, because a rule applied per
component is a rule missing from whichever component was added last.** Keyboard
operability comes from every interactive element being a native control, and the one
shared :focus-visible ring in theme.css covers them all; the browser suite pins both
the visible ring and the skip link against rendered styles, where jsdom is blind.
Reduced motion is the prototype's base.css blanket, now ported whole: durations
collapse globally under prefers-reduced-motion (and under the product's own
data-motion attribute), rather than animations being removed, so anything waiting on
animationend still fires; a browser test asserts the collapse on a real control's
computed style. Targets: the smallest control in the system is the 32px small button
and the 36px theme toggle, both above the 24px floor; inline text links carry the
inline exception. The journey's primary controls are the prototype's own btn-lg at
48px, and the audit found two ported screens that had silently dropped to the 40px
default against the prototype - the prepare screen's start and the wizard's terminal
submit - both now restored to lg with the size pinned in their tests. The live
surface's real controls (icon-btn-lg per the prototype) land with RTC-06, which
A11Y-02 gates.

**Spec** [information-architecture.md](../../product/information-architecture.md)

---

### A11Y-02 · Make the live interview fully operable without a mouse or sight

**Depends on** RTC-06, A11Y-01 · **Blocks** REL-02

The hardest screen in the product: realtime state changes, a timer, captions, push-to-talk and a
reconnection overlay, all announced correctly and none of it stealing focus mid-answer.

**Done when**
- [x] Every state change is announced once, at the right politeness level, without interrupting speech.
- [x] Push-to-talk works by keyboard with clear instructions and no timing trap.
- [x] The reconnection overlay takes focus appropriately and returns it on recovery.

**Done as built behaviour; the real-AT verification is A11Y-04's, which exists for exactly
that.** Announcements flow through one polite live region on the surface, one message at a
time, so a state change is heard once and a caption being read is not cut off; the two
assertive exceptions are earned - the not-transmitting warning while the candidate speaks,
and the overlay's own alertdialog - because both are situations where interrupting IS the
right politeness. Push-to-talk is hold-based with no timeout of any kind: hold Space or hold
the button, release to stop, with the instructions in text beside the control and open/closed
announced. The overlay moves focus to Retry now when it opens - the one decision the person
can act on - and returns focus to wherever it was when recovery closes it, pinned by a test
either way. What this ticket cannot honestly claim is how VoiceOver, NVDA and JAWS actually
render all of it on the real journey; that walk, with findings recorded, is A11Y-04.

**Spec** [realtime-protocol.md](../../architecture/realtime-protocol.md)

---

### A11Y-03 · Provide text and table alternatives for every chart

**Depends on** WEB-01 · **Blocks** PRG-04, ART-05, OPS-01

Every chart in the product carries a written summary of what it shows and a table containing the same
data.

**Done when**
- [x] No chart conveys information available nowhere else.
- [x] Summaries state the finding, not just the axes.
- [x] Colour never carries status or score without an accompanying word.

**Done - satisfied where each chart was built, recorded here after the audit.** The
product currently has two data charts and both carried their alternatives from birth:
the skills screen's standings chart (PRG-04) renders its summary and a competency
table beside it, and the delivery screen's pace chart (ART-05) is a role="img"
labelled by a summary that states the finding, each bar carrying its words-per-minute
as text, with the full table behind a disclosure. Both are pinned by tests that ask
for the summary and the table by role. The marketing page's illustrative charts are
aria-hidden with their content in the surrounding text. Status and score are words
everywhere: bands render as their names, lifecycle states as their vocabulary
(states.ts), and the audit found no colour-only conveyance. The rule stays cheap to
keep because the browser suite's axe run and these per-chart tests fail before a
silent chart ships; a new chart adds its summary, its table and their tests as part
of its own ticket.

**Spec** [information-architecture.md](../../product/information-architecture.md)

---

### A11Y-04 · Test with real assistive technology across the candidate journey

**Depends on** A11Y-02, PRC-01 · **Blocks** REL-02

VoiceOver, NVDA and JAWS, on the real journey, not a component gallery.

**Done when**
- [ ] The full candidate journey is completed with each screen reader and the findings recorded.
- [ ] Blocking findings are fixed before the practice release, not logged for later.
- [ ] The test is repeatable and scheduled, not a one-off.

**Spec** [release-criteria.md](../release-criteria.md)

---

### A11Y-05 · Run usability testing with disabled candidates

**Depends on** A11Y-04 · **Blocks** REL-02

Automated checks and screen-reader passes do not tell you whether a disabled candidate can actually
complete an interview under time pressure. Ask them.

**Done when**
- [ ] Testing includes candidates with visual, motor, cognitive and speech differences.
- [ ] Participants are compensated and the findings are published internally in full.
- [ ] Accommodation gaps found here become tickets, not caveats.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md)

---

### A11Y-06 · Implement and enforce the content rules

**Depends on** DEC-12, WEB-04 · **Blocks** nothing

Candidate copy is supportive and specific; recruiter copy describes observable evidence and uncertainty;
platform copy is operational. "Unverified" never implies false, "insufficient evidence" never implies
poor.

**Done when**
- [ ] Content rules are documented with examples of what to write and what not to.
- [ ] The prohibited interpretations from DEC-12 appear as explicit copy rules.
- [ ] Copy review is part of the definition of done for every candidate-facing ticket.

**Spec** [information-architecture.md](../../product/information-architecture.md)

---

### A11Y-07 · Verify the candidate journey on real mobile devices and networks

**Depends on** RTC-06, A11Y-01 · **Blocks** REL-02

Mobile-first from 320px is a requirement, and a voice interview on a phone on a train is the realistic
case, not the edge case.

**Done when**
- [ ] The journey completes on real devices over a constrained mobile network.
- [ ] Interruptions typical of mobile — a call, a lock screen, a network handover — are recoverable.
- [ ] No candidate surface scrolls horizontally at 320px.

**Spec** [product-requirements.md](../../product/product-requirements.md)
