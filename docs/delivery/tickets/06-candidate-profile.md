# Epic PRO — Candidate profile and documents

**Phase 3** · **Workstream** Go, Python, Web

The candidate's own record: profile, CV, job context, goals, preferences, accommodations and consent.
Extraction is assistive, never authoritative — every extracted fact keeps its provenance and the
candidate can correct it.

---

### PRO-01 · Implement candidate profile, career context and preferences

**Depends on** IAM-01, CTR-01 · **Blocks** CAT-03

Disciplines, target roles, seniority, career context, interview defaults, accessibility preferences and
notification settings.

**Done when**
- [x] Profile reads and writes are scoped to the owning candidate and nobody else.
- [ ] Accessibility preferences set here are honoured by the prepare and live screens by default.
- [x] Partial profiles are usable; nothing is blocked behind a completeness score.

**Backend and contract complete.** The profile lives in the candidate schema, where IAM-06's
structural guards apply by existing, and the leak hunt for this table found a real gap in the
schema's own discipline: the owner policies lacked the tenant-absence clause, so the owner's
rows were readable through a code path that also set tenant context. Both candidate tables now
carry the clause, the structural guard demands it of every future one, and the adversarial matrix
pins the exact shape that leaked. Owner scoping at the HTTP surface is the absence of a
parameter: /me/profile has no way to name anybody else. The empty profile is the valid zero
value, first reads return it rather than an error, and every bound refuses with a field-level
code. The middle box stays open until SES-03's prepare screen exists to do the honouring; the
columns are explicit and the contract exposes them so that screen has one obvious source.

**Spec** [product-requirements.md](../../product/product-requirements.md)

---

### PRO-02 · Implement CV upload, versioning, replacement and deletion

**Depends on** PLT-05, PRO-01 · **Blocks** PRO-03

Upload with size and type bounds, content digest, version history, and deletion that does not silently
rewrite a session already composed from an earlier version.

**Done when**
- [x] Upload, replace and delete all work, with the digest and version recorded per document.
- [x] Deleting or replacing a CV leaves an already-composed session bundle untouched.
- [x] Failed and partial uploads have their own recoverable states.

**Done.** Browser-direct upload against presigned URLs - the integration suite PUTs real bytes at
LocalStack exactly as a browser would - with every upload a new version and the row as the
authoritative record per data-architecture.md. Rows outlive their objects: after replace and
delete, the old version's record and digest still answer, which is how a bundle that pinned it
stays reconstructable, and bundles themselves are written only by the ready transition and frozen
by trigger, so nothing here can touch one. The states are real: uploading, stored, failed - abort
gives a stalled upload its visible ending, completing a corpse is refused, and recovery is simply
the next version. Bounds refuse by name: three media types, 10 MiB, four parts. Another person's
documents do not exist even by id.

**Spec** [data-architecture.md](../../architecture/data-architecture.md)

---

### PRO-03 · Extract structured facts from CV and job description with provenance

**Depends on** PRO-02, CTR-02 · **Blocks** PRO-04

Python extracts roles, dates, skills and achievements. Every fact records the source span it came from,
a confidence, the extraction version, and its processing status.

**Done when**
- [ ] Each fact links to the exact source span that produced it.
- [ ] Text the extractor could not parse is surfaced honestly rather than dropped.
- [ ] Extraction failure degrades to a manually completed profile rather than blocking the journey.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### PRO-04 · Let candidates inspect and correct extracted facts

**Depends on** PRO-03, WEB-04 · **Blocks** nothing

*Gap found against the prototype: the profile shows source tags but no per-fact confidence, source span
or correction state.*

**Done when**
- [ ] Every extracted fact shows its source span, its confidence and whether it has been corrected.
- [ ] Correcting a fact records the correction without destroying the original extraction.
- [ ] A corrected fact is the one used in composition from that point forward.

**Spec** [product-requirements.md](../../product/product-requirements.md)

---

### PRO-05 · Build the private practice evidence bank

**Depends on** PRO-01, EVL-04 · **Blocks** PRG-01

The candidate's accumulated examples and evidence across sessions, owned by them, excluded from employer
access by construction rather than by filtering.

**Done when**
- [ ] The evidence bank is readable only by the owning candidate.
- [ ] It is stored in a projection that tenant authority cannot reach, verified by IAM-06 tests.
- [ ] The candidate can delete individual items and understand the consequence for progression.

**Spec** [practice-mode.md](../../product/practice-mode.md)
