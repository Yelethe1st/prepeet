# Epic IAM — Identity, tenancy and authorization

**Phase 1–2** · **Workstream** Go, Security/privacy, Web

Every protected capability in the product waits on this epic. Deny by default, one explicit active
tenant per request, capabilities rather than hardcoded role checks, and backend policy that does not
care what the navigation chose to render.

---

### IAM-01 · Implement registration, login, logout and session refresh

**Depends on** DEC-02, CTR-01 · **Blocks** IAM-02 · **In progress**

Candidate and organisation registration, password login, logout and refresh, on secure HTTP-only
browser sessions with CSRF defence.

Built against [ADR-0003](../../architecture/decisions/0003-identity-built-in-go.md). The service is in
`services/platform/internal/identity` with migration 0002 behind it, and the HTTP layer is in
`internal/api`, served by `cmd/api` against a real database. Register, login, refresh, logout and `/me`
were exercised end to end: two cookies on login and refresh, identical responses for a new and a known
address, and a session that a still-valid cookie cannot reach once logout has revoked it.

Two things were settled while building it.

**The generated response types cannot express two cookies.** oapi-codegen models a response header as one
field and writes it with `Header().Set`, which replaces rather than appends, so whichever cookie is
written second is the only one the browser receives. The responses for login, refresh and logout are
therefore written by hand. They still satisfy the generated interfaces, so the contract remains the
source and a handler returning the wrong shape still fails to compile. Changing the contract to describe
one cookie was rejected: that would make the document lie about the wire to suit a generator.

**The API layer declares what it needs from identity.** `internal/api` cannot import `internal/identity`
under ADR-0005, and the boundary test enforces it, so the port is declared by the consumer and `cmd/api`
translates. That translation earns its cost in one specific place: identity distinguishes `ErrNotFound`
from `ErrCredentialsInvalid` because its own logic needs to, and the API must not, since a response that
could tell them apart is an account-existence oracle. The collapse happens once, in the adapter.

Organisation registration creates the person, the workspace and the owning membership in one
transaction. The atomicity is the requirement rather than a nicety: a half-created registration gives
somebody who can verify their address, sign in, and find no workspace, with the address now taken by an
account nobody can complete. Verified by committing the user early and watching the assertion fail.

Two things about row-level security surfaced here and are recorded in
[ADR-0002](../../architecture/decisions/0002-postgresql-schema-rls-and-connection-roles.md). Creating a
tenant is an act performed as that tenant, because the policy is written against the primary key. And
"which workspaces do I belong to" cannot be scoped by tenant, so migration 0007 answers it with two
`SELECT`-only policies keyed on the acting user rather than with a `WHERE` clause somebody can forget.

Slugs are generated from the organisation name and retried on collision, because two organisations may
legitimately be called Acme and refusing the second signup would be absurd. The suffix is random rather
than a counter: `acme-2` existing would tell anyone that two organisations of that name registered.

The role vocabulary is deliberately two values, `owner` and `member`. The full matrix is
[TEN-02](16-tenant-administration.md), and inventing names here would mean either that ticket inherits
choices made without its analysis or a migration to undo them.

**Done when**
- [x] Passwords use argon2id, carry their parameters, and upgrade transparently on next login.
- [x] Tokens are opaque, carry 256 bits of entropy, and are stored hashed rather than in plaintext.
- [x] A dummy verification exists so login timing does not distinguish an unknown address.
- [x] Registration answers identically for a new and an existing address, and re-registering does not overwrite the password.
- [x] A wrong password and an unknown address return the same error and cost comparable time.
- [x] Refresh rotates, and presenting a retired token revokes the whole session family while leaving other families alone.
- [x] Logout revokes the family and is idempotent.
- [x] A post-login destination is preserved without allowing an open redirect, refusing anything that could be read as a scheme, an authority or a header break.
- [x] Session tokens travel in HTTP-only, SameSite cookies and never in a response body, with the refresh cookie scoped to the refresh endpoint.
- [x] The four operations are served over HTTP against the generated interface.
- [x] Organisation registration creates the tenant and the owning membership.

The browser reaches these endpoints on its own origin, which is what the contract means by declaring its
server as `/api/v1` rather than an absolute URL. Locally a Next rewrite arranges it; in a deployed
environment the load balancer does. Pointing the client at the API's own port instead would need CORS
configuration that the local arrangement would not exercise, and would make the session cookie's
`SameSite` behaviour something that happens to work rather than something that was decided.

**Spec** [product-requirements.md](../../product/product-requirements.md) · [public-api.md](../../contracts/public-api.md)

---

### IAM-02 · Implement email verification, password recovery, magic link and OTP

**Depends on** IAM-01, INT-01 · **Blocks** nothing

Four token-bearing flows with different expiry, single-use and device-binding characteristics. The
prototype already distinguishes their states — signing in, already used, wrong device, expired.

