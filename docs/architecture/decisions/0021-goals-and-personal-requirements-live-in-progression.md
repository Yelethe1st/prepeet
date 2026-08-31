# ADR-0021: Goals and personal requirements are progression's, not the candidate profile's

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-31  
**Review date:** 2027-02-28  
**Supersedes:** None  
**Superseded by:** None

Recorded because it is a deviation from a binding document.
[domain-model.md](../domain-model.md) lists `Goal` and `PersonalRequirement`
as aggregate roots of the Candidate context, and PRG-03 and PRG-06 build
both in Progression instead.

## Context

The domain model's Candidate context owns the profile: documents,
extracted facts, target roles, consent, accessibility preferences. Its
invariants are about a person's identity and the material they supply, and
its listed child records include "goals, personal requirements" alongside
those.

Both tickets turned out to be about measurement rather than about the
profile. A goal is a band on a scale, resolved from the same observations
readiness reads, and its milestones name the observation and the rubric
version that earned them. A personal requirement resolves to criteria that
are pinned before a session and answered by an outcome carrying a criterion
version, evidence references and an assessability verdict. Neither can be
computed, stored or read without the observation history, the rubric
reference that makes readings comparable, and the freshness of the evidence
behind them.

ADR-0005 forbids a context importing another and
`internal/architecture`'s ownership rule forbids a module naming another
module's schema. Placing goals and personal requirements in Candidate would
therefore have meant one of three things: candidate reaching progression's
tables, which the ownership test fails; a port from candidate to
progression carrying observations, freshness, comparability and band scales,
which is most of progression's surface exported through a seam that exists
only to satisfy a table in a document; or the tracking logic living in
progression while the rows live in candidate, which puts one invariant on
two sides of a boundary.

## Decision

**`progression.goals`, `progression.goal_milestones`,
`progression.personal_requirements`, `progression.requirement_criteria`,
`progression.requirement_outcomes`, `progression.confidence_self_reports`
and `progression.personalisation` are Progression's, in Progression's
schema, owned by `internal/progression`.**

Progression's aggregate roots become CompetencyHistory, ReadinessSnapshot,
Goal and PersonalRequirement. The Candidate context keeps the profile, the
documents, the extracted facts and the consent, which are about the person
rather than about what has been measured of them.

Three consequences are accepted deliberately:

- **The candidate profile does not hold a goal.** A screen showing profile
  and goals together reads two contexts, which is what an API composition
  layer is for and what the catalogue surface already does.
- **Practice data stays candidate-owned rather than tenant-owned.** None of
  these tables carries a `tenant_id`, and each has exactly one policy, keyed
  to the person with no tenant context set. PRG-06 requires that no personal
  requirement is reachable through employer authority; this makes it true by
  there being no employer scope in which the rows exist, rather than by a
  check somebody performs.
- **`RequirementObservation` remains Evaluation's**, as the domain model
  says. Evaluation produces the finding; progression projects it into a
  `requirement_outcome` exactly as it already projects a competency
  observation. The split the model draws between producing a reading and
  keeping a history is preserved.

## Alternatives considered

**Follow the document and put both in Candidate.** Rejected for the three
consequences above: it needs either a boundary violation the build already
fails, or a port wide enough to export most of progression, or an invariant
split across two contexts. The document's placement reads as a listing of
what a candidate owns rather than as a claim about which module computes
it, and the tickets themselves sit in the PRG epic.

**Put them in Candidate and move readiness there too.** Rejected: readiness
is a projection over evaluation output and belongs beside the observations,
and moving it would make Candidate the context that both holds a person's
identity and judges them, which is the coupling the model separates on
purpose.

**A new context for personal targets.** Rejected as a third place holding
half of one subject. Goals, requirements and observations are read together
on every screen that uses any of them.

## Consequences

`docs/architecture/domain-model.md` is updated in the same change: the
Progression row lists the four roots, the Candidate row drops Goal and
PersonalRequirement, and the profile's child-record paragraph says where
they went. A reader of the model should not have to find this ADR to learn
that the model moved.

The privacy consequence is the one worth restating. These tables admit
`DELETE` where nothing else in progression does, because practice
requirement history is the candidate's private evidence about themselves:
no employer sees it, no decision rests on it and no audit obligation
attaches to it, so a candidate's right to erase has nothing to weigh
against. Screening observations are the opposite case and stay append-only.

## Review

Revisit if a tenant is ever given a legitimate view of a candidate's
personal requirements, which would mean these tables need a tenant
dimension and the placement argument changes. Nothing currently planned
does, and ADR-0018's isolation guarantee is candidate-facing copy that says
so.
