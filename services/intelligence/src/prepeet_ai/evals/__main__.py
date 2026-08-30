"""Regenerate the dataset manifest and the evaluation report.

Run after any prompt, model, rubric or fixture change:

    uv run python -m prepeet_ai.evals

The report is a reviewed artifact, not a build output. Regenerating it is a
deliberate act, and the diff is the thing to read: it shows exactly which
span, contradiction or sufficiency outcome moved and in which profession.
"""

from __future__ import annotations

import sys

from prepeet_ai.evals import harness
from prepeet_ai.evals.calibration import calibrate
from prepeet_ai.evals.dataset import refresh_manifest


def main() -> int:
    """Refresh the manifest, run the harness, write the report, report the gates."""
    moved = refresh_manifest()
    for name in moved:
        print(f"manifest: {name} digest updated")

    document = harness.run()
    path = harness.write_report(document)
    totals = document["totals"]
    print(f"report: {path}")
    print(f"  cases                {totals['cases']} across {totals['professions']} professions")
    print(
        f"  evidence grounding   {totals['grounding']['grounded']}/{totals['grounding']['checked']}"
    )
    print(
        f"  unsupported facts    {totals['unsupported_facts']['unsupported']}/"
        f"{totals['unsupported_facts']['assertions']} "
        f"(rate {totals['unsupported_facts']['rate']})"
    )
    print(
        f"  schema conformance   {totals['schema_conformance']['conformant']}/"
        f"{totals['schema_conformance']['records']}"
    )
    expectations = totals["expectations"]
    print(f"  expectations met     {expectations['met']}/{expectations['checked']}")
    print(
        f"  stability            {totals['stability']['stable']}/"
        f"{totals['stability']['expected_stable']} "
        f"({totals['stability']['known_sensitivities']} known sensitivities)"
    )
    print(
        f"  contradictions       {totals['contradictions']['genuine']} genuine, "
        f"{totals['contradictions']['false_positive']} known false positives"
    )
    print(f"  provider calls       {totals['cost']['provider_calls']}")
    print(f"  connections observed {totals['cost']['network_connections']}")
    print(f"  latency              {document['timing']['total_ms']} ms (outside the digest)")
    print(f"  results digest       {document['results_digest']}")

    # QUA-03's answer, printed beside QUA-02's numbers on purpose. Every
    # figure above is uncalibrated, and a reader who is not told that will
    # assume somebody checked.
    outcome = calibrate(report=document)
    print(
        f"  confidence rules     {'calibrated' if outcome.calibrated else 'NOT calibrated'} "
        "against human judgement"
    )
    for refusal in outcome.refusals:
        print(f"                       {refusal.split(':')[0]}")

    violations = harness.gate_violations(document)
    for violation in violations:
        print(f"GATE FAILED: {violation}")
    return 1 if violations else 0


if __name__ == "__main__":
    sys.exit(main())