**Done when**
- [x] Every token is single-use, expiring, and safe to replay without side effects.
- [x] Resend is rate-limited with a visible cooldown.
- [x] Expired and already-used cases have their own outcomes rather than one generic failure.

**Backend complete.** One `identity.action_tokens` table carries all four flows, because they are the
same shape with different expiries: a secret sent somewhere that proves control of that somewhere,
exactly once, for a while. Single-use is transactional rather than checked: the mark and the effect
share one transaction and a guarded update decides races, so exactly one of two concurrent
presentations wins, which a repository-level test proves because no sequential test can. Requesting
a new token supersedes the old immediately, as the prototype promises, and the superseded link earns
its own outcome. Recovery revokes every session in the same transaction that changes the password.
The cooldown is charged before the address is looked up, so it cannot become the enumeration oracle
the identical 202s exist to prevent, and the 429 carries Retry-After plus the same number in the
body for the countdown. Six-digit codes die on the fifth wrong guess.

**Screens ported.** forgot-password, check-email, reset-password, verify-email, magic-link and
otp are routes; auth-expired ported as the shared TokenTrouble component, because it is the same
screen with six wordings and writing them side by side is what keeps the outcomes honest. Each
error code renders its own state, every dead state leads with what did not happen, and the resend
countdown takes its number from the server's Retry-After. Deviations from the prototype, each
recorded where it lives: the reset screen claims only the rules the server enforces; the OTP entry
is one input rather than six boxes, because six inputs for one value fight screen readers and the
phone's code autofill; the attempts counter and recovery-code path did not port because neither
exists behind them.

**Spec** [user-journeys.md](../../product/user-journeys.md) · [threat-model.md](../../security/threat-model.md)

---

### IAM-03 · Implement tenant membership and explicit active-tenant context

**Depends on** IAM-01, PLT-03 · **Blocks** every tenant-scoped capability

A user may belong to several tenants. Every request operates under exactly one, resolved explicitly and
never inferred from a resource identifier.

**Done when**
- [x] `GET /me/memberships` and `PUT /me/active-tenant` work and are audited.
- [x] No handler infers tenant from a path parameter.
- [ ] Switching tenant cannot expose a resource from the previous one, including through a cached read model.

**The selection lives on the session**, not in a cookie or a header. A client-supplied tenant is a claim
the server must verify on every request, and the request that forgets is a cross-tenant read. Stored
server side it is verified once, when it is chosen, and read thereafter from a row the client cannot
reach. Revoking a session revokes the selection with it.

Signing in never selects a workspace, including for somebody who has just registered one. A session that
chose on the user's behalf would mean the first request after login was scoped by something nobody
picked.

**Refusing a workspace is 403, and both alternatives are wrong.** 404 would let anyone test which tenant
identifiers are real. 401 would sign somebody out for clicking a workspace that is not theirs. The
refusal says nothing about whether the workspace exists, asserted by a test that looks for the words.

**Both the selection and its refusal are audited**, in the same transaction as the write. The refusal is
the one worth keeping: repeated attempts on workspaces somebody does not belong to is the shape of an
account probing for access. Migration 0008 adds `audit.events` as append-only by grant rather than by
nobody writing the `UPDATE`, which is verified by trying.

**The path rule is mechanical.** Nobody breaks it deliberately; it breaks because
`/tenants/{tenantId}/sessions` looks natural and the handler scopes itself to what is right there. A test
walks the contract and refuses any path parameter that names a tenant, and adding such an endpoint also
fails to compile until a handler exists. Both were checked by adding one.

**Remaining.** The third box needs something to switch between. Nothing tenant-scoped is served yet, so
there is no read model to leak through and nothing to assert. The pieces are in place and tested:
`Principal.ActiveTenantID` carries the selection, and `database.SetTenant` scopes a transaction to it.
The first tenant-scoped endpoint joins them, and that is where this becomes checkable.

**Spec** [authorization-model.md](../../architecture/authorization-model.md)

---

### IAM-04 · Build the capability catalogue and policy evaluation service

**Depends on** IAM-03 · **Blocks** every protected route · **In progress**

Roles are bundles of capabilities. Authorization evaluates identity, capability, tenant, resource scope,
purpose and resource state, and is the single place that decides.

Built in `services/platform/platform/authz`, 29 tests, no dependency on the identity provider decision
in DEC-02. What remains is binding it to real sessions and memberships, which needs IAM-01 and IAM-03,
and the role bundles themselves, which need the tenant role model.

