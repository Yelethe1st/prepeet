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
`services/platform/internal/identity` with migration 0002 behind it, 27 tests including the
reuse-detection behaviour verified by breaking the implementation on purpose. What remains is the HTTP
layer: the four operations are declared in the OpenAPI contract but no handler serves them yet, and
organisation registration does not yet create the tenant and owning membership.

**Done when**
- [x] Passwords use argon2id, carry their parameters, and upgrade transparently on next login.
- [x] Tokens are opaque, carry 256 bits of entropy, and are stored hashed rather than in plaintext.
- [x] A dummy verification exists so login timing does not distinguish an unknown address.
- [x] Registration answers identically for a new and an existing address, and re-registering does not overwrite the password.
- [x] A wrong password and an unknown address return the same error and cost comparable time.
- [x] Refresh rotates, and presenting a retired token revokes the whole session family while leaving other families alone.
- [x] Logout revokes the family and is idempotent.
- [ ] The four operations are served over HTTP against the generated interface.
- [ ] Organisation registration creates the tenant and the owning membership.
- [ ] A post-login destination is preserved without allowing an open redirect.

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
