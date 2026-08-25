# ADR-0011: The artifact registry — git authors, the database publishes, digests pin

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-26  
**Review date:** 2027-02-26  
**Supersedes:** None  
**Superseded by:** None

Closes DEC-09. Also closes the "publication separation of duties" question
[authorization-model.md](../authorization-model.md) leaves open, for artifacts.

## Context

Personas, plans, rule packs, rubrics, role standards, prompts, model policies and articulation
policies are all versioned artifacts pinned into session bundles. The domain model fixes their
lifecycle (`draft → validating → approved → published → deprecated → retired`) and their identity
(reference, version, digest) and requires published artifacts to be immutable. What was undecided:
where they live, who approves publication, and how a bad publication is rolled back.

The stakes are the evaluation itself. A rubric or prompt changed underneath a session changes what a
candidate was judged by after the fact, which destroys reproducibility — the third-highest quality
attribute — and with it the audit answers the first one depends on.

## Decision

### Two homes, one authority each

**Git authors platform artifacts; the database registry is the runtime source of truth for
published versions.** The repository tree the brief names (`services/intelligence/artifacts/…`)
is where platform artifacts are written and reviewed: pull-request review is the review step,
and CI's artifact validation gate (schema and digest) is the `validating` state for this
authoring path. Publication reads the reviewed content and copies it into the registry as an
immutable row. At runtime nothing reads the filesystem: composition resolves and pins from the
registry, so what a session ran against is always a row with a digest, never "whatever the
deployment's files said".

Tenant-authored artifacts (TEN-04's rubrics) will never live in git; they enter the same registry
through the product, with the service running the `validating` state that CI runs for git-authored
ones. One registry, two authoring paths, one lifecycle.

### The registry

A `content.artifacts` table: type, reference, semantic version, schema version, digest of the
canonical body, the body itself, lifecycle status, and provenance (who drafted, who published,
when). `(reference, version)` is unique. Bodies live inline as JSONB rather than in object
storage, because artifacts are small structured documents and a registry that cannot show what it
published without a second store answers audit questions slowly.

**Published rows are immutable by trigger, not by review.** A database trigger refuses any change
to the body, digest or version of a row once published; status may only move along the machine's
edges (published → deprecated → retired). This is IAM-06's tripwire pattern applied to content:
the shape refuses what the rule forbids.

### Pinning and the current pointer

**Sessions pin digests; a pointer names the current version.** `content.artifact_pointers` maps a
reference (and tenant, where tenant-scoped) to the artifact row compositions should use next.
Composition resolves the pointer at compose time and pins the resolved digest into the bundle.
From that moment the session's relationship is to the digest, and no publication, rollback or
retirement can change what it meant.

### Rollback is a pointer move

Rolling back republishes the past: the pointer moves to a previously published version, the bad
version is marked deprecated, and nothing is deleted or edited. An operator can always answer
"what was current on this date" from the pointer's audit trail, and "what did this session use"
from the bundle's digest, and the two questions never contaminate each other.

### Approval: separation of duties, enforced structurally

**The publisher must not be the drafter.** The aggregate refuses a publish whose actor created the
draft, so the two-person rule is a property of the registry rather than of review vigilance. The
capabilities are `content.artifact_draft`, `content.artifact_publish` and
`content.artifact_rollback`, platform authority all three; publish and rollback additionally
require step-up authentication, because both change how future candidates are evaluated, which is
the catalogue's own definition of what step-up exists for. Tenant rubrics keep their existing
`rubric.draft`/`rubric.publish` capabilities and inherit the same structural rule when they join
the registry.

### Tenant scoping without a second registry

`tenant_id` is nullable: platform artifacts are NULL and readable under any scope, tenant
artifacts are tenant-scoped by row-level security. The read policy is
`tenant_id IS NULL OR tenant_id = app.tenant_id`, so a practice composition running untenanted
reads platform artifacts and no tenant's, and a tenant's composition reads the platform catalogue
plus its own. Who may *write* is the capability layer's decision; the policy's job is only that no
tenant reads another's.

## Consequences

- CAT-02's composer gains a real source: resolve pointers, pin digests, and the bundle's
  "records every pinned artifact version" criterion becomes satisfiable.
- Publication and rollback never touch an in-flight or historical session, and this is a test
  rather than a promise: publish a new version, and a previously pinned digest still resolves to
  byte-identical content.
- The git-authored loader (reading `services/intelligence/artifacts/` into drafts) is a publishing
  tool, not a runtime dependency; it lands with the first real artifact content.
- A registry row is the audit answer for "what did we evaluate this person with", forever.

## Alternatives considered

**Files only, no registry.** Simplest, and the deployment becomes the version: two deployments
can disagree about what "current" means, rollback is a deploy, and tenant-authored artifacts have
no home at all.

**Object storage bodies with database metadata.** Right for media, wrong for small structured
documents: every audit question would need two stores to agree, and the immutability trigger
cannot protect what it cannot see.

**In-product authoring for everything.** Where TEN-04 ends up, but platform artifacts are
engineering-owned and PR review with CI validation is strictly stronger review than any in-product
flow we would build this year.

## What would change this

- Artifact bodies outgrowing inline storage (a persona with embedded media, say) — bodies would
  move to object storage behind the same digest, with the trigger guarding the digest column.
- Tenant artifact volume making platform and tenant catalogues want different retention or
  residency, which would argue for partitioning before it argued for a second registry.
