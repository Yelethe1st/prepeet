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
- [ ] Profile reads and writes are scoped to the owning candidate and nobody else.
- [ ] Accessibility preferences set here are honoured by the prepare and live screens by default.
- [ ] Partial profiles are usable; nothing is blocked behind a completeness score.

**Spec** [product-requirements.md](../../product/product-requirements.md)

---

### PRO-02 · Implement CV upload, versioning, replacement and deletion

**Depends on** PLT-05, PRO-01 · **Blocks** PRO-03

Upload with size and type bounds, content digest, version history, and deletion that does not silently
rewrite a session already composed from an earlier version.

**Done when**
- [ ] Upload, replace and delete all work, with the digest and version recorded per document.
- [ ] Deleting or replacing a CV leaves an already-composed session bundle untouched.
- [ ] Failed and partial uploads have their own recoverable states.

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
