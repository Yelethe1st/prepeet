# evals: the AI quality harness

## What this owns

Datasets, graders and scenarios that measure evidence grounding, unsupported facts, schema conformance,
stability, latency and cost.

- [`datasets/`](datasets): the fixtures and their provenance record (QUA-01).
- [`reports/`](reports): the committed evaluation report artifact (QUA-02).

The harness code lives in `src/prepeet_ai/evals/` rather than beside the data, so it is linted, type
checked, documented and covered by the same gates as the rest of the service. A grader nobody checks is
worth as little as an evaluation nobody checks.

Run it, and regenerate both the manifest digests and the report:

```
cd services/intelligence && uv run python -m prepeet_ai.evals
```

It also runs as part of the ordinary test suite, so every change runs it whether or not anybody
remembers to.

## What this must never do

An eval never runs against real candidate data without an approved lawful basis.
