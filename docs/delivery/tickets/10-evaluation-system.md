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
- [x] Every evidence span carries its source segment, timing and extraction version.
- [x] A span always resolves to real transcript text; fabricated spans fail validation.
- [x] The stage is independently retryable without duplicating evidence.

**Done, at the honest floor, with the honesty gate independent of the extractor.** Completion
now writes the sealed evaluation-input document (turns plus the role's competencies, resolved
through the catalogue in cmd) to the object store under one shared key derivation, and records
its digest on the seal. The worker turns session_completed into the evidence workflow
exactly-once; its adapter presigns the sealed input's own key, Python fetches through the grant
and verifies the digest before reading (the same fetch_verified every capability shares), and
evidence-1 extracts deterministically: competency-token matching, measured outcomes as
supporting - including the adjacent-outcome pattern where the act and the number are neighbouring
sentences - admitted uncertainty as a gap, unlinked mentions as claim_unverified, and silence as
nothing, never a low-value span. Spans carry the exact quote, its character range, and a clock
range tightened to the quoted words.

The second box is the part built to outlive evidence-1: Go's Validate holds every span to the
sealed document itself - quote must be the exact slice at its own range, segment must exist,
competency must have been asked about, timing must sit inside the turn - and one fabrication
refuses the whole batch as SCHEMA_VALIDATION_FAILED, published as evaluation.failed.v1, which
cmd routes into the session's evaluation_failed state. When a model replaces the rule set behind
the same contract, this gate is what still stands. The third box is wholesale replacement per
(session, extraction version) inside one transaction: the retried stage is proven to converge on
identical rows, and the spans table refuses UPDATEs so regeneration can never be an edit.
Aggregation into competency results is EVL-02's, reading these rows.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### EVL-02 · Implement rubric-based competency evaluation as a durable workflow

**Depends on** EVL-01, CAT-01, PLT-06 · **Blocks** EVL-03

Evaluate against the rubric and calibration pinned in the session bundle, never against whatever is
currently published.

**Done when**
- [x] Evaluation uses only the pinned artifact versions, proven by reconstructing an old session.
- [x] Retry produces one result, one usage record and one notification.
- [x] Model, prompt and policy versions are recorded on the result.

**Done, at the aggregate-1 floor.** Composition now pins the rubric alongside the plan
(rubric/practice-default through the registry's own lifecycle, validated by contentctl with
evaluation.ParseRubric), so the bundle carries the rubric's reference, version and digest from
the moment the session is made. At evaluation time the workflow's rubric source reads the pin
from the session's own bundle and fetches the body from the registry BY DIGEST - the currently
published version is never consulted, and the integration suite proves the stored result carries
the pin's coordinates, not the registry's head.

Aggregation is a pure Go function (aggregate-1): sufficiency before scoring, unassessed as its
own outcome rather than a low band, bands data-driven from the rubric's ascending floors, an
incoherent rubric refused at parse. The workflow chains extraction into aggregation; the result
row is immutable (proven by attacking the trigger from inside the owner's scope - an unscoped
attack matches zero rows and proves nothing), one per session by unique constraint, with
evaluation.completed.v1 published in the same transaction and routed in cmd to review_ready.
The retry proof stores twice and finds one row and exactly one event; "one usage record" is that
single result row - aggregate-1 consumes no metered provider, so there is no provider usage to
record. Model and policy versions are recorded as the honest literal none until a model stands
behind the same contract; when one does, these columns are where its versions land.

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
