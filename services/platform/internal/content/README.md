# content — the artifact registry

## What this owns

Versioned, digest-addressed interview artifacts - personas, plans, rule
packs, rubrics, role standards, prompts, model and articulation policies -
their lifecycle, the pointer naming each reference's current version, and
the rollback path. ADR-0011 decides the shape; this package enforces it.

## What this must never do

**It never edits a published version.** A change is a new version, by trigger
rather than by review: a rubric edited after publication changes what a
candidate was judged by after the fact, and reproducibility is the third
quality attribute for exactly that reason.

**It never lets one person ship their own artifact.** The publish refuses an
actor who drafted the version, whatever capabilities they hold. Two people,
structurally: ADR-0011's separation of duties.

**It never deletes history.** Only drafts may be deleted; everything past
validation stands, and rollback deprecates rather than removes, so "what was
current on this date" and "what did this session use" are both always
answerable.

**It never lets a pointer move touch a pin.** Sessions pin digests;
publication and rollback move only the pointer, and the pinned digest
resolves to byte-identical content forever - GetByDigest verifies the hash on
every read, because serving corrupted content as pinned content would put
words into a historical session's record.

**It never shows one tenant another's artifacts.** The platform catalogue
(tenant NULL) is everyone's; a tenant's artifacts are its own by policy, and
a tenant's pointer overrides the platform's for the same reference without
hiding the catalogue from anyone else.
