"""The AI quality harness: datasets, graders and the report they produce.

QUA-01 owns the fixtures under `evals/datasets`; QUA-02 owns the harness
that runs them and writes `evals/reports/latest.json`. The code lives here
rather than beside the data because it is production-shaped: it is linted,
type checked, documented and covered by the same gates as everything else
in this service, and a grader nobody checks is worth as little as an
evaluation nobody checks.
"""
