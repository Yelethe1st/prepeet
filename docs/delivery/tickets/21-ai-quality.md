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

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### QUA-04 · Gate artifact and model publication on an evaluation report

**Depends on** QUA-02, CAT-01 · **Blocks** REL-02

No prompt, model or rubric reaches production without a report, an approver and a rollback plan.

**Done when**
- [ ] Publication is blocked without an attached evaluation report.
- [ ] The approver is a named person and cannot be the author for a material change.
- [ ] Rollback is demonstrated, not assumed.

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
