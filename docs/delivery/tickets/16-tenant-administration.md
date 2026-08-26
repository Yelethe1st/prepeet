# Epic TEN — Tenant administration

**Phase 5** · **Workstream** Go, Web

Organisation settings, members and scoped roles, the rubric library, calibration publication, retention
policy, quota visibility and API access. Published configuration is immutable; an update creates a new
version and never touches a session in flight.

---

### TEN-01 · Implement tenant settings and branding

**Depends on** IAM-03, WEB-02 · **Blocks** nothing

Organisation identity, defaults, candidate-experience settings and notification configuration, scoped to
a tenant administrator.

**Done when**
- [ ] Settings changes are audited with actor and previous value.
- [ ] Defaults apply to new campaigns only, never retroactively.
- [ ] Read-only users see the settings without controls, not a broken form.

**Spec** [product-requirements.md](../../product/product-requirements.md)

---

### TEN-02 · Implement members, scoped roles and the permission matrix

**Depends on** IAM-04 · **Blocks** TEN-03

Invite, assign capability bundles, scope a recruiter to campaigns, remove access, and show what each
role can actually do.

**Done when**
- [x] The permission matrix is generated from the capability catalogue, not written by hand.
- [x] Removing a member revokes access immediately, including active sessions.
- [x] Scope changes are audited and take effect without a redeploy.

**Done.** The screen landed on the backend below: /workspace/members shows everyone with role and
status, revealed by tenant.member_read so a read-only member sees the workspace without controls
rather than a broken form (the note says which role changes things), the owner's row offers
controls to nobody, and every change carries the version its row was read at - a conflict shows
the refusal and rereads rather than retrying blind. The invitation form explains its own floor:
the address must already hold an account, and acceptance is opening the workspace.

The first box is held the strong way: authzgen now emits the bundles, role reasons, capability
reasons and the scoped set into the TypeScript contract, and the matrix on the page is derived
from those at render - the tests pin that every cell traces to the generated bundles and every
"scoped" marking to the contract's own scope requirement, so the table an administrator reads
and the authority the server grants cannot drift apart. The words on screen are the reasons
legal and security reviewed, verbatim.

The member lifecycle runs end to end: an
administrator invites an existing account (the email journey joins when notification owns it),
the invitation is accepted by selecting the workspace - which is also where a revoked-then-
reinvited person gets their original row back, because the decisions they recorded stay
attributed - and every operation is decided by the one policy path: the handler names the
capability, identity.Authorize builds the authz context from the live session and lets Can
decide, so member_manage's step-up requirement holds without the endpoint knowing it exists.

The two closed boxes are proven on live sessions, not claimed: a role change lands on the
member's very next request (the demoted recruiter's own session loses campaign.manage with no
re-login), its audit row carries previous_role, and a revocation strips a still-signed-in
session immediately - capabilities are recomputed from the membership per request, so there is
nothing to propagate and nothing to redeploy. The owner's row is untouchable through this
surface and 'owner' is not assignable here: ownership transfer is a deliberate act for a later
flow, and the anchor means a workspace always has an administrator nobody inside it can remove.
Cross-tenant confinement is the schema's own scope, tested by an administrator of one workspace
failing to see or touch another's membership even by id. What remains is the members screen and
the matrix generated from the catalogue's own bundles.

**In progress: the role model is in.** The vocabulary from the prototype's matrix - recruiter,
hiring_manager, viewer, admin, beside the creator-anchored owner - lives in the capability
contract as bundles with reasons, generated into both languages, with 0006's deliberate
two-value floor widened by migration ('member' rows became 'recruiter', the rename lifted FORCE
row security for exactly one statement so a non-superuser migrator cannot silently update
nothing). appeal.raise joined the catalogue so the matrix's one asymmetric row - recruiters
raise re-reviews, hiring managers resolve them - is a capability difference rather than an
interface promise. What remains is the surface: the members endpoints, immediate revocation
proven against a live session, the audit of role changes with previous value, and the matrix
screen generated from the catalogue.

**Spec** [authorization-model.md](../../architecture/authorization-model.md)

---

### TEN-03 · Implement periodic access review

**Depends on** TEN-02 · **Blocks** nothing

Who has access to candidate evidence, when it was last reviewed, and a prompt to confirm or revoke.

**Done when**
- [ ] Access review is a scheduled prompt with a recorded outcome, not a report nobody opens.
- [ ] Dormant access is surfaced automatically.
- [ ] Review completion is auditable.

**Spec** [user-journeys.md](../../product/user-journeys.md)

---

### TEN-04 · Build the rubric library with immutable version history

**Depends on** CAT-01 · **Blocks** TEN-05

Draft, validate, approve, publish, and a version history that cannot be rewritten.

**Done when**
- [ ] A published rubric is immutable; editing produces a new version.
- [ ] Version history shows who published what and when.
- [ ] A rubric in use by a running campaign cannot be deleted.

**Spec** [domain-model.md](../../architecture/domain-model.md)

---

### TEN-05 · Build calibration authoring, impact preview and publication

**Depends on** TEN-04 · **Blocks** nothing

Per-tenant, per-role anchors, thresholds and weights, with a preview of what publishing would change
before it is published.

**Done when**
- [ ] The impact preview shows the effect on historical sessions without applying it.
- [ ] Publication never re-scores a completed interview.
- [ ] Whether historical re-evaluation is offered at all is an explicit, recorded decision.

**Spec** [evaluation-system.md](../../architecture/evaluation-system.md)

---

### TEN-06 · Implement tenant disclosure and accommodation policy management

**Depends on** SCR-02, SCR-06 · **Blocks** nothing

The tenant's disclosure variants and the accommodations they offer every candidate, versioned and
approved.

**Done when**
- [ ] A disclosure cannot be edited in place once it has been shown to a candidate.
- [ ] Accommodation policy changes apply to new invitations only.
- [ ] Legal approval is recorded against each version.

**Spec** [responsible-hiring.md](../../security/responsible-hiring.md)

---

### TEN-07 · Implement retention policy configuration and legal hold

**Depends on** DEC-15, SEC-05 · **Blocks** SEC-06

Tenant-level retention within the bounds legal approved, plus legal hold with authority, scope, reason,
expiry and review.

**Done when**
- [ ] A tenant cannot configure retention outside the approved bounds.
- [ ] A legal hold records authority, scope, reason and review date, and excludes data from ordinary deletion.
- [ ] Holds past their review date are surfaced rather than left indefinitely.

**Spec** [retention-and-deletion.md](../../security/retention-and-deletion.md)

---

### TEN-08 · Implement usage, quota and billing visibility

**Depends on** DEC-16 · **Blocks** SES-02, OPS-05

What the tenant has used, what remains, and what happens at the limit — in the same terms the invoice
uses.

**Done when**
- [ ] Usage counts match the billing unit decided in DEC-16 exactly.
- [ ] Approaching and reaching the limit both produce a warning before anything is blocked.
- [ ] A candidate is never interrupted mid-interview by a quota event.

**Spec** [cost-and-capacity-model.md](../../operations/cost-and-capacity-model.md)
