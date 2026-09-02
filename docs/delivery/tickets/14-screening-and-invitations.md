# Epic SCR — Screening, campaigns and invitations

**Phase 5** · **Workstream** Go, Web, Security/privacy

Gated on DEC-11. No screening feature ships to a real candidate before the jurisdiction question is
answered, the disclosure is approved, and tenant isolation has been independently tested.

---

### SCR-01 · Implement campaigns with immutably pinned configuration

**Depends on** CAT-01, IAM-06, DEC-11 · **Blocks** SCR-02

*Gap found against the prototype: campaigns are required by screen mode, the public API and the
authorization model, but had no surface until the prototype was extended.*

A campaign fixes one role, one published rubric and calibration, one disclosure and one accommodation
policy. Invitations are issued under a campaign, never standalone.

**Done when**
- [x] A campaign cannot be opened against a draft or unpublished configuration.
- [x] Publishing a new rubric version does not alter a running campaign or re-score completed interviews.
- [x] Recruiter access is scoped per campaign, enforced server-side.

**Two of three. The domain and the schema, not yet the server path.**

Unblocked by [ADR-0020](../../architecture/decisions/0020-screening-disclosure-access-and-appeal.md),
which lets SCR be built while DEC-11's legal determination stays open, because a jurisdiction with no
recorded determination cannot open a campaign at all.

A campaign is assembled by reference and opened by digest. That distinction is the whole of the
second criterion: a recruiter chooses "the backend rubric", and opening resolves it to one specific
published artifact, so republishing that rubric afterwards writes a new row with a new digest and the
running campaign keeps pointing at what it chose. Nothing re-scores because nothing re-resolves.

Opening checks the determination first, then completeness, then publication. That order is
deliberate: the determination is the one failure nobody in the product can fix, so reporting it ahead
of a list of unpublished artifacts saves a recruiter fixing things that will not help.

Both rules are enforced twice, in the service and in the schema, because a rule only in Go is one a
future query walks around and a rule only in the database produces an error nobody can act on. Five
service guards were each proven by breaking them, and four direct SQL probes confirmed each database
refusal fires for its own reason rather than incidentally.

**The third box stays open, though it is closer than it was.** The store now enforces the scope and
an integration test proves a recruiter on one campaign cannot read another in the same tenant, nor a
campaign in another tenant. The access check and the read are one query rather than two steps,
because two steps is where the check gets skipped: a later caller reaching for the plain read is not
doing anything that looks wrong. A refusal is worded identically to a campaign that does not exist,
since a recruiter who can tell "not yours" from "no such thing" can enumerate a tenant's campaigns by
asking.

**Closed: requests now pass through it.** Five operations serve the surface, and the capability
story is worth recording because it is not the obvious one. Every operation declares `campaign.read`,
including the writes, and that is a consequence of the catalogue rather than a loosening:
`campaign.manage` is scoped to a campaign, `Authorize` never names a scope, so declaring manage would
deny everybody always. Per-campaign authority lives where it already provably lives, the
`campaign_recruiter` join, and the handlers route everything about a particular campaign through it:
reading, opening and granting all admit the caller by the join first, so the writes are guarded
exactly as tightly as the reads. A member who is not on a campaign gets the same not-found a campaign
that never existed produces, because telling those apart would let anybody enumerate who is hiring
for what.

The tenant-wide list is deliberately different: `campaign.read` is unscoped in the catalogue
precisely so a recruiter can see which campaigns exist before being assigned to one, and the list
therefore carries no pins and no determination, which an attack proves rather than a comment
promising it. The creator joins their own campaign in the transaction that creates it, because a
campaign its creator cannot open is a race nobody should be able to lose.

The opening failures each keep their own code, since the recruiter's next step differs per code: a
missing determination is counsel's to fix and the message says so in as many words, an unpublished
artifact is its author's, and an incomplete configuration is their own.

The architecture gate earned its keep here. The first draft of the surface imported
`internal/recruiting` directly, reasoning that a second copy of the types would be a translation
layer with nothing to translate; `TestNoContextImportsAnother` refused it, and the refusal was
correct. The vocabulary is now declared in the API package and translated in `cmd`, where every
other pair of contexts already meets.

