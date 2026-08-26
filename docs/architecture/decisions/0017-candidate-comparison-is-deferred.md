# ADR-0017: Candidate comparison does not ship; deferred with reopen triggers

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-26  
**Review date:** 2027-05-26  
**Supersedes:** None  
**Superseded by:** None

Closes DEC-17 by the ticket's own second path: comparison is explicitly
deferred, recorded here so it is a decision with triggers rather than a
question nobody answered.

## Context

[responsible-hiring.md](../../security/responsible-hiring.md) already
holds comparison off by default and, if ever approved, constrains it to
same role and comparable rubric, two to four candidates, coverage and
uncertainty shown, no ranking, explicit indistinguishable outcomes, and
mandatory individual evidence inspection. REV-05 is the only ticket
gated. The question was whether to invest in building that constrained
feature for launch.

## Decision

**Comparison is deferred.** REV-05 is not built, and no surface may
approximate it: no side-by-side evaluation rendering, no cross-candidate
sorting by any evaluation-derived value, no export shaped to enable
offline ranking. The recruiter review surface (REV-02) presents one
candidate's evidence at a time, which is the workflow
responsible-hiring's constraints describe anyway.

Reasoning: before QUA-03 publishes calibration evidence, two candidates'
bands are not comparable measurements even within one rubric, and a
comparison surface would assert they are. The differentiation value at
launch is small; the fairness and legal exposure is the largest on the
board. Deferral is not abandonment: the triggers below reopen it.

## Reopen triggers

All three, together:

1. QUA-03 has published calibration with measured inter-rater agreement.
2. DEC-11's determination is closed in every launch jurisdiction, and
   comparison is lawful in each.
3. A paying tenant has asked for it concretely enough to name the
   decision it would inform.

Reopening means a revision of this ADR carrying responsible-hiring and
legal approval, and REV-05 built to the constraints already written.

## Consequences

- REV-05 leaves the near-term board; REV-02 stays single-candidate.
- Sales conversations get an honest answer with a reason, which ages
  better than a rushed ranking screen.
