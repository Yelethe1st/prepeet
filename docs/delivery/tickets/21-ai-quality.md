# Epic QUA — AI quality, datasets and monitoring

**Phase 2–6, continuous** · **Workstream** AI/data quality

An evaluation nobody can check is not evidence. This epic builds the datasets, the harness and the
monitoring that make it possible to say whether the product is any good — and to notice when it stops
being good.

---

### QUA-01 · Build representative evaluation datasets across professions

**Depends on** CTR-02 · **Blocks** QUA-02

Fixtures spanning the disciplines the product claims to serve — nursing, teaching, finance, sales,
product and engineering — not just software interviews.

**Done when**
- [x] Each supported profession has fixtures with expected evidence and known edge cases.
- [x] Fixtures include insufficient-evidence, contradiction and unassessable cases deliberately.
- [x] Dataset provenance, consent and licensing are documented.

**Done, and the fixtures are synthetic on purpose rather than for convenience.** Six professions in
`services/intelligence/evals/datasets`: nursing, teaching, finance, sales, product and engineering,
twenty six cases with three competencies each. Every case declares what it expects rather than
recording what happened to come out: sufficiency per competency, the evidence spans that must appear
with the words they must quote, the competencies that must stay silent, the contradictions that must
and must not be raised, and the delivery assessability. QUA-02's harness checks all of it, so a
fixture whose behaviour drifts fails a named test instead of quietly becoming the new normal. Every
expectation here was written by hand from the rules and then reconciled against the extractor; where
the two disagreed the transcript was rewritten until the fixture demonstrated what it claims to.

Every profession carries the three deliberate classes, and a test enforces that rather than trusting
it. Unassessable is four states, not one, because each has a different remedy: no candidate speech, no
word timing, a transcript under the confidence floor, and too little speech to measure. All four appear
across the set and a test fails if one is dropped. Insufficiency is likewise three distinct facts in
one case: evidence that exists and is below the rubric's floor of two, an admitted gap, and a
competency the conversation never reached.

Two cases exist to record something the extractor gets wrong. `contradiction_false_positive` is a
measured improvement in one subject across two moments in time, which evidence-1 pairs as a numeric
disagreement because it cannot tell one event from another. The fixture declares the pair as not
genuine, so the harness counts it against the extractor rather than scoring it as a success: eight
pairs across the set, six genuine, two false positives. A third fixture records that a candidate who
says architecture where the rubric says systems design loses the link entirely, because evidence-1
matches the competency name's own tokens. Both are honest properties of the deterministic floor and
both are now numbers rather than suspicions.

Provenance is `evals/datasets/manifest.json` for the machine and `evals/datasets/README.md` for the
reader, carrying every field the specification names plus licensing and a review date with an owner.
The fixtures are synthetic and the manifest says why that is right here and not merely easier: the
dataset's job is to cover a candidate who contradicts themselves, one who says three words and one who
speaks an instruction at the pipeline, and collecting those from real interviews would mean recording
people at their worst and keeping it so a regression test can replay it. The report artifact is
committed alongside, so real transcripts would ride into every clone of the repository forever. Consent
is recorded as not applicable rather than obtained, because obtained would describe a process that
never happened. The manifest's digests are checked against the bytes on disk by a test, so a fixture
cannot be edited while the record of where it came from stays still. Verified by editing one and
watching the named test fail.

**What is deliberately absent, and recorded rather than quietly filled.** No audio, so clipping, noise
and device differences have no fixtures and QUA-05 cannot be built on this dataset alone. No accent or
dialect coverage: writing those by imitation would encode a stereotype rather than test one, and doing
it properly needs real speakers, consent and a lawful basis. Every case is British English, because
supported languages are still an open decision and inventing coverage would fake an answer to it. Word
timings are materialised from a declared speaking rate and explicit pause positions rather than stored,
which keeps the files readable but means pace and pausing are exercised as arithmetic and not as speech
rhythm. Accommodations, document injection and mode leakage belong with the tickets that build those
surfaces. All of it is listed in the manifest, because a limitation nobody wrote down is one somebody
will later mistake for coverage.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### QUA-02 · Build the automated evaluation harness

**Depends on** QUA-01, EVL-02 · **Blocks** QUA-03

Run a candidate evaluation pipeline against the fixtures and report evidence grounding, unsupported
facts, schema conformance, stability, latency and cost.

**Done when**
- [x] The harness runs on every prompt, model or rubric change.
- [x] Unsupported-fact rate is measured, not estimated.
- [x] Results are comparable across runs and versions.

