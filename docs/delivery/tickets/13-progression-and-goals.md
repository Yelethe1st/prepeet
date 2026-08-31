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
- [x] A goal can be created from a gap, a drill or a competency, and tracks real progress.
- [x] Streaks encourage without shaming, and losing one is not framed as a failure.
- [x] Goal state survives a rubric version change.

**Done in the domain and the schema. There is no goals screen, and the last paragraph says why.**

`progression.goals` and `progression.goal_milestones` are migration 0051. Origin is a column with
three values, so a goal raised from a readiness gap, one adopted from a drill and one chosen
outright are different stories a screen can tell rather than the same row. Progress is derived from
the same observations readiness reads, by the same rule for choosing a reading, so a goal and a
readiness view cannot disagree about one competency; a goal with no reading is `not_started` with a
reason, never a zero.

Surviving a rubric version change is why a goal pins its own band scale and rubric reference at
creation. A version bump inside one reference is the same measurement restated and keeps counting. A
different reference measures something else and is reported as incomparable, which is neither a
reset nor a silent substitution. Milestones are the durable half: append-only, one per band, each
naming the observation and the rubric version that earned it, so a later publication cannot take
away what somebody already did. Attacked by editing a goal's competency, which the trigger refuses,
because a goal whose subject moved would re-date every milestone under it.

Cadence is derived from the observation history and stored nowhere. There is no streak column, no
missed count, no target and no last-nagged-at, and the `Cadence` type has no field that counts
something the candidate did not do, so nothing above it can render a reproach. It is weekly rather
than daily, and by week rather than by session, because a daily streak punishes an ordinary life and
counting sessions rewards cramming. The week in progress never ends a run: a Monday morning with
nothing done yet leaves last week's standing. A lapsed run reads as resting, with the longest run
kept, and a candidate who has never practised is not resting, because resting implies something to
rest from.

`progression.goals` has no tenant column and one policy keyed to the person, so no employer
authority reaches a goal. Proven by attacking with the owner's own user id and a tenant set, which
is the only version of that attack that tests the tenant clause: the first version set the tenant
alone, was refused by the candidate clause, and passed with the tenant clause deleted. Thirteen
guards in all were removed one at a time and each failed a named test before being restored.

**What is not wired.** There is no goals screen and no endpoint. The OpenAPI document declares
nothing for progression and was outside this change, so a candidate cannot create a goal or see one;
`/candidate/goals` in the information architecture has no route behind it. Nothing calls `TrackGoals`
on a schedule either, so milestones accrue when something asks rather than when an evaluation
publishes. The three boxes are ticked for the domain, the schema and the store, which is what could
be finished without the contract.

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

**Not started, and deliberately not half started.** All three boxes are about screens, and no screen
was built. The web app types every feature from `@contracts`, and the OpenAPI document declares no
progression endpoint at all: not readiness, not goals, not skills. Adding one means editing
`packages/contracts`, which was outside this change. Building the screens against locally declared
types instead would break ADR-0004's rule that the contract is the source, and would leave three
ticked boxes above a screen with nothing behind it.

What exists is the reading the screens would render. `progression.Freshness` classifies a reading as
fresh, aging, stale or none, with the age in days, and `none` is separate from `stale` for the same
reason unassessed is separate from a low band: never measured and measured long ago are different
facts. Goal progress carries the freshness of what resolved it, and PRG-05 already refuses to treat
a six-month-old reading as current. The thresholds are 30 and 90 days and are deliberately coarse; a
finer scale would imply a precision the evidence does not have.

**Spec** [information-architecture.md](../../product/information-architecture.md)

---

### PRG-05 · Use prior gaps to inform future session composition

**Depends on** PRG-02, CAT-02 · **Blocks** nothing

A candidate's unassessed and weak competencies shape what the next interview asks about, without turning
practice into a narrow loop.

**Done when**
- [x] Composition can accept a targeted gap and demonstrably covers it.
- [x] Targeting never becomes the only thing asked about.
- [x] The recommendation explains why it was made.
- [x] Sparse, stale or incompatible history produces a cautious recommendation rather than an invented trend.

**Done as a recommendation and a port. Nothing consumes it yet, and the last paragraph says so.**

`progression.Targeting` answers which competencies the next session should cover and why. The
catalogue says what a role could be asked about at all, through `RoleCompetencies`, a port declared
in progression and satisfied in `cmd/api` because ADR-0005 forbids progression importing the
catalogue. The adapter derives identifiers with `catalog.CompetencyID`, the same derivation
evaluation uses, because a recommendation naming "Systems design" could never be shown to have been
covered by an observation of `systems-design`. That is nine lines of mapping, which is the point.

At least one slot is always something the history did not ask for, and `reservedSlots` is the shape
of the answer rather than a tuning parameter. Without it a candidate weak at five things would be
asked about the same five until they were not, and a competency that has quietly regressed since it
was last strong would never come up again. `Covers` and `Targeted` stay two lists so that "targeting
is not the whole session" is something a test can check rather than a promise.

Every target carries a machine-readable reason and the sentence a candidate reads, held together so
two screens cannot explain one recommendation two ways. A competency nobody has been asked about is
targeted without being given a band, which is the unassessed rule carried into recommendations, and
it outranks a weak one because the purpose is to reduce what is unknown rather than to drill what is
already measured. A reading old enough to be stale earns a revisit rather than a pass.

Caution has three shapes and three sentences: no history at all, too little to be a pattern, and a
history that exists but was produced under another rubric. Caution never becomes silence, because a
first-run candidate still gets a full session. Thirteen guards were removed one at a time; two
passed with the guard gone, both because the test never reached the branch, and both were rewritten
until they failed as they should.

