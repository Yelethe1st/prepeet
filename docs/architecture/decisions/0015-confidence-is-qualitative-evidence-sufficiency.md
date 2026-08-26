# ADR-0015: Confidence is a qualitative evidence-sufficiency label, and it licenses almost nothing

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-26  
**Review date:** 2027-02-26  
**Supersedes:** None  
**Superseded by:** None

Closes DEC-12: what confidence and coverage mean on every surface, what a
reader may conclude from them, and what they may never be read as. Numeric
thresholds are deliberately not set here; QUA-03 sets them from human
benchmark data, which is the whole reason this ADR can exist before that
data does.

## Context

Confidence appears beside evaluation results on candidate and recruiter
surfaces. [evaluation-system.md](../evaluation-system.md) leaves the
semantics open with one firm instruction: until calibrated, use qualitative
evidence sufficiency and consistency, and never show statistical-looking
intervals without a defensible calibrated meaning.
[responsible-hiring.md](../../security/responsible-hiring.md) requires
unknown, insufficient and unassessable to be separated from poor. The risk
this decision exists to remove: a number next to a candidate's name will be
read as a probability that hiring them succeeds, whatever the tooltip says.

## Decision

### What confidence is

**Confidence is a per-competency qualitative label derived mechanically
from stored evidence: `high`, `medium`, `low`, or `not_assessable`.** It
is a statement about how much verifiable evidence the session produced,
never a statement about the candidate.

The derivation is a pure function of values the evaluation result already
records: the supporting-evidence count, the presence of contradictions,
and coverage. Its rules ship inside the versioned rubric artifact, so the
label a session was given is reconstructable forever from its pinned
rubric, exactly like its bands. The initial rules are deliberately crude
and honestly so; QUA-03 replaces them with rules derived from human
agreement data, as a new rubric version, without rewriting any stored
result.

### What coverage is

Coverage is the named account of what the conversation reached and what it
did not, per competency, as built in EVL-03: `reached` and `not_reached`
lists plus counts. Coverage explains confidence; it is display-adjacent
context, never a score input.

### Numbers

**No numeric confidence is displayed anywhere** until QUA-03 publishes
calibrated thresholds with measured inter-rater agreement. When that
happens, the numeric display decision reopens as a revision to this ADR,
not as a silent addition.

### Prohibited interpretations

These are content rules, handed to A11Y-06, binding on every surface that
renders confidence or coverage:

1. **Not a probability of job success.** No copy, chart or layout may
   present confidence as a likelihood the candidate performs well in the
   role.
2. **Not comparable across roles or rubric versions.** Labels from
   different rubrics or roles never appear in the same visual ranking.
3. **Not a ranking.** Confidence never sorts candidates, anywhere.
4. **Absence is not weakness.** `not_assessable` and low coverage render
   as facts about the session, adjacent to what would resolve them, never
   in the visual vocabulary of a low score.
5. **No fabricated precision.** No percentages, intervals, decimal points
   or gauge-style visuals until calibration gives them defensible meaning.

### Surfaces

The server ships the label and its explanation together, the same pattern
EVL-04 set for framing copy: a consumer cannot render the label without
the sentence saying what it is derived from. Candidate and recruiter
surfaces receive the same semantics; recruiter surfaces additionally carry
the decision-support framing DEC-11's determination will finalize.

## Consequences

- EVL-05 can build: publication validation gains a defined confidence
  basis to check, and PRC-01 gains a renderable, explainable label.
- The crude initial derivation will sometimes look conservative. That is
  the correct direction to be wrong in: under-claiming confidence costs
  polish, over-claiming it costs a candidate fairness.
- QUA-03 stays honest: thresholds arrive from measurement, and their
  arrival is visible as a rubric version bump, not a drifted constant.

## Revisit when

QUA-03 publishes calibrated thresholds; any jurisdiction's determination
under DEC-11 constrains recruiter-facing display further; or candidate
research shows the qualitative labels themselves are being read as
probabilities.