**Done for the stages that live in the Python plane, which is less than the ticket's title implies and
is said plainly below.** `prepeet_ai.evals` runs every fixture through evidence extraction,
contradiction detection, the delivery profile and the coaching gate, applies the pinned rubric's own
sufficiency threshold, and writes one versioned report to `evals/reports/latest.json`.

"Runs on every change" is enforced twice rather than asked for. The harness is part of the ordinary
pytest suite, so CI runs it on every commit whether or not anybody remembers to. On top of that the
committed report carries a digest over its governed inputs: the extraction, calculation, profile and
coaching versions, the model policy, the pinned rubric and policy bodies by digest, and the dataset
manifest. Changing any of them without regenerating fails a named test. Proven by attacking it four
ways: moving the rubric's sufficiency floor from two to three, bumping the extractor's version,
changing the uncertainty rule with no version bump at all, and editing a fixture. Each one failed the
right named test and only that test.

The unsupported-fact rate is a count, not an impression. Every assertion the pipeline makes is checked
against the sealed transcript by exact string comparison: each evidence span against the text at its
own character range, both sides of every contradiction the same way, every coaching quote against the
turn it names, and every coaching placeholder against the rule that a figure in brackets is a fact
wearing them. 159 assertions across 26 cases, 0 unsupported. That zero is only worth something because
the meter was attacked on every surface it covers: a fabricated span, a quote borrowed from the
interviewer, a contradiction side quoting another interview, a coaching quote the candidate never said,
a quote attributed to no turn, and a number smuggled into a placeholder. Each is a named test that
fails if the meter stops moving. Coaching is included deliberately, because it is the one surface where
invented prose would appear, and a rate computed without it would measure the safest thing in the
service and call it the whole product.

Comparability is the artifact. The report is a stable sorted document with a `results_digest` over
everything except timing, so two runs of the same code produce the same digest and a diff across
versions names exactly which span, contradiction or sufficiency outcome moved and in which profession.
Latency sits outside the digest on purpose: a slower laptop is not a regression in the evaluation.
Regenerating is `uv run python -m prepeet_ai.evals`, which exits non-zero on a broken floor, and the
diff is the thing to read.

Cost is reported as zero and the zero is measured rather than asserted. The run installs a counter over
socket connections, so a provider call appearing where there is currently none would show up as a
number rather than as nobody noticing. The counter itself is attacked in the suite, through both
`connect` and `connect_ex`, because an unmeasured zero is a claim.

The hard floors are absolutes the specification already requires and not thresholds anyone chose:
grounding and schema conformance perfect, unsupported facts zero, and every fixture behaving as it
declares. Each gate is proven to be able to fail, and so is the detection behind it: the harness is
handed a deliberately wrong fixture and made to notice a moved sufficiency outcome, a missing span, a
competency that should have stayed silent, a contradiction that never arrives, a wrong delivery status,
a wrong unassessable cause, a stability probe that shifts, and a declared sensitivity that stops
existing. A report generator that cannot fail is a formatter, and the only way to tell them apart is to
hand it something wrong.

**Current numbers.** 26 cases across 6 professions, 79 grounding checks all grounded, 159 assertions
with 0 unsupported, 71 records all schema conformant, 195 declared expectations all met, 4 of 4
stability probes stable with 1 recorded known sensitivity, 8 contradictions of which 2 are declared
false positives, 0 provider calls and 0 network connections.

**What this harness does not do.** It runs the Python stages only. Band aggregation is Go's aggregate-1
and is not executed here, so the rubric's sufficiency threshold is read from the pinned artifact rather
than reimplemented from an opinion, but the bands themselves are untested by this harness and a
cross-language harness is the honest next step. Latency and cost figures describe the harness against
short synthetic transcripts, not a production session, and should not be quoted as if they did. There
is no separate `make evals` target or dedicated CI step: the harness rides the existing Python job,
which satisfies the criterion but means the numbers are not surfaced as a build artifact yet. That
target belongs in the root Makefile, which is outside this ticket's service. Nothing here is calibrated
against human judgement, which is QUA-03's whole subject, so every number that would need calibrating
is reported and none of it is gated.

**Amended by QUA-04.** The committed report was not committed. It was described here and in
`evals/reports/README.md` as the tracked artifact, required by two named tests, and listed in
`.gitignore`, so a clean checkout could not collect the Python suite at all. It is tracked now, with
the per-run timing that motivated the ignore written to an untracked `latest.timing.json` beside it,
and it carries the date it was generated so QUA-04's gate can refuse a stale one.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### QUA-03 · Calibrate confidence and quality thresholds against human benchmarks

**Depends on** QUA-02, DEC-12 · **Blocks** EVL-05

