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
- [ ] Each supported profession has fixtures with expected evidence and known edge cases.
- [ ] Fixtures include insufficient-evidence, contradiction and unassessable cases deliberately.
- [ ] Dataset provenance, consent and licensing are documented.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### QUA-02 · Build the automated evaluation harness

**Depends on** QUA-01, EVL-02 · **Blocks** QUA-03

Run a candidate evaluation pipeline against the fixtures and report evidence grounding, unsupported
facts, schema conformance, stability, latency and cost.

**Done when**
- [ ] The harness runs on every prompt, model or rubric change.
- [ ] Unsupported-fact rate is measured, not estimated.
- [ ] Results are comparable across runs and versions.

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