**Done when**
- [x] The capability catalogue is a versioned contract, free of page-specific names, enforced by test.
- [x] Policy evaluation is one code path, and every decision carries the reason an audit record needs.
- [x] Deny is the default for an unknown capability, an empty context, a missing tenant and a missing scope.
- [x] A scoped capability asked without a scope is denied, so a list endpoint cannot leak by omission.
- [x] Own-data capabilities cannot be satisfied by tenant authority, structurally rather than by filtering.
- [x] Privileged platform capabilities require an active elevation carrying a reason and a ticket.
- [x] Destructive and evaluation-changing capabilities require recent authentication.
- [x] A subject cannot grant authority it does not hold.
- [x] Roles are defined as capability bundles, which needs the tenant role model in TEN-02.
- [x] The catalogue is published as a versioned machine-readable contract alongside the API.

**The contract is now the source, and the Go is generated from it.** Which authority reaches a
candidate's practice history, what needs recent authentication, and what needs an elevation carrying a
ticket are questions legal and security have to answer from one artifact without reading Go. That is the
same argument ADR-0004 makes for the API contract, and it applies here more strongly: this document is
the authorization model.

Every entry carries the reason for its requirements, and the generator refuses to emit one without a
reason. A requirement with no reason is a rule nobody can argue against when somebody proposes changing
it.

Three properties are asserted across the whole catalogue rather than in the entries somebody thought to
test: a candidate capability requires owner and never tenant, so practice data stays unreachable from an
employer; a platform capability is never also a tenant one, so platform staff are not members of every
workspace; and no name contains an interface element. All three were checked by introducing a violation.

That last check was wrong first time and the correction is worth keeping. It matched substrings, so
`evaluation.review` was reported as named after a view. It matches whole segments now, and `screen` is
deliberately not forbidden: in this product a screening interview is a mode rather than a page, so
forbidding it would forbid the vocabulary the product is written in.

TypeScript names are generated alongside, which is what [CTR-01](03-contracts-and-codegen.md) needs to
declare a required capability per operation and what WEB-02 needs to render navigation.