The numeric thresholds the specification deliberately refuses to guess. Set them against human
benchmark ratings and record the evidence.

**Done when**
- [ ] Thresholds are derived from human agreement data, not chosen for how they look.
- [ ] Inter-rater agreement on the benchmark set is measured and reported.
- [ ] The thresholds carry a review date and an owner.

**Not done, and none of the three is ticked. There is no human benchmark data in this repository, so
there is nothing to calibrate against.** Every threshold in the product is still the crude initial rule
ADR-0015 describes as crude, and ADR-0015's prohibition on numeric confidence display stands
unchanged. What landed is the machinery, the refusal, and the plan for the data that does not exist.

The refusal is the part that matters, because the failure mode here is not an incomplete ticket, it is
a calibration nobody performed wearing the word calibrated. `calibrate()` returns
`calibrated=False` with a named reason, `numeric_confidence_permitted()` returns False, and the
harness command prints `NOT calibrated against human judgement` beside every number it reports, so a
reader who is not told cannot assume somebody checked. Two separate acts are required before anything
here can say otherwise: a rating set declaring human provenance with a collection record, and the
plan's owner naming that set in `approved_benchmark_sets`. A test also asserts that no file in
`evals/calibration` claims a human rater today, so fabricated benchmark data cannot arrive quietly.
Verified by planting one: a set of 78 items with three raters in perfect agreement, dropped into
`benchmarks/`, failed the tripwire test by name and was still refused with `SET_NOT_APPROVED`, and
numeric confidence stayed prohibited. Doing the second act as well did produce a calibration, which is
the design working rather than a hole: it takes two visible edits by somebody who has decided to lie.

The arithmetic is real and is checked against answers worked out by hand rather than against its own
output. Observed agreement, Cohen's kappa, Fleiss' kappa and Krippendorff's alpha, each with the
worked example in the test's own docstring, and each refusing where the statistic is undefined: a
kappa whose chance agreement is one is not 1.0, and an agreement rate over zero comparable items is
not 0.0. Alpha is there because a real exercise loses ratings and an estimator that cannot take a gap
invites somebody to fill it in. The confidence derivation mirrors Go's `confidenceOf` exactly, so the
sweep sweeps the rule the product applies, and the sweep only generates candidates that would still be
a publishable rubric under the registry's own ordering rules. The threshold record a calibration would
publish carries the measured agreement, the whole sweep rather than only the winner, an owner and a
review date, and cannot be constructed without all four.

`evals/calibration/exercise.json` exercises all of it end to end. Its three raters are declared rules,
not people, and its labels are computed at runtime rather than stored, because a file of stored labels
is indistinguishable on inspection from a file of human ratings. Every figure it produces is stamped
`exercise_only`.

**What is not calibrated, stated plainly.** The confidence thresholds. The sufficiency floor. The
band boundaries. Every quality threshold in the evaluation system. No inter-rater agreement has been
measured on any benchmark set, because no benchmark set exists; the only agreement figures in the
repository describe three arithmetic rules disagreeing with each other. Nothing here licenses a
numeric confidence display, a percentage, an interval or a gauge on any surface. EVL-05, which this
ticket blocks, is still blocked.

**What would unblock it, and why it was not done here.** Ratings are judgements by identified people
about recorded speech, and collecting them needs a lawful basis, a purpose statement, a retention rule
and each rater's agreement to be named. The plan in `evals/calibration/plan.json` states the floors a
set would have to clear: three independent raters, sixty items, all six professions, and Krippendorff's
alpha at or above 0.800, which is quoted from Krippendorff's published convention rather than chosen
here, because a floor picked by the team that wants to pass it is not a floor. Inventing raters to fill
this in would have produced a green ticket and a worse product.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### QUA-04 · Gate artifact and model publication on an evaluation report

**Depends on** QUA-02, CAT-01 · **Blocks** REL-02

No prompt, model or rubric reaches production without a report, an approver and a rollback plan.

**Done when**
- [x] Publication is blocked without an attached evaluation report.
- [x] The approver is a named person and cannot be the author for a material change.
- [x] Rollback is demonstrated, not assumed.

**Done for the git-authoring path, which is the only path an artifact takes today, and the one place
it is still not enforced is named below rather than left to be discovered.** `prepeet_ai.evals.publication`
is the validating state ADR-0011 gives to artifacts authored in git. It refuses a publication eleven
ways and every one of them is a named test driven by a deliberately broken record: no report at all, a
report quoting a run that does not exist, a report produced before the governed inputs moved, a report
older than the policy's age limit, an undated report, a report below one of the harness's hard floors,
a report that never ran against these bytes, an approver who is the author, an approver who is a
service principal on a material change, a missing rollback plan, and a rollback plan naming a version
that is not in the tree or has been edited since. A twelfth test admits a complete record, because a
gate that has only ever refused is a gate nobody can use.

