# Practice Mode

**Status:** Proposed  
**Owner:** Candidate product  
**Last updated:** 2026-08-24

## Purpose

Practice mode helps a candidate improve interview answers and articulation for a target role. The candidate owns the history and controls its use.

## Experience contract

The candidate shall be able to:

1. Create or update a profile and target role.
2. Supply an optional CV and job description and correct extracted facts.
3. Configure discipline, role, seniority, shape, duration, persona/style, pressure, accommodations, and
   the personal requirements this session should assess.
4. Review consent and recording policy.
5. Pass device and connection checks.
6. Complete a realistic voice interview without intrusive live scoring.
7. Receive evidence-linked results and coaching.
8. Replay their authorized transcript/audio.
9. Review articulation observations and selected drills.
10. Redo an answer and compare it with the original.
11. Track comparable, evidence-linked progress against competencies and personal requirements over time.
12. Receive explainable recommendations based on private prior practice evidence.

## Candidate-visible outputs

- Overall evidence summary and coverage.
- Competency results with sufficient/insufficient evidence.
- Per-answer strengths, gaps, rationale, and next action.
- Transcript and synchronized audio evidence.
- Neutral claim and contradiction explanations.
- Fact-preserving stronger-answer structure.
- Articulation dimensions, deterministic metrics, timestamps, and practice drill.
- Readiness and progression by role/discipline.
- Personal-requirement outcomes with criteria, evidence, sufficiency, and next action.
- Behaviourally anchored trend metrics and their comparison basis.

Do not present one opaque articulation percentage. Unknown competencies remain unassessed rather than zero.

## Personal requirements and cumulative learning

A candidate may select a catalogued personal requirement or define an observable requirement in their
own words, such as giving an appropriate greeting, delivering a concise introduction, explaining
technical trade-offs, asking relevant questions, or closing clearly. Before the session starts, the
system resolves the request into candidate-visible, versioned criteria and pins the selected criteria in
the session bundle. It asks for clarification or reframes a request that is not observable or would
require a prohibited inference.

Each result is `achieved`, `partially_achieved`, `not_demonstrated`, or `not_assessable`. It cites the
relevant transcript/audio evidence, states which criteria were and were not demonstrated, and offers one
or two concrete actions where improvement is useful. `Not_assessable` includes a session that never
created a fair opportunity to demonstrate the requirement and must not be converted to a low score.

Private practice observations accumulate into a candidate-controlled development profile. The profile
identifies recurring strengths, unresolved gaps, improvement across comparable sessions, and stale
evidence. It may shape future composition, drills, and recommendations, but every recommendation explains
which prior observations caused it and targeting never becomes the whole interview. Sparse history
produces cautious suggestions rather than invented trends.

Quantitative measures are behaviourally anchored and retain criterion/rubric versions, evidence,
sufficiency, and comparison basis. The interface may show measures such as criteria demonstrated, goal
completion rate, answer-structure completeness, evidence use, and answer duration. It must not average
incompatible roles, formats, criteria, or rubric versions. Confidence is collected only as an optional
candidate pre/post-session self-rating; it is never inferred from delivery or media.

## Articulation coaching

Measure observable delivery first: pace, pauses, fillers, repetition, restarts, answer length, audio quality, and transcription confidence. Semantic coaching may then assess directness, structure, conciseness, signposting, specificity, and coherence.

Feedback should state listener impact and one or two actions. Suggested rewrites never invent candidate facts; missing evidence uses placeholders or questions.

Recommended drills:

- headline first;
- 60-second compression;
- deliberate pause instead of filler;
- STAR compression;
- signposting;
- concrete-language substitution;
- one-example constraint;
- playback and redo.

Personal baselines require sufficient observations and never enter employer screening.

## Realtime policy

The interview should remain realistic. Continuous articulation scores, filler counts, and corrections are prohibited during answers. Optional future cues may cover microphone level, answer duration, or severe pace change only after usability validation.

## Progression

- Store append-only competency observations with evidence and rubric version.
- Store append-only personal-requirement observations with evidence and criterion version.
- Compute readiness against a pinned role standard.
- Group progression by role and discipline; do not average incomparable roles.
- Show assessed/unassessed and evidence freshness.
- Use prior gaps to inform future practice composition.
- Explain every cumulative recommendation and allow the candidate to pause or delete personalization.

## Privacy

- Practice data is candidate-owned and private by default.
- Recruiters cannot access it because the candidate screens for their tenant.
- Model-improvement consent is separate and off by default.
- Candidate may request export, correction, or deletion subject to documented exceptions.

## Required states

First-run, returning, composing, ready, device failure, reconnecting, recovered, interrupted, processing, delayed, evaluation failed, insufficient evidence, unassessable articulation, review ready, and deleted/expired media.

## Acceptance outcomes

- Coaching is traceable and fact-preserving.
- A failed optional coaching stage does not erase valid evaluation.
- Redos preserve original history.
- Poor recording quality never becomes poor articulation.
- Supported candidates can complete the entire journey by keyboard and assistive technology.
