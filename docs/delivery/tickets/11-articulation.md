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
- [x] Each dimension carries a level and the evidence segments behind it.
- [x] No aggregate delivery score exists anywhere in the API or the interface.
- [x] Intelligibility is defined and tested as followability, with no accent-conformity component.

**Done at the deterministic floor: articulation-profile-v1.** All ten dimensions are derived
from the features and the candidate's own words with the rule stated beside each level as its
reason - pace from words per minute (no direction called "correct"; the reason gives the number),
pausing from long pauses per minute, fluency from fillers plus restarts, conciseness from
repeated phrases, signposting and structure and precision from the answers' own phrases and
figures, responsiveness from whether an answer echoes its question's terms. Each level names the
turn sequences behind it; a level that cannot be measured says why (vocal delivery is
not_assessable until the recording is decoded here). The profile rides the analysis document
alongside the measurements.

No aggregate exists: the profile document has no field a total could live in, a test forbids
score, overall, total, percentage and grade at any depth of the served analysis, and the results
API's delivery block is proven on the wire to carry only status, warnings and the note.
Intelligibility is followability, defined as transcript confidence and sentence length and
nothing else; tests show it moving with each of those, and a source-level test forbids any input
that could encode an accent - no audio, no phonetics, no locale reaches the calculator. The
delivery screen that renders the profile is ART-05's; a model-backed refinement of the
language-dependent dimensions would sit behind the same document with the same gate.

**Spec** [articulation-system.md](../../architecture/articulation-system.md)

---

### ART-04 · Generate fact-preserving delivery coaching and suggested structure

**Depends on** ART-03 · **Blocks** ART-05

One or two priorities per session, each stating listener impact and one action, with a suggested shape
built from the candidate's own sentences.

**Done when**
- [x] A suggested rewrite never introduces a fact, metric or outcome the candidate did not say.
- [x] Missing information appears as a placeholder or a question, never as an invented detail.
- [x] A fact-preservation test suite runs against the coaching stage in CI.

**Done at the articulation-coaching-v1 floor, fact-preserving by construction and by gate.** One
or two priorities are the profile's weakest measurable dimensions, each with a listener impact,
one action and a selected drill from the spec's list, written as neutral copy about what the
listener experiences, never about the person. The suggested shape (headline, context, reasoning,
result) is assembled only from the anchoring answer's own sentences; a slot with nothing to fill
it is a bracketed question. The floor cannot invent a fact because it has no vocabulary to do so.

preserve() is the gate that outlives the floor: every quote must be an exact substring of its
own candidate turn, every placeholder a bracketed question with no digit (a number in brackets is
a fact wearing them), every priority's evidence a real candidate turn, and one violation refuses
the whole coaching. The served analysis is gated first; a refusal becomes a stated absence with
the reason, never a served invention. tests/test_delivery_fact_preservation.py attacks the gate
four ways plus a foreign-turn citation and runs in the same CI job as every Python test
(uv run pytest with the coverage floor). The delivery screen that renders this is ART-05's.

**Spec** [articulation-system.md](../../architecture/articulation-system.md)

---

### ART-05 · Build the delivery screen with timestamped evidence and drills

**Depends on** ART-04, WEB-04 · **Blocks** ART-06

*Implemented in the prototype; carry it into production.* Measurements with the candidate's own ranges,
pace and pause visualisation with a table alternative, the ten dimensions, timestamped click-to-play
observations, and the eight delivery drills.

**Done when**
- [x] Every chart has a text summary and a table alternative.
- [ ] Every observation plays the moment in the candidate's own recording.
- [x] A screening session shows the boundary explanation, not a locked page.

**Built at /session/[id]/delivery from candidate-session-articulation, served by GET
/interviews/{id}/delivery (the stored analysis document, DELIVERY_NOT_READY until the workflow
lands).** Measurements with the calculator named on screen; the pace chart is a role=img
labelled by its text summary with a full table alternative beside it (the first box, asserted
by test); the ten dimensions each with level, reason and evidence, never summed (a test forbids
a delivery score in the rendered page); at most two priorities with listener impact and one
action; the suggested shape with placeholders marked in the DOM; the eight drills with the
selected ones first. Two recorded deviations: the prototype's personal ranges are replaced by
copy saying there is no correct rate and a personal range appears once ART-07 has enough
sessions; and the recording is not yet served, so every observation jumps to its transcript
moment by keyboard (proven) and the second box stays open until playback exists. The third box
is structural today: no screening session can reach any candidate session surface (the
API's practice-only enum), and the explanatory boundary copy lands with the SCR epic's screens
behind DEC-11.

**Spec** [articulation-system.md](../../architecture/articulation-system.md) · [information-architecture.md](../../product/information-architecture.md)

---

### ART-06 · Implement redo and the original-versus-redo comparison

**Depends on** ART-05, PRC-03 · **Blocks** nothing

A candidate redoes one answer and sees it beside the original with the metric deltas — and the original
is never overwritten.

**Done when**
- [x] The original answer and its evidence survive the redo intact.
- [x] The comparison shows metric deltas and states which question the redo actually answered.
- [x] Redos are practice-only and never enter a screening evaluation.

**Done on PRC-03's linked-session model.** The review screen offers "Redo this answer" per
answer, creates the retake through POST redos and goes to prepare it like any session; an
answer already redone links to its comparison instead of offering a second, and a refusal is
shown by name with nothing navigating. The comparison lives on the retake's delivery screen:
the session says it is a retake and of what, the original's own delivery analysis supplies the
other side, and the table shows original, redo and the change for pace, fillers and long pauses,
with the question the redo actually answered quoted above it. The original is another session's
row set, read and never written; the first box is PRC-03's DeepEqual proof. Practice-only is the
redos table's mode CHECK and the command's refusal, so a screening evaluation cannot contain a
redo by construction. A missing original analysis is said, not hidden.

**Spec** [practice-mode.md](../../product/practice-mode.md)

---

### ART-07 · Build personal delivery baselines and trends

**Depends on** ART-01, PRG-01 · **Blocks** nothing

Compare a candidate with their own history once there is enough of it, and never with a universal
standard.

**Done when**
- [x] A baseline requires a documented minimum number of measured sessions before it is drawn.
- [x] Copy states that suggested ranges are guidance, not a correct speaking rate.
- [x] Baselines are purpose-scoped and provably unreachable from screening.

**Done: baseline-1.** MinBaselineSessions is five, documented on the constant and answered in
the honest absence (sessions measured, minimum required) so a screen can say how far away the
baseline is; not-assessable sessions never count. A range is the middle half (nearest-rank
quartiles, no interpolation, so a range is always made of values that occurred) of the
candidate's own measured sessions per metric. The note ships with every baseline, ready or
not: guidance about you, not a target, no correct speaking rate; the delivery screen renders the
ranges beside this session's numbers only when ready and otherwise says how many sessions remain.

Purpose scoping is structural, not remembered: GET /me/delivery-baseline reads the history under
the caller's own practice scope, and a screening analysis is a tenant's row that scope cannot
see under RLS. The integration proof seeds five assessable analyses in a scope the candidate
does not own and shows their baseline still counts zero measured sessions. Trends over time are
the same history plotted, which PRG-03's progression screen owns.

**Spec** [articulation-system.md](../../architecture/articulation-system.md)