The last box closed when TEN-02's role model landed: the tenant vocabulary is recruiter,
hiring_manager, viewer, admin and owner, each a bundle in the contract with its reason, derived
from the prototype's own permission matrix. The property tests grew with it: the administrator
holds everything any membership role does, owner and admin are capability-identical (what
distinguishes an owner is the anchor to the workspace's creation, not holding more), the viewer
holds only reads, and a recruiter raises re-reviews but can never resolve one - the matrix's one
asymmetric row, carried by the new appeal.raise capability.

**Spec** [authorization-model.md](../../architecture/authorization-model.md)

---

### IAM-05 · Implement the tenant and workspace switcher in the web application

**Built into the application shell** rather than as a screen of its own, because switching workspace is
something somebody does while doing something else. See [WEB-02](05-web-foundation.md).

**Depends on** IAM-03, WEB-02 · **Blocks** nothing

*Gap found against the prototype: no switcher exists, so multi-tenant membership is unreachable in the
interface even though the API supports it.*

**Done when**
- [ ] A user in more than one tenant can see and switch active tenant from the application shell.
- [ ] The active tenant is visible at all times, not only inside the switcher.
- [ ] Switching re-fetches rather than re-labels, and never renders stale tenant data.

**Spec** [information-architecture.md](../../product/information-architecture.md)

---

### IAM-06 · Enforce practice and screening authority separation

**Depends on** IAM-04 · **Blocks** SCR-01, REV-01

A tenant's authority never reaches a candidate's practice history, in either direction, through any
route, read model, cache, analytics table or export.

**Done when**
- [x] Adversarial tests attempt practice reads under tenant authority and fail at both the policy layer and RLS.
- [x] Analytics and search projections carry the same separation.
- [x] A leak in this path is wired as a stop-ship alert, not a bug report.

**Done, with the alert half stated precisely.** The policy layer's proof iterates every owner
capability in the generated catalogue against a deliberately over-provisioned tenant subject, so a
new owner capability is covered the moment it exists. The RLS proof lives in the isolation suite:
migration 0011 creates the candidate schema's first table with owner-only policy and no tenant
dimension, and adversarial tests read and write it under every context shape a tenant-side code
path could run with, including naming the row by id. A write tripwire trigger refuses the one
shape the policy cannot catch - the owner's own row written inside a transaction that also
carries tenant context - and its exception says stop-ship, because that is what it is.

Projections carry the separation structurally: the suite refuses any view or materialized view
joining a candidate table to anything carrying tenant_id, and the detector proves itself against
a planted offender on every run. Future candidate tables inherit every rule by existing: the
guards read the catalog, not a table list.

What "stop-ship alert" means today: the adversarial suite is part of the required CI gate, the
runtime write tripwire fails closed with a message naming the invariant, and reads under tenant
authority return nothing by construction. The pager rule on the tripwire's log line lands with
the first practice repository, which is the first code that could trip it.

**Spec** [authorization-model.md](../../architecture/authorization-model.md) · [responsible-hiring.md](../../security/responsible-hiring.md)

---

### IAM-07 · Implement time-bound platform elevation with reason and audit

**Depends on** IAM-04 · **Blocks** OPS-07

Support access to tenant data is exceptional: scoped, reason-bound, ticket-linked, time-limited, and
recorded whether or not anything was read.

**Done when**
- [x] Elevation requires reason and ticket, expires automatically, and can be revoked.
- [x] Every read performed under elevation is separately audited.
- [x] An unexpired elevation is visible to the operator and to their team.

**Done.** The grant cannot exist without a reason and a ticket - by CHECK as well as validation -
cannot outlive the one-hour cap, which refuses rather than clamps because an operator silently
granted less time than they asked for believes they hold time they do not, and dies at its
timestamp with no job to run: liveness is the comparison in the query. Revocation is immediate,
guarded, and audited in the same transaction as the grant's ending; the active list joins to
users so a teammate reads a name, a reason and a ticket, not identifiers.

The second criterion is enforced at the one choke point every authenticated request passes
exactly once: session lookup. A request made while a grant is active writes its own audit row
naming the grant and carrying the request id, so a future endpoint cannot forget to be recorded
under elevation by never knowing it had to be, and a lookup whose audit write fails fails the
request - an elevated read that cannot be recorded is exactly the read the criterion forbids.
Proven by counting: two requests under a grant are two rows naming it, requests before and after
are none. The capability gate (platform.privileged_elevate) and the console land with OPS-07,
which this unblocks.

**Spec** [authorization-model.md](../../architecture/authorization-model.md) · [observability.md](../../operations/observability.md)

---

### IAM-08 · Implement configured OAuth sign-in and account linking

**Depends on** IAM-01, IAM-02 · **Blocks** WEB-06's sign-in and registration screens

Sign in and register with Google and Microsoft, on the same session the password flow issues.

This ticket exists because the plan lost it. [DEC-02](01-decisions-and-adrs.md) settled that
"password, OAuth, magic link, OTP and recovery are required for the first release", and
[ADR-0003](../../architecture/decisions/0003-identity-built-in-go.md) lists "configured OAuth" among
the flows built in Go. IAM-01 delivered the first item on that list and IAM-02 delivered the last
three. OAuth sat in the middle of the same sentence and was never written down as work, so nothing
scheduled it and nothing gated on its absence. There is no OAuth code in `internal/identity` and no
provider endpoint in the OpenAPI document.

Enterprise federation is **not** in scope and is not what this is. ADR-0003 defers SSO and SCIM
deliberately, to be adopted for tenant members only, behind an adapter, and priced against the buyer
who asks. The prototype's `login.html` stacks "Continue with Okta (Northwind Health)" alongside
Google and Microsoft, which reads as one feature and is two: the Okta button belongs to that deferred
work and stays out until a buyer pays for it. Only the consumer providers are built here.

The seam is already in place. ADR-0003's risk table promised that "the identity module keeps a single
authentication entry point so a federation adapter attaches at one boundary", and `identity.Service`
has one: `Authenticate` issues the session, and everything about a password is decided before it.
An OAuth identity that resolves to a user has to arrive at that same call rather than mint a session
of its own, or there are two ways to be signed in and only one of them is audited, rate-limited and
revocable.

**Done when**
- [ ] The contract gains the provider endpoints, and the generated code and the handlers follow it.
- [ ] `state` and PKCE are mandatory, single-use and time-bound, and a replayed or absent `state` is refused rather than tolerated.
- [ ] An OAuth sign-in issues the same session the password flow issues, through the same entry point, with the same cookies, rotation and revocation.
- [ ] Linking rules are explicit and proven: an OAuth identity whose verified email matches an existing account links to it; an unverified email from a provider never does.
- [ ] A provider that is unreachable, slow, or returns an error leaves the person on a screen that names what happened and offers email and password, without a half-created account behind it.
- [ ] Every provider is configured rather than compiled in, so adding one is configuration and a test.
- [ ] The callback screen is ported from `screens/oauth-callback.html` with its four states: processing, slow, invalid state and expired code.
- [ ] Registration through a provider creates the same account a form does, including account type, and never silently creates a tenant.

**Watch for**

An OAuth identity meeting an existing password account is the whole risk of this ticket, and the
failure mode is account takeover: a provider that asserts an email it has not verified, linked to an
account somebody else owns, hands over that account. The email is not the identity; a verified email
from a provider that says so is. Anything else creates a new account or refuses.

The second is a half-created account. The callback does several things that can each fail after the
provider has already succeeded, and a person who lands back on the sign-in screen with an account
that exists but cannot be signed into has no way to describe what went wrong. One transaction, as
organisation registration already does.

**Spec** [authorization-model.md](../../architecture/authorization-model.md) · [ADR-0003](../../architecture/decisions/0003-identity-built-in-go.md)
