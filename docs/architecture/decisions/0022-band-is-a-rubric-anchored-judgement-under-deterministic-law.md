# ADR-0022: A band is a rubric-anchored judgement, and deterministic code is the law it answers to

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-09-04  
**Review date:** 2027-03-04  
**Supersedes:** None  
**Superseded by:** None

Closes the "Score/band semantics" item in
[evaluation-system.md](../evaluation-system.md)'s open decisions, and the
Phase 0 question in
[model-backed-evaluation.md](../model-backed-evaluation.md): whether
aggregation stays permanently deterministic or may accept model-proposed
per-anchor assessments. It decides the second, under conditions.

This ADR is about **which component decides a band**. It does not touch
[ADR-0015](0015-confidence-is-qualitative-evidence-sufficiency.md):
confidence remains a mechanically derived label, and a model's
self-reported certainty is never it.

## Context

`aggregate-1` derives a band arithmetically: supporting spans over
eligible spans, compared against ratio thresholds in the pinned rubric.
That was the right floor. It is reproducible, cheap, and it proved
storage, provenance, sufficiency, publication and rollback without a
provider in the loop.

It is also, measurably, unable to do the job the product promises. A
practice session run end to end on 2026-09-04 produced this evidence for
the `systems-design` competency:

```text
[supporting] "I led the systems design for a booking platform serving 4 million patients."
[supporting] "The design separated the availability read path from the reservation
              write path, which cut p95 latency to 120 milliseconds."
[supporting] "I wrote 5 design documents that quarter."
band: strong · supporting 3 · contradictory 0 · unverified 0
```

The third span is not systems-design evidence. It was linked because the
token `design` occurs inside `design documents`, and it was graded
`supporting` because the sentence contains a digit. It then carried a
third of the ratio that produced **strong**. Nothing downstream could
detect this: to arithmetic, three supporting spans and no contradictions
is an unambiguous top band, and the sentence's meaning is not an input.

The deeper problem is that the rubric this system is heading towards
cannot be consulted by arithmetic at all. Anchors are written statements —
"compares credible alternatives against stated constraints" — and deciding
whether an answer meets one is a reading task. If band derivation stays
arithmetic while rubrics gain criteria and anchors, the anchors become
decorative: authored, reviewed, published, pinned, and consulted by
nothing. That is worse than not having them, because the surfaces would
show a candidate anchored language that did not produce their result.

The alternative is not "let a model score people". It is to move the
reading task to the component that can read, and keep every guarantee that
makes the result defensible in code that cannot be argued with.

## Decision

**A model proposes the band by reasoning against published anchors.
Deterministic code decides whether that proposal is admissible, and its
answer is final.**

### What the model proposes

For each in-scope rubric criterion, a typed finding: a status, the
strongest anchor the evidence supports, the evidence identifiers for and
against, a short basis in terms of the anchor, and the gap to the next
anchor. From those, a recommended competency band with its basis, plus
unresolved questions and the structured features the confidence layer
consumes.

Every material sentence must be derivable from referenced evidence and the
pinned rubric. The model may report that evidence is ambiguous. It may not
resolve ambiguity with an assumption.

### What deterministic code keeps, without exception

- input integrity and digest verification;
- exact quote grounding at the recorded character and clock ranges;
- candidate-authored text only, never interviewer text;
- closed vocabularies for status, band, kind and reason;
- sufficiency, coverage and required-criterion floors, which are never
  waivable by a proposal;
- prohibited inference and prohibited output rules;
- the final confidence label, derived per ADR-0015;
- publication, immutability and provenance;
- refusal of the whole attempt when any material item is invalid.

### The validator may lower. It may never raise.

When an objective rule fails — sufficiency unmet, evidence ineligible,
required criterion uncovered, a band supported only by duplicated or
invalid evidence — the validator lowers the competency to `unassessed`,
or rejects the attempt outright. It does not adjust between two valid
semantic bands using evidence counts.

The asymmetry is deliberate. A validator that could raise a band would be
`aggregate-1` again, arriving after the model and overruling a reading it
cannot perform; the arithmetic would be back in charge wearing a
different name. A validator that can only lower expresses the actual
relationship: the model reasons, the law constrains, and the constraint
only ever runs in the direction of claiming less.

### The deterministic floor stays implemented and stays labelled

`aggregate-1` is not deleted, deprecated, or hidden. It remains the
fallback when a route fails, a budget is exhausted, or a boundary has no
approved model assessment. A result it produced records that fact in its
provenance and is never presented as equivalent to a model-assessed
result unless equivalence has been measured for that rubric boundary.

### Promotion is per boundary, on measured agreement

Model band authority is enabled for a named release boundary — role
family, rubric version, language, mode — only when QUA-03's human
benchmark shows it materially improves agreement with expert raters for
that boundary. It is not a global switch, and practice approval never
implies screening approval.