Four attacks on the handlers, each failing its own named test: opening without the join, taking the
creator from the body, leaking pins on the tenant-wide list, and granting without the join.

**An attack on this found a real gap in the first version of these tests.** Making
`recruiting.campaign`'s own policy permissive left every test green, because the recruiter read goes
through a join with `campaign_recruiter` and was defended by that table's policy instead. The tests
were real but proved something narrower than they claimed. Two tests now read `campaign` and
`campaign_pin` directly, so nothing but each table's own policy stands in the way, and weakening
either one fails a named test that reports what leaked.

**Spec** [screen-mode.md](../../product/screen-mode.md) · [authorization-model.md](../../architecture/authorization-model.md)

---

### SCR-02 · Implement versioned candidate disclosure and consent

**Depends on** DEC-11, SCR-01 · **Blocks** SCR-04

Employer, purpose, AI involvement, recording, access, retention, rights route, accommodations, result
disclosure and human decision ownership — versioned, approved, and never bundled with optional
processing.

**Done when**
- [x] The exact disclosure version a candidate accepted is recorded with the acceptance.
- [x] Consent is unbundled: declining optional processing does not block the interview.
- [x] A disclosure change creates a new version and never rewrites what someone already accepted.

**One of three, and the two open ones are open for the same reason as SCR-01's third.**

The disclosure is a registry artifact rather than a table of its own, for the reason migration 0024
gave for consent texts: the words a person agreed to must resolve to identical words forever, and the
registry already keeps that promise. A campaign pins its disclosure by digest exactly as it pins its
rubric.

**Unbundling is done and proven.** It is a row per processing purpose rather than a boolean on the
acceptance, because that is what unbundling means structurally: a single "I agree" covering both the
interview and model improvement cannot be declined by half, and a schema able to record only one
answer would make the requirement impossible to satisfy however carefully the screen was written.
`MayProceed` looks only at required purposes, so an optional one may be granted, refused or never
answered and the interview is unaffected. An unanswered required purpose is its own outcome, distinct
from a refusal, because reading silence as agreement is the failure the separation exists to prevent.

Model improvement can never be marked required, refused in Go and again by a CHECK constraint, since
bundling by another name is still bundling. A disclosure must cover all ten areas screen-mode.md and
responsible-hiring.md list, and a section present but blank counts as absent, which is how that guard
would most plausibly be defeated by accident.

Five guards proven by breaking them, including one that catches the opposite mistake: making optional
consent block the interview fails a named test too, so the rule cannot be tightened by accident any
more than it can be loosened.

**The write path landed, and the first and third boxes closed with it.** An acceptance and its
consent decisions are written in one transaction, because an acceptance without its decisions would
read as consent to everything and decisions without their acceptance would be answers to a document
nobody is recorded as having seen.

Both are proven against real PostgreSQL. A disclosure republished from 1.0.0 to 2.0.0 leaves the
earlier acceptance standing with its own digest, which is the third criterion happening rather than
being asserted, and the row itself refuses UPDATE and DELETE, so no code path can quietly restate
what somebody agreed to. Making the table editable fails that test by name. The database refuses
model improvement as a required consent even when a caller skips `ValidatePurposes` entirely, which
is what a future caller that forgot would do.

The standing decision is the latest row per purpose, so SEC-04's withdrawal is already expressible:
a change of mind is a new row and nothing is edited.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md)

---

### SCR-03 · Implement job context capture for a campaign

**Depends on** SCR-01, PRO-03 · **Blocks** EVL-06

The job description or role context the interview draws on, extracted into requirements that EVL-06 can
report evidence against.

**Done when**
- [ ] Requirements are extracted with provenance and are reviewable by the recruiter before issue.
- [ ] The recruiter can correct an extracted requirement.
- [ ] Requirements are pinned into the campaign configuration.

**Spec** [screen-mode.md](../../product/screen-mode.md)

---

### SCR-04 · Implement invitation issue, delivery, expiry and revocation

**Depends on** SCR-02, INT-01 · **Blocks** SCR-05

Single-use expiring links, delivery tracking including bounces, resend, and revocation whose
consequences are previewed before it happens.

