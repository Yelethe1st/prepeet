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
- [x] Switching tenant cannot expose a resource from the previous one, including through a cached read model.

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

**The cached read model was the last box, and it was leaking.** The note here used to say there was
nothing to leak through because nothing tenant-scoped was served. That stopped being true as the
screens landed, and nobody came back to it: every query key in the web application is scoped by what
it reads rather than by whose it is, so `["sessions"]`, `["profile"]` and `["documents"]` mean the
same thing in both workspaces, and TanStack Query answers from cache before it revalidates. The
first paint after a switch was the previous workspace's data.

Switching now removes every cached query except the session itself, which is what says who the
caller has become and is re-read on the next line. Removed by exclusion rather than by naming what
to drop: adding the tenant to every key, or listing the keys to clear, both work until somebody adds
a key and forgets, and the forgotten one is the leak. A test puts a key nobody has added yet into
the cache and asserts it is gone, which is the rule rather than an instance of it.

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
- [x] A user in more than one tenant can see and switch active tenant from the application shell.
- [x] The active tenant is visible at all times, not only inside the switcher.
- [x] Switching re-fetches rather than re-labels, and never renders stale tenant data.

**Done.** The switcher is in the shell's topbar rather than on a screen of its own, because
switching workspace is something somebody does while doing something else. Radix rather than a
native select, which buys the part a styled dropdown usually loses: focus trapped and restored,
typeahead, and the trigger wired to the listbox for assistive technology. The trigger carries the
active workspace's name at rest, so the second box is satisfied without opening anything.

The third box was the one with substance, and it is IAM-03's last criterion seen from the other
side. Switching re-reads the session, because switching changes what the session may do and the
navigation is built from that; and it removes every cached query except the session, because
re-fetching what the shell shows while leaving the previous workspace's reads in the cache is
re-labelling with extra steps. Both are asserted, the second by seeding a key nobody has added yet
and finding it gone.

One thing the browser suite caught that no unit test could: a workspace name is user supplied, and
the Radix trigger does not shrink where the native select it replaced did. At 320px with text at
200% the topbar measured 357px and the page scrolled sideways. The trigger is bounded and truncates.

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
- [x] The contract gains the provider endpoints, and the generated code and the handlers follow it.
- [x] `state` and PKCE are mandatory, single-use and time-bound, and a replayed or absent `state` is refused rather than tolerated.
- [x] An OAuth sign-in issues the same session the password flow issues, through the same entry point, with the same cookies, rotation and revocation.
- [x] Linking rules are explicit and proven: an OAuth identity whose verified email matches an existing account links to it; an unverified email from a provider never does.
- [x] A provider that is unreachable, slow, or returns an error leaves the person on a screen that names what happened and offers email and password, without a half-created account behind it.
- [x] Every provider is configured rather than compiled in, so adding one is configuration and a test.
- [x] The callback screen is ported from `screens/oauth-callback.html` with its four states: processing, slow, invalid state and expired code.
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

**In progress: the domain is in and attacked; the endpoints and the screen are not.**
Migration 0039 is two tables because there are two lifetimes: a state lives for minutes and dies on
first use, a link between a provider account and a person lives as long as the account. The state
is stored hashed, as action tokens are; the PKCE verifier is not, because it has to be replayed to
the token endpoint and a hash cannot be.

Single-use is the UPDATE's own `used_at IS NULL` rather than a read followed by a write, so two
callbacks arriving together cannot both win. The state is consumed *before* the provider is called,
which is why a replay costs nothing downstream: the test asserts the provider was called once for
one usable state. `ChallengeFor` is checked against RFC 7636's own worked vector rather than
against itself, because a challenge that looks right and is rejected at the token endpoint is the
mistake this primitive actually makes.

The linking rule is the ticket's whole risk and it is attacked rather than asserted. An account is
found by the provider's subject and never by email; a verified address links to the account that
owns it; an unverified address pointed at an existing account is refused with
`ErrOAuthEmailUnverified`. A state minted for one provider cannot complete another's callback,
which would otherwise make the weaker provider's redirect replayable at the stronger one.

One thing this uncovered in the existing code. An account created by a provider has no password
hash, and `Authenticate` answered an unparseable-hash error rather than the ordinary refusal: a
different shape of failure, and therefore an oracle telling an attacker which addresses are
provider-only, which is exactly the set worth attacking through a provider. Empty hashes now take
the dummy-verify path and answer `ErrCredentialsInvalid` like every other wrong password, asserted
by comparing the two messages.