The floors are not restated in the policy. `harness.gate_violations` owns them, so this gate cannot
loosen an absolute the specification already requires, and coverage is checked by digest rather than
by trust: a report vouches for the rubric and policy bodies its governed inputs actually record, and a
plan or catalogue change is refused with `REPORT_DOES_NOT_COVER_THE_ARTIFACT` because today's harness
produces no evidence about them. That refusal is honest rather than obstructive, and it is where the
cross-language harness QUA-02 asked for would pay for itself.

Where it bites in this repository is the whole tree. A named test walks every artifact and requires
each one to be either covered by a publication record that itself survives the gate, or listed by
digest in the policy's pre-gate list. The list is digests rather than filenames on purpose, so editing
one of the nine existing artifacts fails exactly as adding a tenth does. Proved both ways: adding an
unrecorded rubric failed `test_every_artifact_in_the_tree_is_recorded_or_declared_pre_gate` by name,
and moving the pinned rubric's sufficiency floor from 2 to 3 failed that test plus the report coverage
test, each restored afterwards with the restore asserted rather than assumed.

The approver rule is stricter than the ticket asks. ADR-0011 refuses a publisher who drafted the
change for every artifact, not only material ones, so that is the rule enforced. On top of it, a
material change may not be approved by a service principal, which closes a real gap: `contentctl`
drafts as one account and publishes as another, satisfying the registry's two-person rule with two
accounts and no person, so the human who approved a git-authored artifact was recorded nowhere a
machine could read. The publication record is where that name now lives.

Rollback is executed rather than asserted. The plan is run, the previous version is read from the
tree, and its bytes and digest are compared with what was published; the failure paths are run too, so
a target that has been deleted or edited since is reported rather than silently landed on. The
demonstration uses a synthetic two-version tree in `tmp_path`, because the real tree holds exactly one
version of every reference, which is itself worth recording: a republication of
`rubric/practice-default@1.1.0` could not name a resolvable predecessor today, since no 1.0.0 of it
exists on disk. The registry's half of rollback, the pointer move, the deprecation and the
immutability trigger, is enforced and tested in the control plane and is deliberately not
reimplemented here.

**What is not true, and should not be read into the ticks.** Nothing in Go's `contentctl` calls this
gate. CI refuses the change before it can be deployed, so no artifact reaches the tree without a
record, but somebody running the publishing tool by hand against a database would still insert a row
that never passed here. Wiring the gate into the loader is Go work and is outside this service. No
publication record exists yet for any real artifact: all nine predate the gate and are grandfathered
by digest, because writing a record for them now would mean naming an approver for an approval that
never happened. Tenant-authored artifacts never appear in this tree and this gate never sees them.

**One QUA-02 defect fixed on the way, because this gate reads the report.** `evals/reports/latest.json`
was described as the committed artifact, required by two named tests, and listed in `.gitignore`, so a
clean checkout could not even collect the Python suite. It is now tracked, and the per-run timing that
was the reason for ignoring it is written to a sibling `latest.timing.json` which is not, so a rerun
leaves the tree clean. The report also now carries the date it was generated, outside the results
digest beside timing, because a gate cannot check an age it cannot see.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md)

---

### QUA-05 · Build fairness and assessability monitoring

**Depends on** QUA-02, ART-02, DEC-13 · **Blocks** REL-03

Watch transcription quality, assessability, insufficiency, interruption, accommodation completion,
override and re-review rates for patterns that indicate a group is being disadvantaged.

**Done when**
- [ ] Metrics are monitored by device, audio condition and supported-matrix boundary.
- [ ] Demographic attributes are not collected casually; any collection has a lawful purpose and governance first.
- [ ] A concerning pattern has a defined escalation path, not just a dashboard.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md)

---

### QUA-06 · Monitor AI quality in production with alerting and rollback

**Depends on** QUA-04, PLT-08 · **Blocks** OPS-04

Fixtures passing in CI does not mean production is healthy. Watch invalid output, schema failures,
unsupported facts, latency and cost against live traffic.

**Done when**
- [ ] Live quality metrics alert on regression, tied to the artifact version that caused it.
- [ ] Rollback can be triggered from the alert.
- [ ] A quality freeze can be declared and holds until it is lifted deliberately.

**Spec** [observability.md](../../operations/observability.md)