**Done when**
- [ ] An invitation token is single-use, expiring and safe against enumeration.
- [ ] Revocation states plainly what it does and does not delete.
- [ ] Delivery failures are visible to the recruiter and recoverable.

**Spec** [public-api.md](../../contracts/public-api.md)

---

### SCR-05 · Build invitation acceptance and identity resolution

**Depends on** SCR-04, IAM-01 · **Blocks** SCR-06

The candidate opens the link, the token is validated, and their identity is resolved without ever
revealing another candidate's existence.

**Done when**
- [ ] Acceptance works for a new account and an existing one without leaking whether either exists.
- [ ] An expired or revoked invitation has its own outcome with a route to the employer.
- [ ] Declining is a first-class outcome recorded without penalty.

**Spec** [user-journeys.md](../../product/user-journeys.md) · [threat-model.md](../../security/threat-model.md)

---

### SCR-06 · Implement the accommodation request and fulfilment path

**Depends on** SCR-02, SES-03 · **Blocks** SCR-08

Captions, push-to-talk, extra thinking time, and an alternative assessment path when voice is not
accessible — none of which change the rubric or the anchors.

**Done when**
- [ ] Accommodations are requestable before and during preparation, and are recorded on the session.
- [ ] Response latency is excluded from evaluation for every candidate, accommodated or not.
- [ ] An alternative path exists and is exercised, not merely promised.

**Spec** [screen-mode.md](../../product/screen-mode.md) · [responsible-hiring.md](../../security/responsible-hiring.md)

---

### SCR-07 · Enforce screening candidate result disclosure policy server-side

**Depends on** DEC-11, IAM-04 · **Blocks** REL-03

Whatever DEC-11 decided, the API enforces it. Hiding a link is not a control.

**Done when**
- [ ] Direct API access to an evaluation the policy does not permit fails, regardless of the interface.
- [ ] URL manipulation cannot reach coaching, evidence, notes, comparison or decisions.
- [ ] The permitted status view contains exactly what the approved policy allows and nothing more.

**Not started, but half of the enforcement is already structural and worth recording so it is not
rebuilt.** The candidate result reads, `getResults` and `getTranscript`, are gated on
`session.read_own_practice` and their adapters hardcode `Mode: "practice"`: the session is looked up
as practice owned by the caller, so a screening evaluation is not filtered out of the response, it is
never found. URL manipulation cannot reach a screening evaluation through the candidate result path
because that path only ever asks for practice.

What is missing is the other direction: the permitted status view a screening candidate is allowed,
driven by the campaign determination's `result_disclosure`. That cannot be built yet, and the reason
is a fact rather than a choice: a session carries no `campaign_id`, so a screening session as a
fully-wired concept does not exist until SCR-04 and SCR-05 issue and accept an invitation into one.
Building the disclosure view now would mean inventing the session-to-campaign link those tickets own,
and testing it would mean constructing a screening session there is no honest way to construct. The
enforcement mechanism is ready; the thing it enforces on is not built.

**Spec** [screen-mode.md](../../product/screen-mode.md)

---

### SCR-08 · Implement screening interruption and re-invitation governance

**Depends on** SES-06, DEC-14, SCR-06 · **Blocks** nothing

A connection or device failure in a screening interview goes to a human, not to an automatic retry
policy that quietly advantages whoever has better broadband.

**Done when**
- [ ] Interruptions are recorded with cause, duration and resulting coverage.
- [ ] Re-invitation requires a named human decision and records the reason.
- [ ] The candidate is told what happened and what happens next.

**Spec** [screen-mode.md](../../product/screen-mode.md)

---

### SCR-09 · Enforce the supported language, accent and audio-quality boundary

**Depends on** DEC-13, ART-02 · **Blocks** REL-03

Outside the supported matrix, the product says so and offers an accommodation — it does not quietly
produce a worse evaluation.

**Done when**
- [ ] Out-of-matrix conditions produce an explicit assessability status and a warning.
- [ ] The published matrix is reachable by candidates and recruiters, not buried internally.
- [ ] Fairness monitoring in QUA-05 tracks outcomes at the boundary.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md)
