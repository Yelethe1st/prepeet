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

---

### ART-08 · Detect hedging and softeners as evidence under precision

**Depends on** ART-01, ART-03 · **Blocks** nothing

"I think", "maybe", "sort of", "kind of", "I guess", "probably", "a little bit": the phrases that
blunt a claim the candidate can actually back.

Hedging is the one delivery feature the product does not measure that it plainly should. It is
language, not sound, so it is computed from the transcript exactly as fillers and restarts already
are, and it needs no audio, no phonetics and no locale, which is what keeps it on the right side of
ART-03's accent rule.

It is evidence under **precision**, not an eleventh dimension. The profile's shape is contractual and
ten dimensions are what ART-03 published; a hedge is a candidate not saying exactly what they mean,
which is what precision already measures.

**Done when**
- [x] Hedge phrases are counted per turn in the deterministic feature set, with the same versioned calculation and the same not-assessable rules as fillers.
- [x] The phrase list is data rather than code, versioned with the calculator, so adding one is a data change and a test.
- [x] Precision's reason names the count and the phrases behind it, and its evidence points at the turns they occurred in.
- [x] A hedge on a claim the candidate goes on to support is told apart from a hedge that honestly marks uncertainty, and only the first is ever coached.
- [x] No count is presented as a target, and zero hedges is never described as the goal.

**Done, at articulation-features-v2.** `hedge_count` and `hedge_phrases` ride each turn beside
the fillers they resemble, counted by the same arithmetic over the same tokens: no audio, no
phonetics, no locale, which is what keeps the feature clear of ART-03's accent rule. The version
bumped because the calculator's output changed, and a changed calculator must not claim to be the
old one.

The phrases are in `hedges.json` beside the module, versioned as `articulation-hedges-v1`, with
the exclusions written down and argued rather than merely absent: "just" is temporal far more
often than it is softening, "like" is already excluded from fillers for the same reason, and
"really" strengthens a claim rather than softening it. They are sorted longest-first at load
instead of by hand, so an edit to the file cannot silently stop "a little bit" being matched by
leaving "a bit" in front of it.

The distinction that makes the feature safe is made in the profile rather than the calculator,
because only there is it known whether the claim was backed. Precision already defines support as
a concrete figure, so a hedge in a turn carrying a figure is a claim the candidate could have made
plainly and softened anyway, and a hedge in a turn with no figure is somebody honestly marking an
estimate and is never surfaced at all. The reason reads: `1 of 1 answers carried a concrete
figure; 1 of those softened it with "i think"`.

Neither moves the level, and that is the point rather than an omission. Penalising "I think it
was about 30%" would penalise an honest estimate, which is the opposite of the lesson, so the
level stays what precision has always measured and the hedging is evidence beside it. A test
asserts the level is identical for the same answer softened and plain, and another forbids
"should", "aim", "target", "avoid" and "reduce" anywhere in the reason.

One trap is recorded in the suite itself. Two of the tests first passed against
`no assessable candidate speech`, because the replacement answer was eleven words and the floor is
twenty, so an assertion that something was *absent* was true without measuring anything. A test now
asserts the fixture is assessable before the three that depend on it.

**Watch for**

Zero is the wrong target and the reason matters. An interview answer that hedges nothing is an
answer claiming certainty the candidate does not have, and coaching somebody into that is coaching
them to bluff, which the interview will find out and which this product exists to argue against.
"I think" before a genuine estimate is correct English and correct behaviour. The feature is only
useful where the hedge sits in front of something the candidate then evidences, and the coaching
has to be able to tell the difference or it should stay quiet, per ART-07's rule that guidance is
never a target.

The second is register. Hedging frequency varies with first language, gender and culture, and a
count read without that context is a politeness tax dressed as feedback. It is practice-only for
that reason: it never reaches a screening evaluation, and it never becomes a number anybody is
compared on.

**Spec** [articulation-system.md](../../architecture/articulation-system.md)

---

### ART-09 · Let a candidate say a generated insight was wrong

**Depends on** ART-05 · **Blocks** nothing

