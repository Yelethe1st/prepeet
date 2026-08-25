# candidate — the candidate's own data

## What this owns

The profile: disciplines, target roles, seniority, career context, interview
defaults, accessibility preferences and notification settings. Later tickets
add documents, extracted facts and the private evidence bank.

## What this must never do

**It never serves anyone but the owner.** Every store operation scopes its
transaction as the owner and nothing else; the schema's owner-only policy,
tenant-absence clause and write tripwire (IAM-06's guards) stand below it,
and the HTTP surface has no parameter that could name another person.

**It never demands completeness.** The zero-value profile is valid, an
unsaved profile reads as the empty one rather than an error, and no feature
gates on a completeness score. A partial profile is a profile.

**It never interprets the candidate's words.** Disciplines and roles are
trimmed, never mapped or corrected; the catalogue that gives them structure
is CAT-03's, and until then these are the person's own vocabulary.

**Saves replace whole records.** The profile is one screen, the screen
submits what it shows, and field-level merges are where two tabs quietly
assemble a profile neither of them displayed.

## Documents

The CV is versioned, never rewritten: replacement is version n+1 existing.
The row is the authoritative record - key, type, bytes, digest, state - and
rows outlive their objects, because the digest a session bundle pinned must
stay answerable after the bytes are deleted. That is PRO-02's rule that
deletion never rewrites a session composed from an earlier version, held
structurally: bundles are written only by the ready transition and are
immutable by trigger, and document history is never destroyed.

Uploads are browser-direct against presigned URLs; the server never proxies
a file and the browser never holds a durable credential. A stalled upload
has its own visible state - uploading, then failed on abort - and recovery
is simply the next version. Types are allowlisted by name, never sniffed,
because sniffing is how an SVG with a script becomes "an image".

## The accessibility promise

extended_time, captions and reduced_motion are stored here so the prepare
and live screens can honour them by default. The columns are explicit rather
than a settings blob because each is a promise a screen has to keep, and a
blob key is a promise nothing can find. The honouring itself lands with
SES-03's prepare screen, which reads them from this record.
