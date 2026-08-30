# tenantadmin: administering one workspace

## What this owns

Three things a tenant administrator does: the workspace's settings and
branding, the periodic review of who can reach candidate evidence, and the
rubric library.

Tables: `tenancy.tenant_configuration`, `tenancy.access_reviews` and
`tenancy.access_review_items`. The `tenancy` schema is identity's, so those
three are declared table by table in `internal/architecture/ownership_test.go`
and the declaration cuts both ways: this module may not name
`tenancy.memberships`, and identity may not name these. Both directions are
proven by planting a crossing and watching the gate fail.

## What this must never do

**It never builds a second version history.** The rubric library is a surface
over `content.artifacts`, reached through a port. That registry already stores
versioned, digest-identified, published artifacts with a lifecycle, a
separation of duties on publication, an immutability trigger and a rollback
path, and a rubric is one of its types. A second history here would be a
second answer to "what is version 1.1.0 of this rubric", and the second answer
is the one that drifts.

**It never edits a settings version.** A change is a new row, append-only by
trigger and by grant. That is what lets the audit trail show what a value was
before, and what lets a campaign keep the defaults it was created under: the
campaign pins a settings version, and `AtVersion` re-reads exactly that
document however many times the workspace has saved since.

**It never writes a rubric schema of its own.** Whether a body is a usable
rubric is decided by the context that reads rubric bodies, injected as a
`RubricValidator`, for the same reason the artifact loader injects its
catalogue parser.

**It never asks who belongs to the workspace.** Opening an access review needs
a roster; that is identity's answer, and it arrives through the `Roster` port
with cmd wiring the two. A query against `tenancy.memberships` here would make
the two modules one thing with a directory between them.

## The access review's one honest limitation

Dormancy is read from `audit.events`, because that is the only tenant-scoped
record of what a person did in a workspace: a session belongs to a person
across every workspace they belong to and cannot answer "dormant here".

The consequence is that somebody who only ever read pages has no audited act
and reads as dormant. That is the conservative direction - it prompts a
reviewer to confirm them rather than hiding them - but it is a false positive
and it is not pretended otherwise. A read-audit trail would fix it and belongs
with the sensitive-read auditing in the authorization model.

## What is not wired

There is no HTTP surface and no scheduler. The endpoints would need operations
added to `packages/contracts/api/openapi.yaml`, and the review's cadence would
need a job that can enumerate tenants, which nothing can do today without a
role that bypasses row-level security. `Due` answers the question per
workspace and `Open` is idempotent behind a unique index, so the scheduler is
a caller away rather than a redesign.
