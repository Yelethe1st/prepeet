# Evaluation reports

`latest.json` is the committed evaluation report: the output of the QUA-02 harness over every fixture
in [`../datasets`](../datasets). It is a reviewed artifact, not a build output.

## Why it is committed

QUA-02 requires results comparable across runs and versions. A number printed to a terminal is
comparable to nothing. This file is a stable document with sorted keys and a `results_digest` over
everything except timing, so two runs of the same code produce the same digest and a diff between two
versions shows exactly which span, contradiction or sufficiency outcome moved and in which profession.

Timing sits outside the digest deliberately. A slower laptop is not a regression in the evaluation.

## Why a change to the pipeline fails the build

Two named tests hold it:

- `test_the_committed_report_matches_a_fresh_run` fails when the pipeline's output changes and the
  report has not been regenerated.
- `test_the_committed_report_was_produced_by_the_current_governed_inputs` fails when a governed input
  changes without a fresh run. The governed inputs are the extraction, calculation, profile and coaching
  versions, the model policy, the pinned rubric and policy bodies by digest, and the dataset manifest.
  That is what "the harness runs on every prompt, model or rubric change" has to mean to be checkable.

Regenerating is therefore a deliberate act, and the diff is the thing to read.

```
cd services/intelligence && uv run python -m prepeet_ai.evals
```

The command exits non-zero if a hard floor is broken.

## The hard floors

Grounding and schema conformance must be perfect, the unsupported-fact rate must be zero, and every
fixture must behave as it declares. None of those is a calibrated threshold: they are absolutes the
specification already requires. Every number that needs calibrating against human agreement, including
confidence and evidence thresholds, belongs to QUA-03 and is reported here without being gated.
