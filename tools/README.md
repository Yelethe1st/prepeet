# tools — developer tooling

## What this owns

Contract checking, code generation, build gates, dependency auditing, seeding, load testing, prompt
evaluation and data migration utilities.

A gate that lives here is invoked through a make target rather than copied into a CI step, so the
pipeline and an engineer run the same check. `coverage/` holds the coverage floor and the filter that
excludes generated packages from it, `docs/links.sh` checks internal documentation links, and
`vulncheck/` turns a govulncheck report into a pass or a failure.

## What this must never do

A tool never becomes a runtime dependency of a deployable.
