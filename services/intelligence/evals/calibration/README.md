# Calibration

QUA-03's subject: the numeric thresholds the specification deliberately refuses to guess, and what it
would take to set them honestly.

## Nothing here is calibrated

**No threshold in this product has been calibrated against human judgement.** There are no human
benchmark ratings in this repository, so there is nothing to calibrate against. The confidence rules in
`rubric/practice-default@1.1.0` are the crude initial rules
[ADR-0015](../../../../docs/architecture/decisions/0015-confidence-is-qualitative-evidence-sufficiency.md)
describes as deliberately crude, and ADR-0015's prohibition on numeric confidence display remains in
force.

That prohibition is a value this code computes, not a sentence somebody has to remember:

```python
from prepeet_ai.evals.calibration import numeric_confidence_permitted

numeric_confidence_permitted()  # False, and the refusal says why
```

## What is built

- `plan.json`: what a benchmark exercise would have to collect and clear. The acceptance floors, the
  agreement metric, the label scale and its owner and review date. Every floor the calibrator applies
  is read from here rather than written in code, so changing what would count as calibrated is a
  reviewable diff in a document somebody owns.
- `exercise.json`: a synthetic exercise whose three raters are declared rules rather than people. Its
  labels are computed from those rules at runtime rather than stored, so no invented judgement is ever
  committed to this repository as data. It exists to exercise the arithmetic end to end.
- `benchmarks/`: where a real human benchmark set would go. Empty.
- `prepeet_ai.evals.agreement`: observed agreement, Cohen's kappa, Fleiss' kappa and Krippendorff's
  alpha, each checked against hand-computed answers rather than against its own output, and each
  refusing rather than returning a misleading number where the statistic is undefined.
- `prepeet_ai.evals.calibration`: the confidence derivation the product actually uses, a sweep over
  every candidate threshold that would still be a valid rubric, the record a completed calibration
  would publish, and the refusal.

## Two acts, not one

Dropping a file into `benchmarks/` that declares `"rater_provenance": "human"` is not enough. The
plan's owner also has to name that set in `approved_benchmark_sets`. A calibration is a claim that
people were asked and agreed; making it require two deliberate acts is the cheapest defence against a
plausible-looking file becoming a published threshold.

A test also asserts that no file in this directory claims human provenance today. When real ratings
arrive, the person who collected them updates that test, this README, the plan and the ticket. That is
the point: the claim should be impossible to make by accident.

## What the calibrator would do with real data

1. Measure inter-rater agreement on the benchmark set with Krippendorff's alpha, and refuse the set if
   it falls below the plan's floor. Agreement below the floor means the raters do not agree on what the
   evidence shows, and a threshold fitted to labels that disagree with each other is fitted to noise.
2. Take the majority label per item as the consensus, leaving ties out rather than breaking them.
3. Sweep every candidate threshold that would still be a publishable rubric and score each against the
   consensus.
4. Publish the winner as a new rubric version through ADR-0011's registry, with the measured agreement,
   the whole sweep rather than only the winner, an owner and a review date.

Step four is a rubric version bump on purpose. A threshold that changed without one would be a constant
that drifted, and ADR-0015 requires the label a session was given to stay reconstructable from its
pinned rubric forever.

## What would have to exist first

Ratings are judgements by identified people about recorded speech. Collecting them needs a lawful
basis, a purpose statement, a retention rule and each rater's agreement to be named in the record. None
of that exists, which is a reason the work has not been done rather than an obstacle to doing it. The
current fixtures are synthetic, so a benchmark exercise over them raises no candidate privacy question
at all; a benchmark drawn from real sessions is a different piece of work entirely.
