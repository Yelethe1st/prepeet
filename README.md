# Prepeet

A multi-tenant, voice-first interview platform with two modes. **Practice** is candidate-owned
preparation with evidence-linked coaching. **Screen** is employer-configured interviewing that produces
evidence for a named human reviewer, and never decides an outcome itself.

The specification lives in [docs/](docs/README.md). The work is broken into 166 tickets in
[docs/delivery/tickets/](docs/delivery/tickets/README.md). The interface design is the high-fidelity
prototype in [screens/](screens/README.md), which the production frontend is ported from.

## Getting started

```bash
make bootstrap    # install every toolchain and dependency
make local-up     # PostgreSQL, LocalStack and Temporal in containers
make test         # run every suite
make cover        # run every suite and enforce the coverage floors
make dev          # how to start each deployable
```

`make help` lists everything.

Requirements: Go 1.26, Node 22 with pnpm, Python 3.12 or later with
[uv](https://docs.astral.sh/uv/), and Docker for the local stack. No cloud account is needed to run the
product locally: LocalStack stands in for S3, Secrets Manager and KMS, so adapters are written once
against the AWS APIs and differ only by endpoint and credentials between here and production.

## Repository layout

```text
apps/web/              Next.js application, ported from the prototype in screens/
services/platform/     Go control plane: API, authorization, persistence, lifecycle, audit
services/intelligence/ Python intelligence: composition, evaluation, articulation, coaching
packages/contracts/    Hand-authored OpenAPI, Protobuf and event schemas
packages/generated/    Generated clients, never edited by hand
infrastructure/        Terraform, Temporal, observability, local stack
tools/                 Contract checking, seeding, load testing, coverage gates
docs/                  Specification, decisions and the ticket backlog
screens/               The HTML prototype the interface is ported from
```

Every module directory carries a README stating what it owns and what it must never do.

## How this is built

Three constraints apply to every change, and each is enforced rather than encouraged.

**Test first.** The failing test is written before the implementation and ships in the same change.
Nothing merges without a test covering it, and coverage floors are enforced per deployable in CI. See
[the definition of done](docs/delivery/tickets/README.md).

**Documented.** Every exported type, function, endpoint, workflow and component says which rule it
enforces and which invariant it protects. Documentation explains why rather than restating what the code
already says.

**Ported, not redesigned.** The interface carries across the prototype's layout, copy and interaction
states. Deviations are recorded with a reason.

## The boundaries that matter

These are invariants rather than preferences, and a change that breaks one is a stop-ship:

- A candidate's practice history is never reachable from employer authority, in either direction.
- Every material evaluation conclusion cites the evidence behind it.
- Unknown, insufficient and unassessable are distinct from low performance, everywhere.
- A named human owns every hiring decision. There is no automatic advance or rejection.
- Accent, personality, emotion, honesty and protected characteristics are never scored or inferred.
- Sessions pin every artifact version they used, so a result can be reconstructed later.

[docs/security/responsible-hiring.md](docs/security/responsible-hiring.md) holds the full set.

## Where this runs

AWS, `eu-west-2` London, ECS on Fargate, with candidate recordings, transcripts and evaluations stored
and processed in the tenant's region. Model and transcription providers are named sub-processors and may
process content elsewhere under contract, which is disclosed to candidates rather than buried in an
annex. See [ADR-0001](docs/architecture/decisions/0001-hosting-platform-and-regional-topology.md).

## Status

Foundation in progress. The three deployables build, run and are covered by tests. Nothing in the
screening epics ships before
[DEC-11](docs/delivery/tickets/01-decisions-and-adrs.md) settles disclosure and appeal rights per
jurisdiction.