Until a boundary is promoted, the model's assessment runs in shadow with
no publication path, exactly as
[model-backed-evaluation.md](../model-backed-evaluation.md)'s Phase 4
describes.

### Divergence is monitored

Where both are computed, the difference between the model's band and
`aggregate-1`'s is recorded and alerted on. Sustained divergence in either
direction is a quality signal about the model, the anchors, or the
extractor, and is investigated rather than averaged away.

## Alternatives considered

**Keep band derivation permanently arithmetic.** Rejected. It cannot read
an anchor, so rich rubrics would be authored and ignored, and the observed
false positive above would remain undetectable by construction. Its one
genuine advantage, byte-reproducibility, is preserved differently: the
floor stays implemented and every result records which route produced it.

**Model proposes, arithmetic decides.** Rejected. This is the current
system with an expensive preamble: the deciding component still cannot
consult the anchors, so the model's reading changes nothing that reaches
a candidate.

**Model band authoritative with schema validation only.** Rejected. Schema
validation cannot enforce sufficiency, coverage, grounding or prohibited
inference, so a fluent, well-formed, entirely unsupported assessment would
publish. The floors are the point.

**Let the validator raise as well as lower.** Rejected for the reason
given above: symmetric adjustment reinstates arithmetic authority.

## Consequences

- Rich rubrics become load-bearing rather than aspirational, which makes
  the rubric schema and its authoring path a hard prerequisite rather than
  a parallel workstream. Nothing here ships before it.
- Reproducibility changes meaning for model-assessed results: audit
  reproducibility — the same inputs, artifacts, route and versions
  recorded well enough to explain a result later — rather than
  byte-identical reruns. Deterministic-floor results keep the stronger
  property, and their provenance says which they are.
- `evaluation-system.md` can close "Score/band semantics" and state the
  boundary instead of leaving it open.
- REV-02's review screen already renders the pinned versions beside every
  band; `band_basis` and the adjacent-anchor gap become new fields on a
  surface that was built to carry them.
- REV-06 appeals improve materially. A re-review can engage with written
  reasoning against a published anchor, which "the supporting ratio was
  0.67" never permitted.
- Evaluation gains a second model stage per session, with the latency and
  cost that implies. Budgets and per-stage routing under ADR-0019 are how
  that stays bounded.

## Risks and mitigations

- **The model is worse than arithmetic on some boundary.** Shadow
  operation and a per-boundary agreement gate before promotion; the floor
  remains selectable per boundary.
- **Systematic under-banding is invisible.** The lower-only asymmetry
  means an over-cautious model is not corrected by the validator. Mitigated
  by divergence monitoring and QUA-05's fairness and assessability
  monitoring, with a named escalation owner. This is the residual risk this
  decision most deliberately accepts, and it is accepted because
  under-claiming costs polish while over-claiming costs a candidate
  fairness.
- **A provider silently changes a model revision.** ADR-0019's recorded
  configured name and resolved revision, plus quality monitoring that
  treats an unexplained shift as an incident.
- **Anchors are written badly, so the reasoning is well-formed and wrong.**
  Anchors are reviewed and published artifacts under ADR-0011, benchmarked
  before promotion, and versioned so a correction is a new version rather
  than a silent edit.
- **Non-determinism is challenged in a hiring context.** Every material
  claim resolves to exact candidate-authored text; sufficiency and
  coverage floors are non-waivable; the human decision remains the
  decision under ADR-0020; provenance is complete; the deterministic floor
  is available and labelled.

## Reversibility and migration

Reversible per boundary at any time by routing that boundary back to
`aggregate-1`, which is why it is never deleted. Published results keep
their pins, so history stays readable whichever route produced it, and a
reverted boundary does not invalidate results already published under the
model route.

Migration is Phases 1 and 4 through 7 of
[model-backed-evaluation.md](../model-backed-evaluation.md): rich rubric
first, extraction in shadow, practice release, then screening only with
its own approval. No result reaches a candidate from a boundary that has
not passed its own gate.

## Validation

- QUA-03 publishes per-boundary agreement thresholds and the measured
  agreement against them before any promotion.
- The shadow comparison reports grounding, band agreement, insufficiency
  handling, latency and cost against the deterministic floor.
- The observed false positive in Context becomes a regression fixture: an
  assessment that admits "I wrote 5 design documents that quarter" as
  systems-design evidence fails the benchmark.
- Publication re-validates independently, so a band that passed stage
  validation but violates a floor still cannot publish.

## Revisit when

QUA-03 reports that model and deterministic agreement are
indistinguishable for a boundary, which would make the model stage
unjustifiable cost there; a jurisdiction's determination under DEC-11
constrains automated assessment further; or divergence monitoring shows
the lower-only asymmetry is producing systematic under-banding that
monitoring cannot correct.
