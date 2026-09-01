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
- [x] Settings changes are audited with actor and previous value.
- [x] Defaults apply to new campaigns only, never retroactively.
- [ ] Read-only users see the settings without controls, not a broken form.

**The two structural boxes are closed in the bounded context; the screen is not
built.** `tenancy.tenant_configuration` stores a workspace's settings as versions
rather than as a value, because both closed boxes need the same thing: the previous
document has to stay readable, exactly, forever. A save is an insert, append-only by
trigger and by grant, and both halves are proven by attacking them - the migrator
that owns the table cannot rewrite or delete a version, and `prepeet_app` holds
neither privilege.

The audit row carries the actor and the names of the fields that moved, computed
from the two documents rather than described by the caller; the previous values
themselves are the previous version, which is still there. That split is deliberate:
`audit.events` is exported to tenants and is not a place for candidate-facing
invitation copy, and "what was it before" is answered by reading version n-1.

"Never retroactively" is a property rather than a promise. A campaign pins the
settings version it was created under and `AtVersion` re-reads that document however
many times the workspace has saved since, which is tested by pinning, changing, and
re-reading. The pin's other half - recruiting actually recording the version on a
campaign - is not wired, because `internal/recruiting` was another agent's this
session.

Concurrency is the primary key: two administrators saving the same version collide
and the second is refused, so neither silently undoes the other. Validation runs
before storage, because a table that cannot delete anything past a draft is a bad
place to put a document nobody can use.

The third box is not started and cannot be from here. It needs a settings screen and
a `tenant.settings_read` capability distinct from `tenant.settings_manage`; the
capability catalogue lives under `packages/contracts/`, which this session was told
not to touch, so a read-only member currently has no capability that would reveal the
settings at all.

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
- [x] Dormant access is surfaced automatically.
- [x] Review completion is auditable.

**The review exists as a thing that has to be answered; nothing schedules it yet.**
The ticket's first line decided the design. A report is a query somebody could run; a
review is a row that exists, is due on a date, names every person who can reach
candidate evidence, and cannot be closed while one of them is unanswered for. That
last clause is enforced rather than encouraged: `Complete` refuses with
`ErrReviewIncomplete` while any item is pending, a unique partial index allows one
open review per workspace, and neither table grants DELETE to the application role,
because a review somebody can remove answers "has access been reviewed" with whatever
the last person to look wanted it to say.

The items are a snapshot taken at open time, not a live view, so a completed review
still shows what was confirmed and against which role. Revoking through the review
removes the access before the row is written and leaves the item pending if the
removal fails, because "revoked" written beside access that still works is a lie
somebody will act on.

Dormancy is computed at open time against a standard recorded on the review itself,
since "nobody was dormant" means different things read against 30 days and against
180. The signal is `audit.events`, which is the only tenant-scoped record of what a
person did in a workspace: a session belongs to a person across every workspace they
belong to and cannot answer "dormant here". The honest cost is stated in the package
README - somebody who only ever read pages has no audited act and reads as dormant.
That is a false positive in the conservative direction, prompting a confirmation
rather than hiding anybody, and the fix is the sensitive-read auditing the
authorization model already calls for.

Completion is audited with the actor, the review, and the counts confirmed and
revoked. The first box is open on its first word: `Due` answers per workspace and
`Open` is idempotent behind the unique index, but nothing drives them. A scheduler
would have to enumerate tenants, and nothing can do that today without a role holding
BYPASSRLS, which would bypass every other policy in the database. That is a decision
somebody should make deliberately rather than something to slip in here.

The roster and the revocation both arrive through ports (ADR-0005) and are wired in
neither `cmd/api` nor `cmd/worker` yet, because there is no endpoint to wire them to.

**Spec** [user-journeys.md](../../product/user-journeys.md)

---

### TEN-04 · Build the rubric library with immutable version history

**Depends on** CAT-01 · **Blocks** TEN-05

Draft, validate, approve, publish, and a version history that cannot be rewritten.

**Done when**
- [x] A published rubric is immutable; editing produces a new version.
- [x] Version history shows who published what and when.
- [x] A rubric in use by a running campaign cannot be deleted.

**Built as a surface over the artifact registry, and that is the whole ticket.**
`content.artifacts` already stores versioned, digest-identified, published artifacts
with a lifecycle, a separation of duties on publication, an immutability trigger and a
rollback path, and a rubric is one of its types. A second version history here would
have been a second answer to "what is version 1.1.0 of this rubric", and the second
answer is the one that drifts. So this ticket added no migration at all: the library
decides what a workspace may do with a rubric and the registry decides what a version
is.

The first box is the registry's trigger from migration 0013, and the library holds up
its end by offering no edit: a revision is a fresh draft of the same reference at a
new version, which is the only shape either side accepts. The second is the registry's
too - `Versions` returns every version with its drafter, its publisher and its
publication time, proven against real PostgreSQL, including that one workspace cannot
read another's history for a reference that genuinely exists.

The third box is half done and honestly so. The refusal is built, named
(`ErrRubricInUse`), carries the campaigns that are blocking so an administrator has
something to act on, and guards both discarding a draft and retiring a published
version - retiring deprecates rather than removes, because "what was this candidate
judged by" has to stay answerable. What is missing is the answer: `RubricUsage` is a
consumer-defined port and campaigns live in `internal/recruiting`, which was another
agent's this session, so nothing implements it yet. Until cmd wires it the guard is
tested against a fake and cannot be claimed.

Two refusals are the library's own rather than the registry's: a platform template is
readable by every workspace and is nobody's to change, and validation runs before
anything is drafted. Whether a body is a usable rubric is decided by the context that
reads rubric bodies, injected as a `RubricValidator` for the reason the artifact
loader injects its catalogue parser - writing a rubric schema here would have been the
same mistake as writing a second version history.

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

**Depends on** DEC-16 · **Blocks** SES-02, OPS-05 · **In progress**

What the tenant has used, what remains, and what happens at the limit — in the same terms the invoice
uses.

**Done when**
- [x] Usage counts match the billing unit decided in DEC-16 exactly.
- [x] Approaching and reaching the limit both produce a warning before anything is blocked.
- [x] A candidate is never interrupted mid-interview by a quota event.

**The ledger, the boundary and the warning are in; SES-02 consumes them next.** The unit is
ADR-0014's, held structurally: billing.usage_entries is append-only by trigger (proven by
attacking it), one start and at most one credit per session by unique constraint, and the
invoice is a sum - the boundary test shows a platform-interruption credit reopening exactly the
capacity it returns. Reservation locks the quota row, so eight concurrent starts at a limit of
one admit exactly one. The warning ladder is proven in order: none, approaching at the
threshold while starts still succeed (nobody's first notice is a refusal), reached at the
limit; and shrinking a quota below usage refuses new starts while rewriting nothing. Nothing
anywhere consults quota after a start, which is the third box as structure; SES-02 wires
ReserveStart into the start flow and the sixty-second early-abandon credit into completion.
GET /tenant/usage and /tenant/quota serve the same numbers under tenant.billing_read. What
remains here: administrator notification when the warning trips (with the notification epic),
and OPS-05's quota-setting surface.

**Spec** [cost-and-capacity-model.md](../../operations/cost-and-capacity-model.md)
