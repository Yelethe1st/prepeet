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
- [ ] Every token is single-use, expiring, and safe to replay without side effects.
- [ ] Resend is rate-limited with a visible cooldown.
- [ ] Expired and already-used cases have their own outcomes rather than one generic failure.

**Spec** [user-journeys.md](../../product/user-journeys.md) · [threat-model.md](../../security/threat-model.md)

---

### IAM-03 · Implement tenant membership and explicit active-tenant context

**Depends on** IAM-01, PLT-03 · **Blocks** every tenant-scoped capability

A user may belong to several tenants. Every request operates under exactly one, resolved explicitly and
never inferred from a resource identifier.

**Done when**
- [ ] `GET /me/memberships` and `PUT /me/active-tenant` work and are audited.
- [ ] No handler infers tenant from a path parameter.
- [ ] Switching tenant cannot expose a resource from the previous one, including through a cached read model.

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
- [ ] Roles are defined as capability bundles, which needs the tenant role model in TEN-02.
- [ ] The catalogue is published as a versioned machine-readable contract alongside the API.

**Spec** [authorization-model.md](../../architecture/authorization-model.md)

---

### IAM-05 · Implement the tenant and workspace switcher in the web application

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
- [ ] Adversarial tests attempt practice reads under tenant authority and fail at both the policy layer and RLS.
- [ ] Analytics and search projections carry the same separation.
- [ ] A leak in this path is wired as a stop-ship alert, not a bug report.

**Spec** [authorization-model.md](../../architecture/authorization-model.md) · [responsible-hiring.md](../../security/responsible-hiring.md)

---

### IAM-07 · Implement time-bound platform elevation with reason and audit

**Depends on** IAM-04 · **Blocks** OPS-07

Support access to tenant data is exceptional: scoped, reason-bound, ticket-linked, time-limited, and
recorded whether or not anything was read.

**Done when**
- [ ] Elevation requires reason and ticket, expires automatically, and can be revoked.
- [ ] Every read performed under elevation is separately audited.
- [ ] An unexpired elevation is visible to the operator and to their team.

**Spec** [authorization-model.md](../../architecture/authorization-model.md) · [observability.md](../../operations/observability.md)
