# Publication records

One record per artifact publication, from QUA-04. A record is the machine-readable answer to "who
approved this, against which evaluation report, and how do we get back".

## Why this directory is empty

Every artifact in [`../../artifacts`](../../artifacts) predates this gate. Writing a record for one of
them now would mean naming an approver for an approval that never happened, so they are listed by
digest in [`../publication-policy.json`](../publication-policy.json) under `pre_gate_artifacts`
instead. The list is digests rather than filenames on purpose: editing one of those files fails the
gate exactly as adding a new one does.

## The shape of a record

```json
{
  "record_version": "1.0.0",
  "artifact": {
    "file": "rubric/practice-default@1.2.0.json",
    "type": "rubric",
    "reference": "rubric/practice-default",
    "version": "1.2.0",
    "sha256": "the digest of the file as committed"
  },
  "change": { "material": true, "summary": "What changed and why" },
  "author": { "id": "person:...", "name": "The person who wrote it", "kind": "person" },
  "approver": { "id": "person:...", "name": "The person who approved it", "kind": "person" },
  "approved_on": "2026-09-01",
  "evaluation_report": {
    "path": "evals/reports/latest.json",
    "results_digest": "the report's results_digest",
    "governed_digest": "the report's governed_inputs.digest"
  },
  "rollback": {
    "kind": "previous_version",
    "to_version": "1.1.0",
    "to_sha256": "the digest of the version being rolled back to"
  }
}
```

`rollback.kind` is `previous_version` for anything with a predecessor, and `withdraw` only for the
first version of a reference, where there is no pointer to move back to.

## Running the gate

```
cd services/intelligence && uv run python -m prepeet_ai.evals.publication evals/publications/<record>.json
```

It exits non-zero and names every refusal. The whole tree is also checked by
`test_every_artifact_in_the_tree_is_recorded_or_declared_pre_gate`, so a new artifact with no record
fails CI whether or not anybody runs the command.

## What the gate refuses

A missing evaluation report, a report that quotes a different run, a report produced before the
governed inputs moved, a report older than the policy's age limit, an undated report, a report that
never ran against these bytes, a report below one of the harness's hard floors, an approver who is the
author, an approver who is a service principal on a material change, a missing rollback plan, and a
rollback plan naming a version that is not in the tree or has been edited since. Each refusal is a
named test.

## What it does not do

Nothing in Go's `contentctl` calls this gate, so the registry itself would still accept a publication
that never passed here. CI refuses the change before it can be deployed, which blocks the deployment
rather than the insert. Wiring the gate into the publishing tool is Go work and belongs with the ticket
that does it.