**What is not wired.** Nothing consumes a recommendation. Interview composition takes its
competencies from the session's configured role, and there is no endpoint through which a candidate
could ask for one, for the same contract reason as PRG-04. "Composition demonstrably covers it" is
ticked for the recommendation and the port, which is the half this ticket owns; wiring it into the
session bundle is interview's and needs a contract change.

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
- [x] The system rejects or safely reframes a requirement that asks for personality, emotion, accent quality, inferred confidence or another prohibited inference.
- [ ] Evaluation reports achieved, partially achieved, not demonstrated or not assessable for every selected requirement, with resolving evidence and criterion version.
- [x] A session that offered no fair opportunity to demonstrate a criterion returns not assessable and never a zero or failure.
- [x] Results state which criteria were demonstrated, what was missing and one or two concrete next actions where useful.
- [x] Progress metrics expose their definition, evidence, sufficiency, version and comparison basis; incompatible sessions remain separate.
- [x] Optional pre/post confidence is candidate self-report stored separately from evaluated observations.
- [ ] Prior observations produce explainable next-session suggestions and drills, and the candidate can inspect, stop using, export or delete the private history.
- [x] No personal requirement, observation, metric or recommendation is reachable through employer authority.

**Six of ten. The four open ones need a session and a contract, and each says which.**

Migrations 0052 and 0053 and `internal/progression/requirements.go`. Goals and personal requirements
live in progression rather than in the candidate profile, against what the domain model said;
[ADR-0021](../../architecture/decisions/0021-goals-and-personal-requirements-live-in-progression.md)
records the deviation and the model is updated in the same change.

**Prohibited inference.** `Resolve` checks prohibitions before the catalogue and always, so a request
that mixes an observable behaviour with a prohibited one is not half accepted. A protected
characteristic is refused outright and never reframed, because reframing implies the request was
reasonable and merely badly worded. A request with a recoverable observable intent, "sound more
confident" being the one that matters, is reframed to the behaviour underneath it and says plainly
what it declined to assess; the reframing is stored on the requirement, because one shown once at
creation is a substitution the candidate cannot go back and check. A request the catalogue cannot
read asks for clarification rather than inventing criteria. There is no path from an intent to a
criterion that does not go through `Resolve`.

**Not assessable.** `opportunity` is a separate argument to `Judge` rather than something inferred
from the findings, because "we asked and they did not" and "we never asked" produce identical
findings and opposite outcomes, and a function that had to guess would eventually guess wrong in the
direction that hurts a candidate. `Score` returns nothing rather than zero for anything unassessed,
so a metric cannot average silence into a decline. Four CHECK constraints hold the same line in the
schema: a not-assessable row must state why and must list nothing as demonstrated or missing, and
the three assessed outcomes each have to mean what they say. An achievement claimed without evidence
is not counted, which was the guard whose first test never reached it.

**Metrics.** The series key is requirement, criterion version, role and shape together, so two roles,
two shapes or two criterion versions are separate series rather than one flattering average. Every
metric carries its definition, its comparison basis in words, the criterion version, the assessed
count and the sufficiency. The unassessable are counted apart and never as failures. Three sessions
is the sufficiency threshold and deliberately not two, because a line through two points is the
shape most easily mistaken for a trend; below it the metric is still shown with its sufficiency
stated, since hiding it leaves the candidate with nothing when they are most interested.

**Employer authority.** None of these tables carries a tenant column, and each has one policy keyed
to the person with no tenant context set. The claim is therefore structural rather than checked:
there is no employer scope in which a personal requirement exists. Attacked with the owner's own
user id and a tenant set, across requirements and criteria, and paired with the same read at the
same moment with the tenant cleared so the refusal cannot be the row being absent. A second
candidate's DELETE was aimed at four rows known to exist under the first. `confidence_self_reports`
carries no rubric, band, criterion or evidence column, asserted against `information_schema`, so a
self-rating cannot become an evaluated reading by being joined to one.

**Erasure.** These tables admit DELETE, which nothing else in progression does. Practice requirement
history is the candidate's private evidence about themselves: no employer sees it, no decision rests
on it and no audit obligation attaches to it, so the right to erase has nothing to weigh against.
Erasing a requirement cascades to its criteria and its outcomes, because erasure that left the
results behind would not be erasure. UPDATE is still refused: editing a recorded result is not
erasure and has no honest use. Twenty-three guards were removed one at a time; two passed with the
guard gone because their tests never reached the branch, and both were rewritten.

**The four open boxes.**

Create, edit, pause and retire exist in the store and are proven against PostgreSQL, but "select it
for a practice session" does not: selection happens in the interview wizard and the pin goes into
the session bundle, which is interview's, and there is no endpoint either. Same for pinning before
session start: criteria are versioned and immutable per version here, and nothing pins them into a
bundle. Both wait on a contract change and on interview.

Evaluation reporting an outcome is representable and storable, and `Judge` produces the four values
correctly, but nothing produces one: the stage that would judge a personal requirement lives in
`services/intelligence`, which was outside this change, and no worker route projects a requirement
outcome the way `evaluation.completed.v1` is projected into an observation. The box is about
evaluation doing it, so it stays open.

Inspect, stop using, export and delete are all built: `Export` answers everything this context holds
in one transaction, `SetPersonalisation` stops history shaping recommendations without deleting it,
and `EraseRequirement` cascades. What is missing from that box is the first half. Suggestions and
drills from prior requirement outcomes are not built; PRG-05's targeting works from competency
observations only, and no code reads `PersonalisationEnabled` yet, so the switch is stored and
honoured by nothing.

**Spec** [practice-mode.md](../../product/practice-mode.md) · [product-requirements.md](../../product/product-requirements.md) · [domain-model.md](../../architecture/domain-model.md)
