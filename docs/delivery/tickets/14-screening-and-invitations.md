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
- [x] An invitation token is single-use, expiring and safe against enumeration.
- [x] Revocation states plainly what it does and does not delete.
- [x] Delivery failures are visible to the recruiter and recoverable.

Built in `recruiting.invitation` (migration 0056) with the token discipline
identity's action tokens already set: only the sha256 hash is stored, the
plaintext lives for the one call that emails it, and the lookup is by hash of
32 random bytes, so there is nothing sequential to enumerate. Single-use is
the consume-guard on a null outcome; a resend supersedes every live link for
the recipient so two forwarded emails cannot both be accepted. Expiry is
computed from `expires_at` against the clock, not a stored flag, so a live link
past its window reads expired without a job flipping it. Revocation sets an
outcome and deletes nothing, and the endpoint says so; it is scoped to the
named campaign so a recruiter on one campaign cannot revoke another's link by
id. Delivery status rides on each invitation, read from `notification.emails`
in cmd (recruiting cannot name that schema), and resend is the recovery path,
refused for an invitation already accepted, declined or revoked. This also
advances INT-01's delivery-status item on the visible-where-it-matters half;
the provider-feedback ingestion that sets `bounced_at`/`complained_at` remains
INT-01's outstanding work.

**Spec** [public-api.md](../../contracts/public-api.md)

---

### SCR-05 · Build invitation acceptance and identity resolution

**Depends on** SCR-04, IAM-01 · **Blocks** SCR-06

The candidate opens the link, the token is validated, and their identity is resolved without ever
revealing another candidate's existence.

**Done when**
- [x] Acceptance works for a new account and an existing one without leaking whether either exists.
- [x] An expired or revoked invitation has its own outcome with a route to the employer.
- [x] Declining is a first-class outcome recorded without penalty.

Built as three public endpoints under `/screening/invitation`: resolve, accept
and decline, none authenticated because the candidate arrives holding only a
token. The candidate reaches their invitation row through the token-scoped
policy in migration 0057, which admits exactly the row whose stored hash equals
the `app.invitation_token_hash` the transaction carries: proof of holding the
token stands in for the tenant a candidate does not have, the way the practice
owner-policy stands in for it in 0012. Acceptance provisions the candidate in
identity when the address has no account and signs into the existing one when
it does, returning the same session either way, so acceptance never reveals
whether the address was registered (ADR-0003). A new candidate is passwordless
and verified, because arriving with an emailed token proves control of the
address. Resolving a link that expired, was revoked or was already answered
returns that outcome and the employer who issued it; a token that names nothing
is refused without saying more. Declining is a guarded, single-use outcome that
issues no session and creates no account. The threat-model T8 note is updated to
match.

**Spec** [user-journeys.md](../../product/user-journeys.md) · [threat-model.md](../../security/threat-model.md)

---

### SCR-06 · Implement the accommodation request and fulfilment path

**Depends on** SCR-02, SES-03 · **Blocks** SCR-08

Captions, push-to-talk, extra thinking time, and an alternative assessment path when voice is not
accessible — none of which change the rubric or the anchors.

**Done when**
- [ ] Accommodations are requestable before and during preparation, and are recorded on the session.
- [x] Response latency is excluded from evaluation for every candidate, accommodated or not.
- [ ] An alternative path exists and is exercised, not merely promised.

**One of three. The domain, the schema and the store, not yet the server path: the two open boxes
wait on the contract, exactly as TEN-01's did, because "requestable" and "exercised" are things a
candidate does and no candidate can reach code that has no HTTP surface.**

Three facts, three append-only tables in migration 0055: what was requested, what was granted and by
whom, and which session it was actually applied to. The state a candidate sees is derived from those
rows at read time and never stored, so it cannot disagree with them. Even attaching a request to its
session is an insert into the fulfilment table rather than an edit of the request, which is what lets
every one of the three tables refuse UPDATE and DELETE outright.

Two design constraints are structural rather than behavioural, because a rule that is a column you
did not create is harder to lose than a rule that is a check you wrote.

The request is a named adjustment from screen-mode.md's list, never a diagnosis. There is no
free-text column and no field to put one in, since a "reason" box on an accommodation form is where a
medical condition gets asked for whether or not anybody meant to ask. A unit test asserts the
struct's exact fields, so a place for a condition cannot appear without a reviewed diff, and the
schema's CHECK refuses a diagnosis-shaped value from a caller that skipped Go entirely.

An accommodation never reaches evaluation as a signal because evaluation cannot read it, not because
it promises not to. The tables live in the recruiting schema, the ownership gate refuses any other
module a query that names it, and ADR-0010 leaves no hand-written SQL to walk around that. Giving
evaluation a probe query that reads `recruiting.accommodation_request` fails
`TestAModuleNamesOnlyItsOwnSchemas` with a line naming exactly what leaked.

**The second box was mostly closed before this ticket and is ticked on named evidence, not on this
ticket's work.** `TestResponseLatencyIsInvisibleToScoring` (SES-05, in internal/evaluation) proves
two candidates giving the same answers at different speeds aggregate identically, for every
candidate because the aggregation cannot see timing at all, and QUA-01's datasets carry pause
positions precisely to prove the intelligence plane cannot read them. What SCR-06 adds is the other
half: accommodation status is equally invisible, so "accommodated or not" cannot re-enter as a
proxy. Extra thinking time therefore changes the session's clock and nothing else.

The fulfilment rule, that only a standing grant can be applied to a session, is enforced twice: the
store refuses with an error a caller can act on, and the database's trigger refuses again for the
future path that skips the store. Breaking the Go guard fails
`TestFulfilmentWithoutAGrantIsRefusedInGo` and, satisfyingly, the failure message is the trigger
firing, which is the defence in depth demonstrating itself. A grant later withdrawn is a newer
append-only row and stops further fulfilment, so the record of what stood when the interview ran
survives every change of mind. A decision without a named human is refused for the reason the
jurisdiction determination refuses an unnamed approver.

The phase rule fails closed: a request mid-interview is refused toward SCR-08's incident path, and a
caller that does not state the session phase is refused rather than read as "no session yet",
because silence read as early is how a request would slip in late. The session phase arrives as this
context's own vocabulary through the caller, since recruiting may not import interview (ADR-0005);
the composition root will translate when the surface lands.

Nine guards proven by breaking them, each failing a named test: the phase rule, the vocabulary, the
diagnosis field, FORCE RLS, request append-only, the fulfilment trigger, the tenant policy (attacked
at a row that exists under the other tenant, reporting what leaked), the Go grant guard, and the
evaluation boundary.

**What remains for the two open boxes.** The candidate request and status endpoints and the
recruiter review queue wait on the OpenAPI contract, which another stream holds. The alternative
path is grantable, fulfillable and recorded on a session today, and
`TestTheAlternativePathIsGrantedAndRecordedLikeAnyAdjustment` exercises that record; conducting an
alternative assessment session is the interview context's work and is the difference between this
record and the criterion's "exercised". Migration 0055 also admits `accommodation_policy` as a
registry artifact type, so a campaign can pin one by digest exactly as it pins its rubric; making it
required at open is deliberately left until the policy artifact has an authoring surface, for a
campaign cannot be asked to pin what nobody can yet publish.

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
