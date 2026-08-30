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
- [x] Observations are append-only; a correction adds a record rather than editing one.
- [x] Each observation carries the rubric and calibration version that produced it.
- [x] Re-evaluation under a new rubric is representable without destroying the earlier view.

**Done.** progression.observations is the new context's first table: one row per evaluated
competency per evaluation, projected in the worker from evaluation.completed.v1 (composed with
the review_ready transition; both idempotent, so a redelivery converges - proven with three
appends leaving exactly one history). Append-only is a trigger, attacked with UPDATE and DELETE
from inside the owner's scope; a correction is a new row whose supersedes names its predecessor,
and the predecessor is proven unchanged.

Provenance is the full pin: rubric reference, version and digest, aggregation and extraction
versions, model and policy versions (the honest none at today's floor - "calibration version"
resolves to the rubric version until QUA-03 exists) ride every row, so any historical point on a
future chart reconstructs against exactly what judged it. Re-evaluation is structural: a second
evaluation of the same session and competency under rubric 2.0.0 adds its reading beside the
1.1.0 one, both readable, ordered, the earlier untouched. Unassessed observations are stored
too, deliberately: a chart that dropped them would read silence as decline. The history read
model and the progression screens are PRG-02 and PRG-03's.

**Spec** [domain-model.md](../../architecture/domain-model.md)

---

### PRG-02 · Compute readiness against a pinned role standard

**Depends on** PRG-01, CAT-01 · **Blocks** PRG-04

Readiness is measured against a specific, versioned role standard — never a floating average across
incomparable roles.

**Done when**
- [x] Readiness names the role standard and version it was computed against.
- [x] Progression is grouped by role and discipline, and incomparable roles are never averaged.
- [x] Assessed and unassessed competencies stay visibly distinct.

**Done as a calculation and a read model. Nothing is wired to it yet, and the last section says why.**

`progression.readiness_snapshots` and `progression.readiness_competencies` are migration 0046's two
tables, and the calculation above them is pure: `Compute` takes one pinned standard and the
candidate's history and answers one readiness. The pin travels inside the answer rather than beside
it, so a figure cannot be produced or stored without the reference, version and digest that produced
it. `ParseStandard` refuses a standard that cannot name itself, `Compute` refuses one built by hand
that cannot, and the schema refuses the row. Each of the three was proven by removing it and watching
its own named test fail.

Two roles are two answers at every layer. `ComputeAll` returns a list ordered by discipline then role
and refuses two standards claiming one role, because ambiguity about which answer belongs to a role
is how averaging starts. There is no function that combines two readinesses, no column that could
hold a combined figure and no table for one, so incomparable roles cannot be averaged for want of
anywhere to put the average. The test that matters is the unglamorous one: a backend engineer's
strong systems-design reading does not answer an engineering manager's standard, though both are
software engineering. Comparability includes the rubric, per the evaluation spec, so a reading
produced under another rubric reference is reported as incomparable rather than counted.

Assessed and unassessed are mirror shapes rather than a flag. An unassessed requirement carries no
band, no resolving observation and no date, and must state why: `never_observed`, `not_assessed`,
`incomparable_rubric` or `incomparable_band`, because a competency that has never come up asks for a
different next session from one measured under a rubric this standard cannot be compared with. Four
CHECK constraints hold the mirror in both directions, so a met requirement with no reading is refused
as firmly as an unassessed one wearing a band.

The met, below and unassessed totals are deliberately not stored. A count beside the rows it
summarises is a count that can stop agreeing with them, and the disagreement that would matter here
is exactly the invisible one, an unmeasured competency shown as a pass. They are derived from the
requirement rows when a readiness is computed and again when one is read back, by the same function.
The first attempt did store them, with a CHECK that they summed to the requirement count; writing the
attack showed that check passing while the lie went through, which is how the column came out.

Recomputation converges instead of accumulating. A snapshot's identity is a digest of the pin and
every resolved requirement, so an unchanged answer is the row already written and a changed answer
appends beside it, which makes the history a record of what changed rather than of how often somebody
looked. Append-only by trigger, attacked with UPDATE and DELETE from inside the owner's scope.
Row-level security attacked from a second tenant, aimed at a snapshot id known to exist under the
first: zero rows for the snapshot, zero for its requirements, and nothing from the store; the same
for a second practice candidate. Sixteen guards in all were broken deliberately and each failed a
named test before being restored.

**What is not wired.** No `role_standard` artifact has been published: `services/intelligence/artifacts`
holds none and was outside this change, so the document shape is defined and parsed here but nothing
resolves one yet. `contentctl` does not validate the type, no worker route computes a readiness when
an evaluation publishes, and `GET /api/v1/me/readiness` remains only in the contract. The boxes above
are ticked for the calculation, the schema and the read model, which is what this ticket owns. A
candidate cannot see readiness until PRG-04 builds the screen, and nothing will populate it until a
standard exists to pin.

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
