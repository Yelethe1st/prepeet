# Epic ART — Delivery and articulation

**Phase 3–4** · **Workstream** Python, Web, AI/data quality

Measured first, interpreted second. Deterministic features are computed before any model sees the
answer, so a model can never invent a words-per-minute figure. Practice only: articulation is excluded
from employer scoring by default, and accent is never scored at all.

Delivery sequence follows [articulation-system.md](../../architecture/articulation-system.md): transcript
measures first, audio measures second, personalisation third.

---

### ART-01 · Compute deterministic delivery features from audio and transcript

**Depends on** RTC-04, RTC-05, DEC-07 · **Blocks** ART-02

Words per minute, pause frequency and duration, fillers per hundred words, restarts, repeated phrases,
abandoned sentences, answer length, and recording quality — each with a calculator version and input
references.

**Done when**
- [x] Every metric is reproducible from the same inputs and carries its calculator version.
- [x] Metrics are computed before any model call and are never model-generated.
- [x] Known audio fixtures produce known values within a documented tolerance.

**Done at the transcript-timing floor: articulation-features-v1.** Words per minute, pause
count, average, maximum and long-pause count (700 ms floor), fillers per hundred words (an
unambiguous list; "like" and "so" are deliberately not counted because a count that guesses is
worse than one that abstains), restarts (an immediately repeated word or two-word phrase),
repeated three-word phrases, answer length and transcript confidence, each per candidate turn
with its calculator version and the turn sequence it came from, aggregated over assessable turns
only (totals, never averages of averages). Reproducibility is proven by equality over a
round-tripped copy of the input; a test asserts the module imports nothing that could consult a
model. Served over the contract's AnalyzeArticulation with provider_calls zero.

The honest boundary, stated in the result rather than papered over: audio-derived quality
(clipping, noise, volume) is not computed yet because the recording is not decoded here, so
audio_quality is null and every result carries AUDIO_QUALITY_NOT_COMPUTED; the "audio fixture"
of the third box is a known transcript-timing fixture with hand-computed expectations and a
documented tolerance (0.5 wpm, 0.1 fillers per hundred). Thin speech (under 20 words), missing
word timing and low transcript confidence are not_assessable by name, never a low value. The Go
workflow that requests the analysis at completion and stores it belongs to ART-02.

**Spec** [articulation-system.md](../../architecture/articulation-system.md)

---

### ART-02 · Implement assessability status and quality warnings

**Depends on** ART-01 · **Blocks** ART-03

Assessable, partially assessable, or not assessable. Poor audio produces a status and a warning — never
a low delivery result.

**Done when**
- [x] Clipping, insufficient speech and low transcript confidence each produce an explicit status.
- [x] A not-assessable result states plainly that it is not a low result and has not affected any score.
- [x] Valid content evaluation continues unaffected when delivery is unassessable.

**Done.** Each cause names itself: AUDIO_CLIPPED and AUDIO_SILENT from a sample-level quality
calculator (clipping above one sample in fifty at full scale, proven on synthetic samples so the
rule exists before any decoder does), INSUFFICIENT_SPEECH and TRANSCRIPT_CONFIDENCE_LOW from the
transcript side, each an explicit not_assessable status. The plain statement is server-supplied
copy on the analysis document and again on the results API's delivery block, so no surface can
drop or soften it: "not a low result, and it has not affected any score", asserted by test on
the wire.

The third box is structural rather than remembered: delivery runs as its own Temporal workflow
under its own id (articulation-{session}) with its own immutable row (evaluation.articulation),
started from the same session_completed event as evidence but never awaited by it. The
integration proof stores a content result, lands a not-assessable delivery beside it, and reads
the result back DeepEqual-identical with zero evaluation.failed events. The results endpoint
reports delivery as pending until the row lands. Decoding the recording to feed the clipping
calculator real samples is the remaining audio work, tracked with ART-03's profile.

**Spec** [articulation-system.md](../../architecture/articulation-system.md)

---

### ART-03 · Produce the ten-dimension delivery profile with evidence

**Depends on** ART-02, EVL-01 · **Blocks** ART-04

Structure, conciseness, fluency, pace, pausing, precision, signposting, intelligibility, vocal delivery
and responsiveness — each a level with linked evidence, never collapsed into one percentage.

**Done when**
- [ ] Each dimension carries a level and the evidence segments behind it.
- [ ] No aggregate delivery score exists anywhere in the API or the interface.
- [ ] Intelligibility is defined and tested as followability, with no accent-conformity component.

**Spec** [articulation-system.md](../../architecture/articulation-system.md)

---

### ART-04 · Generate fact-preserving delivery coaching and suggested structure

**Depends on** ART-03 · **Blocks** ART-05

One or two priorities per session, each stating listener impact and one action, with a suggested shape
built from the candidate's own sentences.

**Done when**
- [ ] A suggested rewrite never introduces a fact, metric or outcome the candidate did not say.
- [ ] Missing information appears as a placeholder or a question, never as an invented detail.
- [ ] A fact-preservation test suite runs against the coaching stage in CI.

**Spec** [articulation-system.md](../../architecture/articulation-system.md)

---

### ART-05 · Build the delivery screen with timestamped evidence and drills

**Depends on** ART-04, WEB-04 · **Blocks** ART-06

*Implemented in the prototype; carry it into production.* Measurements with the candidate's own ranges,
pace and pause visualisation with a table alternative, the ten dimensions, timestamped click-to-play
observations, and the eight delivery drills.

**Done when**
- [ ] Every chart has a text summary and a table alternative.
- [ ] Every observation plays the moment in the candidate's own recording.
- [ ] A screening session shows the boundary explanation, not a locked page.

**Spec** [articulation-system.md](../../architecture/articulation-system.md) · [information-architecture.md](../../product/information-architecture.md)

---

### ART-06 · Implement redo and the original-versus-redo comparison

**Depends on** ART-05, PRC-03 · **Blocks** nothing

A candidate redoes one answer and sees it beside the original with the metric deltas — and the original
is never overwritten.

**Done when**
- [ ] The original answer and its evidence survive the redo intact.
- [ ] The comparison shows metric deltas and states which question the redo actually answered.
- [ ] Redos are practice-only and never enter a screening evaluation.

**Spec** [practice-mode.md](../../product/practice-mode.md)

---

### ART-07 · Build personal delivery baselines and trends

**Depends on** ART-01, PRG-01 · **Blocks** nothing

Compare a candidate with their own history once there is enough of it, and never with a universal
standard.

**Done when**
- [ ] A baseline requires a documented minimum number of measured sessions before it is drawn.
- [ ] Copy states that suggested ranges are guidance, not a correct speaking rate.
- [ ] Baselines are purpose-scoped and provably unreachable from screening.

**Spec** [articulation-system.md](../../architecture/articulation-system.md)
