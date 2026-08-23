# Epic EVL — Evaluation and evidence

**Phase 3** · **Workstream** Python, Go, AI/data quality

Every material conclusion cites evidence. Unknown, insufficient and unassessable are distinct from low
performance, and validation happens in Go before anything is published — a model cannot talk its way
past the schema.

---

### EVL-01 · Build the evidence extraction pipeline

**Depends on** SES-04, RTC-04, CTR-02 · **Blocks** EVL-02

Turn a sealed transcript into evidence spans linked to competencies, with timestamps that resolve back
to the audio.

**Done when**
- [ ] Every evidence span carries its source segment, timing and extraction version.
- [ ] A span always resolves to real transcript text; fabricated spans fail validation.
- [ ] The stage is independently retryable without duplicating evidence.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### EVL-02 · Implement rubric-based competency evaluation as a durable workflow

**Depends on** EVL-01, CAT-01, PLT-06 · **Blocks** EVL-03

Evaluate against the rubric and calibration pinned in the session bundle, never against whatever is
currently published.

**Done when**
- [ ] Evaluation uses only the pinned artifact versions, proven by reconstructing an old session.
- [ ] Retry produces one result, one usage record and one notification.
- [ ] Model, prompt and policy versions are recorded on the result.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### EVL-03 · Implement sufficiency, coverage and the insufficient-evidence outcome

**Depends on** EVL-02 · **Blocks** EVL-05, REV-02

Decide, per competency, whether there is enough evidence to say anything at all — and represent "not
enough" as its own outcome rather than as a low score.

**Done when**
- [ ] Insufficient evidence is a distinct state everywhere it appears: API, storage, and every surface.
- [ ] Coverage reports what the conversation reached and what it did not.
- [ ] No aggregate score is computed in a way that treats unassessed as zero.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md) · [responsible-hiring.md](../../security/responsible-hiring.md)

---

### EVL-04 · Detect unverified claims and contradictions neutrally

**Depends on** EVL-01 · **Blocks** PRO-05, REV-02

Surface claims that could not be verified and statements that conflict — framed as things to ask about,
never as accusations of dishonesty.

**Done when**
- [ ] Both sides of a contradiction are quoted with timestamps.
- [ ] Copy states that unverified does not mean untrue, everywhere the concept appears.
- [ ] No honesty, integrity or credibility inference is produced anywhere in the pipeline.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md)

---

### EVL-05 · Implement confidence semantics and publication validation

**Depends on** EVL-03, DEC-12, QUA-03 · **Blocks** PRC-01, REV-02

Go validates every evaluation before publication: schema, evidence references, sufficiency consistency,
prohibited inference, and mode visibility.

**Done when**
- [ ] An evaluation with a dangling evidence reference cannot be published.
- [ ] Confidence is computed from a defined, documented basis rather than asserted by a model.
- [ ] A validation failure produces a visible, retryable state rather than a published bad result.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### EVL-06 · Map job requirements to evidence without a match percentage

**Depends on** EVL-01, SCR-03 · **Blocks** REV-02

Each job requirement is reported as evidenced, partial, not discussed, or not assessable — with the
evidence attached and no headline compatibility number anywhere.

**Done when**
- [ ] Requirements are reported per requirement, with evidence, never as a single percentage.
- [ ] "Not discussed" and "not assessable" are distinguishable from "not evidenced".
- [ ] Missing evidence produces suggested human follow-up questions.

**Spec** [screen-mode.md](../../product/screen-mode.md)

---

### EVL-07 · Handle evaluation failure, partial results and budget exhaustion

**Depends on** EVL-02, DEC-10 · **Blocks** REL-02

An optional coaching stage failing must not erase a valid evaluation, and budget exhaustion must not
quietly produce a worse result for the candidate.

**Done when**
- [ ] A failed optional stage leaves the core evaluation intact and published.
- [ ] Budget exhaustion omits optional narrative but retains the deterministic result and its status.
- [ ] Terminal and retryable failures are distinguishable to both the operator and the candidate.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)
