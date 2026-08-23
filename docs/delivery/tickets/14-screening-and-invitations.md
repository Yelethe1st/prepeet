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
- [ ] A campaign cannot be opened against a draft or unpublished configuration.
- [ ] Publishing a new rubric version does not alter a running campaign or re-score completed interviews.
- [ ] Recruiter access is scoped per campaign, enforced server-side.

**Spec** [screen-mode.md](../../product/screen-mode.md) · [authorization-model.md](../../architecture/authorization-model.md)

---

### SCR-02 · Implement versioned candidate disclosure and consent

**Depends on** DEC-11, SCR-01 · **Blocks** SCR-04

Employer, purpose, AI involvement, recording, access, retention, rights route, accommodations, result
disclosure and human decision ownership — versioned, approved, and never bundled with optional
processing.

**Done when**
- [ ] The exact disclosure version a candidate accepted is recorded with the acceptance.
- [ ] Consent is unbundled: declining optional processing does not block the interview.
- [ ] A disclosure change creates a new version and never rewrites what someone already accepted.

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
