# Epic PRG — Skills, progression, goals and readiness

**Phase 3** · **Workstream** Go, Python, Web

Longitudinal candidate value. Observations are append-only and carry the rubric or personal-criterion
version that produced them, so a change never silently rewrites a candidate's history. Roles, formats,
rubrics, and criteria are not averaged together just because they produced numbers.

---

### PRG-01 · Store append-only competency observations with rubric provenance

**Depends on** EVL-05 · **Blocks** PRG-02, ART-07

Every observation records its evidence, its rubric version and when it was made. Nothing is updated in
place.

**Done when**
- [ ] Observations are append-only; a correction adds a record rather than editing one.
- [ ] Each observation carries the rubric and calibration version that produced it.
- [ ] Re-evaluation under a new rubric is representable without destroying the earlier view.

**Spec** [domain-model.md](../../architecture/domain-model.md)

---

### PRG-02 · Compute readiness against a pinned role standard

**Depends on** PRG-01, CAT-01 · **Blocks** PRG-04

Readiness is measured against a specific, versioned role standard — never a floating average across
incomparable roles.

**Done when**
- [ ] Readiness names the role standard and version it was computed against.
- [ ] Progression is grouped by role and discipline, and incomparable roles are never averaged.
- [ ] Assessed and unassessed competencies stay visibly distinct.

**Spec** [practice-mode.md](../../product/practice-mode.md)

---

### PRG-03 · Build goals, milestones and practice cadence

**Depends on** PRG-02, WEB-04 · **Blocks** PRC-04

Targets a candidate sets, milestones they pass, and a cadence that encourages practice without becoming
punitive.

**Done when**
- [ ] A goal can be created from a gap, a drill or a competency, and tracks real progress.
- [ ] Streaks encourage without shaming, and losing one is not framed as a failure.
- [ ] Goal state survives a rubric version change.

**Spec** [product-requirements.md](../../product/product-requirements.md)

---

### PRG-04 · Build the skills and progression screens with evidence freshness

**Depends on** PRG-02, WEB-04 · **Blocks** nothing

Competency evidence, trend over time, evidence freshness, readiness by role, and a route from a gap
straight into a session that targets it.

**Done when**
- [ ] Every competency can be expanded to the evidence behind it, with its date.
- [ ] Stale evidence is visibly stale rather than silently counted as current.
- [ ] Charts carry text summaries and table alternatives.

**Spec** [information-architecture.md](../../product/information-architecture.md)

---

### PRG-05 · Use prior gaps to inform future session composition

**Depends on** PRG-02, CAT-02 · **Blocks** nothing

A candidate's unassessed and weak competencies shape what the next interview asks about, without turning
practice into a narrow loop.

**Done when**
- [ ] Composition can accept a targeted gap and demonstrably covers it.
- [ ] Targeting never becomes the only thing asked about.
- [ ] The recommendation explains why it was made.
- [ ] Sparse, stale or incompatible history produces a cautious recommendation rather than an invented trend.

**Spec** [practice-mode.md](../../product/practice-mode.md)

---

### PRG-06 · Let candidates define and measure personal interview requirements

**Depends on** CAT-02, EVL-05, PRG-01, WEB-04 · **Blocks** nothing

A practice candidate chooses what they want a session to test — for example greeting, introduction,
answer structure, technical trade-offs, questions or closing — and sees cumulative, actionable progress
without inviting the system to infer confidence or another prohibited trait.

**Done when**
- [ ] A candidate can create, edit, pause and retire a personal requirement and select it for a practice session.
- [ ] Candidate-authored intent resolves to candidate-visible, versioned, observable criteria that are pinned before session start.
- [ ] The system rejects or safely reframes a requirement that asks for personality, emotion, accent quality, inferred confidence or another prohibited inference.
- [ ] Evaluation reports achieved, partially achieved, not demonstrated or not assessable for every selected requirement, with resolving evidence and criterion version.
- [ ] A session that offered no fair opportunity to demonstrate a criterion returns not assessable and never a zero or failure.
- [ ] Results state which criteria were demonstrated, what was missing and one or two concrete next actions where useful.
- [ ] Progress metrics expose their definition, evidence, sufficiency, version and comparison basis; incompatible sessions remain separate.
- [ ] Optional pre/post confidence is candidate self-report stored separately from evaluated observations.
- [ ] Prior observations produce explainable next-session suggestions and drills, and the candidate can inspect, stop using, export or delete the private history.
- [ ] No personal requirement, observation, metric or recommendation is reachable through employer authority.

**Spec** [practice-mode.md](../../product/practice-mode.md) · [product-requirements.md](../../product/product-requirements.md) · [domain-model.md](../../architecture/domain-model.md)