One decision to review. Registration through a provider creates a candidate and only a candidate,
because an organisation registration creates a tenant and an owning membership and the last box
forbids doing that silently. That makes the eighth box only half true as written, and it is left
open rather than ticked: a recruiter signing up with Google presently has to register on the form
first, and whether that is right is a product call rather than an implementation one.

Three endpoints now carry it. `/auth/oauth/providers` is what the sign-in screen draws its buttons
from, so a deployment with none configured answers an empty list and shows email and password
alone. `/auth/oauth/{provider}/start` mints the state and answers where to send the browser;
`/auth/oauth/{provider}/callback` finishes it. The callback ends at the same `issued` that Login
and Refresh end at, which is the third box: one place writes the cookies, so a session held by
somebody who signed in with Google is indistinguishable from one held by somebody who typed a
password, including to logout and to revocation. Asserted by counting the cookies rather than by
reading the code.

Two defects were found while wiring it. The rate limiter was first keyed on the provider name,
which put every person signing in with Google into one bucket: one attacker starting sign-ins
would have locked everybody out of that provider, a lockout dressed as a rate limit. There is no
address at that point in the flow, so the counter is by network alone, and a test asserts the key
does not contain the provider. And the unverified-address refusal must not confirm that an account
exists, so its message says what would be true either way, with a test refusing the words that
would leak it.

`platform/oidc` is the provider half, and it is one client rather than a Google type and a
Microsoft type: the endpoints are configuration, which is what makes the sixth box true in
practice rather than in principle. Adding a third provider is a map entry and its credentials.

It does not verify an ID token, deliberately. The code is exchanged at the provider's own token
endpoint over TLS and the claims are then read from the provider's own userinfo endpoint with the
access token, so the answers come from the issuer directly rather than in a bearer artefact that
has to be validated. That trades one round trip for not shipping a JWT verifier, a JWKS cache and
a clock-skew policy, each of which is a way to be subtly wrong about who somebody is.

One line carries the linking rule and it fails closed: `email_verified` absent is not verified.
Microsoft omits it for personal accounts, where the address may be one the holder simply typed, so
reading absence as true would let anybody set an address there and link to somebody else's account
here. Both the absent and the explicitly-false cases are tested against a provider the suite
controls, along with the verifier actually reaching the token endpoint and never reaching the
authorization endpoint.

A provider missing its credentials is left out of the map rather than added broken, so the
sign-in screen never draws a button that fails at the token endpoint. The endpoints carry
defaults because knowing them is not the deployer's job; the credentials carry none, because a
default credential is one somebody forgot to set.

The screen is at `/auth/callback/[provider]`, with the provider in the path because it is part of
the redirect URI registered with each provider and a registered URI that varies by query string is
one more thing to get wrong in two places. The sign-in screen draws its buttons from
`/auth/oauth/providers`, so a deployment with none configured renders nothing at all rather than an
empty heading or a divider with nothing above it.

One recorded deviation, and it is the whole design of the screen. The prototype shows three stages
with per-stage timings ticking over as they complete. There is one request here and no way to know
which part of it the server is in, so rendering three stages that advance on a timer would be an
animation pretending to be telemetry, on the screen where somebody is waiting to find out whether
they are signed in. It shows what is true: in progress, and after six seconds, that it is taking
longer than usual.

The failure states are one state rather than the prototype's two, for the same reason the server
answers one refusal for a forged state and a replayed one: they cannot be told apart from outside,
so the screen does not claim to. It names what the server said, carries the reference to quote,
lists the three things that usually cause it, and offers email and password. A provider that
declines redirects with `error` and no code, which is shown as a failure rather than left to become
"no code", and a truncated callback link never reaches the server at all.

`/auth/callback/google` is in the accessibility and layout sweeps, which caught the one defect
here: the muted foreground on the danger surface measures 4.48:1 in the light theme, under the 4.5
it needs, and looking at it would not have found that.

Only the eighth box is left, and it is the open product question rather than work.

**Spec** [authorization-model.md](../../architecture/authorization-model.md) · [ADR-0003](../../architecture/decisions/0003-identity-built-in-go.md)