A thumbs up and a thumbs down on each generated strength, priority and drill, recorded against the
artifact digest that produced it.

Nothing in the product currently carries a signal from the person the coaching is about back to the
people who maintain it. QUA-06 monitors AI quality in production and QUA-03 calibrates against human
benchmarks, and neither has an input from the candidate reading the output. The one person who knows
whether "your opening establishes a clear, defensible position" is true of their own answer is the
person who gave it.

**Done when**
- [x] Each generated insight can be marked helpful or not, once, changeable, and never required.
- [x] The signal records the pinned artifact digest, the model policy and the dimension, so a drop is attributable to what produced it rather than to a date.
- [ ] Rejections are queryable per artifact version and feed QUA-06's monitoring, rather than sitting in a table nobody reads.
- [x] A rejection changes nothing the candidate is shown: it is a report about the coaching, not a way to edit it.
- [x] Nothing is asked for. No free-text box, no prompt, no modal, and no follow-up if it is ignored.
- [x] Practice only, and never attached to a screening evaluation, where a candidate rating their own assessment would be a channel for pressure.

**Done except QUA-06's read, which has nowhere to run from.**
`evaluation.insight_feedback` (migration 0038) carries the verdict with the artifact digest and
the policy version taken at the moment the insight was on screen, so a drop in helpfulness is
attributable to a version rather than to a date. `RecordInsightFeedback` upserts on the unique
constraint, so pressing the other thumb corrects the row rather than adding a second opinion from
the same person about the same sentence: this is the one judgment in the schema that is
deliberately not immutable, because it is a report about the coaching rather than a record of what
happened.

Practice-only is structural twice over. There is no `tenant_id` column and no tenant policy, so a
screening scope matches no row it could read or write, and the domain refuses a screening ref
before the database with a sentence rather than a constraint name. Both are attacked: a screening
session is refused, a tenant scope reads nothing, and a second candidate reads nothing. The last
two also assert the owner still sees the row, because an attack that matches zero rows for want of
any row proves nothing at all.

**The aggregate read has nowhere to run from, and that is the finding of this ticket.** A rates
query was written, generated and then removed. Both database roles are NOBYPASSRLS and the only
policy scopes to one candidate, so a query counting rejections across candidates returns an empty
result rather than an error: a report that silently answers "none" is worse than no report,
because somebody will believe it. QUA-06's read needs its own path, either a metric emitted per
verdict or a reporting role with a policy written for it, and that decision belongs with QUA-06
rather than being faked here. The index the read will want is in place.

`PUT /interviews/{sessionId}/delivery/feedback` takes the verdict and answers 204 with an empty
body, because nothing changed: returning the coaching again would suggest the verdict had edited
it. The digest and policy version are read from the stored analysis rather than taken from the
request, so a client cannot attribute a verdict to an artifact that did not produce what it was
looking at. `DeliveryView` carries the candidate's own verdicts back, as an empty list rather than
null, so the screen shows which thumb is pressed instead of asking again and does not crash on the
person who has never pressed anything, which is most of them.

On the screen the controls sit under each priority and under each drill chosen for this session.
An unselected drill gets none: it is a menu item rather than something generated about this
candidate, and there is nothing for them to answer about it. A failure to send is silent, which is
deliberate. This is feedback about the product given as a courtesy, and interrupting somebody
reading their own coaching to report that our own call failed makes our problem theirs.

Four things are asserted rather than intended: the priority's text is byte-identical before and
after a rejection, no dialog and no textbox exist on the screen, a verdict given before this visit
is still pressed, and pressing the other thumb corrects rather than accumulates.

**Watch for**

A thumbs down means "this did not describe me", and the temptation is to read it as "this was
harsh". Those are different, and only the first is a quality signal. The rate is the measurement,
per artifact version and per dimension; an individual rejection says nothing on its own and must not
be actionable on its own, or the honest coaching is the coaching that gets tuned away.

**Spec** [articulation-system.md](../../architecture/articulation-system.md) · [ADR-0011](../../architecture/decisions/0011-artifact-registry-review-publication-and-rollback.md)
