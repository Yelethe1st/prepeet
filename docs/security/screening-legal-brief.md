# Screening: what we need determined, per jurisdiction

**Status:** Awaiting legal determination
**Owner:** olabode omoyele
**Last updated:** 2026-09-01

A brief for counsel. It exists because [DEC-11](../delivery/tickets/01-decisions-and-adrs.md) is the
longest pole in the project: seventeen tickets across screening and recruiter review are built and
cannot launch anywhere until this is answered for that place.

It asks four questions and nothing else. The system has been built so that the answers are data it
reads rather than assumptions it was compiled with, which is why this can be a short brief rather
than a review of a product.

## What has already been decided, so you do not have to

[ADR-0020](../architecture/decisions/0020-screening-disclosure-access-and-appeal.md) settled the
mechanism. Three consequences matter for scoping your work:

**A jurisdiction with no recorded determination cannot run a screening interview at all.** Not a
warning, not a permissive default: opening a campaign there is refused. So the absence of an answer
is already safe, and there is no pressure to answer quickly for places we are not selling into.

**Consent is unbundled everywhere, whatever any jurisdiction requires.** Required processing and
optional processing are separate records with separate acceptances, declining the optional never
blocks the interview, and model improvement can never be a condition of taking one. This is the
strictest position and is not up for jurisdiction-by-jurisdiction variation, so it needs no
determination.

**Disclosure versions are immutable and acceptance records the exact version accepted.** Changing the
text creates a new version and never rewrites what somebody already agreed to.

## What we are asking

We recommend determining **the United Kingdom only**, and leaving every other jurisdiction genuinely
blocked rather than speculatively answered. [ADR-0001](../architecture/decisions/0001-hosting-platform-and-regional-topology.md)
commits production to `eu-west-2`, London, and names a UK healthcare provider as the shape of the
first buyer. The release gate for screening already requires the pilot be limited to named tenants,
roles and jurisdictions, and that limit is now enforced by the code rather than by a checklist.

### 1. What must a candidate be told before an AI-assisted screening interview?

Our current position, from [responsible-hiring.md](responsible-hiring.md), is that ten things are
disclosed before consent: the employer, the purpose, that AI is involved, what is recorded and
processed, who has access, how long it is kept, how to exercise their rights, what accommodations
exist, what they will be told of the result, and that a named human owns the decision.

The system refuses a disclosure that leaves any of the ten blank. We need to know whether that list
is sufficient, and what wording any of it requires.

### 2. What may a candidate see of their own evaluation?

Pick one. The system enforces the choice server-side on every read path, not by hiding a link.

| Position | What the candidate sees |
| --- | --- |
| `full_evaluation` | Everything the reviewer sees |
| `evidence_without_band` | The evidence and coverage, not the suggested band |
| `completion_status` | That the interview was completed, nothing further |
| `submission_only` | Confirmation it was submitted |

### 3. Is appeal a legal right, a tenant's option, or our policy?

Absent a determination the platform treats appeal as a right, which is the strictest of the three.
Loosening a right that was honoured is a product change; discovering a right was owed and not offered
is a compliance failure with candidates already affected.

We need to know which it is in the UK, and what the obligation entails if it is a right: who may
review, within what period, and what the candidate must be told of the outcome.

### 4. Who signs this, and on what date?

The determination is stored with a named approver and an approval date, and the schema refuses a row
without one. This is not a formality: if an audit asks who decided what a candidate may see, the
answer has to be a person.

## What we are not asking

**Enterprise federation, SSO and SCIM.** Deferred by
[ADR-0003](../architecture/decisions/0003-identity-built-in-go.md) and out of scope.

**Candidate comparison.** Off by default and deferred by
[ADR-0017](../architecture/decisions/0017-candidate-comparison-is-deferred.md), with a closed DEC-11
in every launch jurisdiction as one of its reopen triggers. It does not need answering now.

**Anything about practice mode.** A candidate practising alone has no employer, no tenant and no
screening evaluation. This brief is only about the employer-facing path.

## What happens with the answers

They become one stored, versioned record per jurisdiction, read at run time. A campaign pins the
version it was opened under, exactly as it pins its rubric, so a later determination never changes a
running campaign or re-scores a completed interview.

The second open item is the approved disclosure **text** itself. The ten required areas are already
fixed in code, so drafting can start now and be approved alongside the determination rather than
after it. Serialising the two would add weeks for no reason.
