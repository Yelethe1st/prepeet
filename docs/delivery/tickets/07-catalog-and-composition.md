# Epic CAT — Catalogue, artifacts and session composition

**Phase 2–3** · **Workstream** Python, Go, Web

Roles, shapes, personas and rubrics come from the server, not from hardcoded lists. Composition turns a
candidate's or recruiter's choices into an immutable bundle that pins every artifact version the session
will use — which is what makes a result reproducible a year later.

---

### CAT-01 · Build the artifact registry with review, publication and rollback

**Depends on** DEC-09, PLT-03 · **Blocks** CAT-02, TEN-04, QUA-04

Rubrics, calibrations, prompts, personas and interview blueprints as versioned, digest-addressed
artifacts with an approval step and a rollback path.

**Done when**
- [ ] An artifact can be drafted, reviewed, published, pinned and rolled back.
- [ ] Published artifacts are immutable; a change creates a new version.
- [ ] Publication never mutates an in-flight or historical session.

**Spec** [domain-model.md](../../architecture/domain-model.md)

---

### CAT-02 · Implement interview composition as a durable workflow

**Depends on** CAT-01, PLT-06, CTR-02 · **Blocks** SES-02

Go accepts the request; a Temporal workflow calls Python to compose the interview plan; the result is
persisted as an immutable bundle with every artifact version and input digest recorded.

**Done when**
- [ ] Composition is idempotent and safe to retry without producing two bundles.
- [ ] The bundle records every pinned artifact version, input digest and policy version.
- [ ] Composition failure is a visible, retryable state rather than a dead session.

**Spec** [session-lifecycle.md](../../architecture/session-lifecycle.md)

---

### CAT-03 · Serve the discipline, role, shape and persona catalogue

**Depends on** CAT-01 · **Blocks** CAT-04

Server-provided metadata with validation and duration limits, so the product is not quietly restricted
to software roles by a hardcoded list.

**Done when**
- [ ] Catalogue endpoints serve disciplines, roles, shapes and personas with their valid combinations.
- [ ] Invalid combinations are rejected server-side with a field-level error, not filtered in the browser.
- [ ] Adding a profession is a data change, not a deployment.

**Spec** [public-api.md](../../contracts/public-api.md)

---

### CAT-04 · Build the practice interview configuration wizard

**Depends on** CAT-03, WEB-04 · **Blocks** SES-01

Role and focus, shape, interviewer, length and difficulty, review and start — URL-addressable, validated
per step, preserving entered data when validation fails.

**Done when**
- [ ] Each step is addressable, restorable and validated independently.
- [ ] Failed validation moves focus to the first problem and preserves everything already entered.
- [ ] The wizard refuses to compose a screening interview, which only a recruiter can create.

**Spec** [practice-mode.md](../../product/practice-mode.md)

---

### CAT-05 · Collect recording preference and practice consent at composition

**Depends on** CAT-04 · **Blocks** SES-03

*Implemented in the prototype; carry it into production.* The candidate chooses what is kept — audio and
transcript, or transcript only — and understands what each choice costs them.

**Done when**
- [ ] The recording preference is stored on the session and honoured by RTC-05.
- [ ] Choosing transcript-only visibly forfeits replay and delivery measurement for that session.
- [ ] The preference is versioned with the consent text presented alongside it.

**Spec** [practice-mode.md](../../product/practice-mode.md) · [retention-and-deletion.md](../../security/retention-and-deletion.md)

---

### CAT-06 · Build the content authoring and publication-approval surface

**Depends on** CAT-01, IAM-04 · **Blocks** nothing

*Gap found against the prototype: the content-author and publication-approver roles exist in the
authorization model, but no screen lets anyone author, test or approve an interview artifact.*

**Done when**
- [ ] A content author can draft a blueprint, question set or persona and test it against fixtures.
- [ ] A separate approver publishes; the author cannot approve their own artifact.
- [ ] Publication records approver, time, digest and the artifacts it supersedes.

**Spec** [authorization-model.md](../../architecture/authorization-model.md) · [domain-model.md](../../architecture/domain-model.md)
