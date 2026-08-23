# ADR-0003: Authentication is built in Go

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-24  
**Review date:** 2027-02-24  
**Supersedes:** None  
**Superseded by:** None

Implements [DEC-02](../../delivery/tickets/01-decisions-and-adrs.md).

## Context

Three parts of identity were never in question, because no vendor models them the way this product
needs. Tenancy and membership are ours, under [ADR-0002](0002-postgresql-schema-rls-and-connection-roles.md).
Authorization is ours, in `platform/authz`. Invitation resolution is ours, because it carries an unusual
requirement: a screening candidate arrives holding a token, may or may not already have an account, and
resolving that must not reveal whether their email exists.

What was open is authentication: registration, password login, logout, session refresh, email
verification, recovery, magic link, OTP, configured OAuth, and step-up for sensitive administration.

Two populations pull in opposite directions. Candidates are self-serve, high volume, low value each,
and many will never pay, because practice mode is free to try by design. Most identity vendors price
per monthly active user, so the bill would scale with exactly the population that generates no revenue.
Tenant members are few, valuable, and will eventually demand SSO and SCIM, which is the population
per-seat pricing suits and where a vendor genuinely saves months.

[ADR-0001](0001-hosting-platform-and-regional-topology.md) commits to storing candidate content in
`eu-west-2`. An identity vendor holding candidate emails and authentication events becomes a named
sub-processor with its own region, which narrows the field further.

## Decision

**Authentication is built in Go**, in `services/platform/internal/identity`. Enterprise federation is
deferred rather than designed out: when a buyer requires SSO or SCIM, it is adopted for tenant members
only, behind an adapter, and priced against that buyer.

The scope is bounded by work already done. The prototype in `/screens` covers all ten authentication
routes and, more usefully, their states: verifying, verified, already verified and failed for email
verification; signing in, success, already used and wrong device for magic links; auto-advance, paste,
resend countdown and recovery code for OTP. The expensive and easily underestimated part of building
authentication is deciding what every failure looks like to a person, and that work exists. What
remains is implementing it.

The decisions that follow from building it:

**Passwords use argon2id**, with parameters named in one place and a recorded version, so raising them
later is a migration rather than an archaeology exercise. A password verified against outdated
parameters is transparently rehashed on next successful login.

**Sessions are opaque random tokens, stored hashed, not JWTs.** This is the least obvious choice here
and the one with the clearest reason. Membership revocation must invalidate active sessions promptly:
a recruiter removed from a tenant should stop being able to read candidate evidence within seconds,
not when a token happens to expire. A stateless token cannot be revoked without building the server
side lookup that stateless tokens exist to avoid, so the lookup is built directly. Tokens are hashed at
rest with SHA-256, so a database read does not yield usable credentials.

**Refresh tokens rotate, and reuse is treated as theft.** Each refresh issues a new token and retires
the old one. Presenting a retired token means either a stolen token or a client bug, and both are
handled the same way: the entire session family is revoked and the event is audited. Being logged out
is a cheap failure; an attacker keeping a foothold is not.

**Nothing reveals whether an account exists.** Registration, login, recovery and invitation acceptance
all answer identically for a known and an unknown address, and login performs a dummy hash verification
when the user is absent so that response timing does not leak what the response body will not.

**Step-up is authentication age, not a second factor.** `platform/authz` already refuses destructive and
evaluation-changing capabilities unless the subject authenticated within fifteen minutes. The session
records when authentication last happened, distinct from when the session was issued.

## Alternatives considered

**A hosted vendor such as Auth0, Clerk, WorkOS or Stytch.** Fastest to a working login, best attack
protection, and the strongest enterprise SSO story. Rejected on cost shape and residency: per active
user pricing scales with free candidates, and the vendor's region becomes a sub-processing question
against a residency commitment already made to tenants. This is the alternative most likely to be
revisited, and the trigger is the team being too small to carry password security well.

**Self-hosted identity such as Keycloak, Ory or Zitadel.** Looks like the compromise and mostly is not:
it removes per-user pricing while adding the operational burden of running identity infrastructure,
which is most of the cost of building without the control of having built.

**Split from the start: build for candidates, buy for tenant members.** The right end state, and
premature now. It means two authentication paths before there is a single enterprise buyer asking for
one, so the split is deferred until the buyer exists.

**JWT sessions.** Fewer database reads per request. Rejected because revocation is a requirement rather
than a nicety here, and the workarounds, short expiry with a denylist, reconstruct the state that JWTs
were chosen to avoid.

## Consequences

Positive. Per-user cost is zero for the population that will dominate. Candidate emails and
authentication events stay in `eu-west-2` with no additional sub-processor. The invitation resolution
flow, which has a real privacy requirement, is ours to get right. Session revocation is immediate
because sessions are state we hold.

Negative, and this is the real cost: **we own password security forever.** Breached password lists,
credential stuffing defence, bot signups, rate limiting, and the day a CVE lands in a dependency are
all ours now. A vendor does that work continuously and does it well. This obligation should be visible
in the security review cadence in [threat-model.md](../../security/threat-model.md) rather than assumed.

Operational. Session lookup adds a database read per authenticated request, which is a cache candidate
later and should not be one before it is measured.

Organisational. Enterprise SSO becomes a project rather than a configuration change when the first
buyer asks. That is a known, dated cost rather than a surprise.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Password handling is got wrong | argon2id with reviewed parameters, no bespoke cryptography, and the isolation and enumeration properties covered by tests rather than by intent |
| Credential stuffing against candidate accounts | Rate limiting per address and per network, and a breached-password check before the practice release gate |
| Account enumeration through timing | A dummy verification runs when the user is absent, and the timing property is asserted by test |
| A stolen refresh token is used quietly | Rotation with reuse detection revokes the whole session family and audits the event |
| Enterprise SSO is needed sooner than expected | The identity module keeps a single authentication entry point so a federation adapter attaches at one boundary |
| Session lookup becomes a hot path | Measured before it is cached, and the cache is not allowed to outlive a revocation |

## Reversibility and migration

Moderately expensive to reverse. Moving to a vendor later means migrating user records and forcing a
password reset for every candidate, because password hashes are not portable between systems in
practice. Sessions would be invalidated once. The membership, capability and invitation models are
unaffected, since none of them were ever the vendor's to hold, which is what keeps this bounded.

## Validation

- Registration, login, recovery and invitation acceptance answer identically for known and unknown addresses.
- Login timing does not distinguish an unknown user from a wrong password.
- A retired refresh token revokes the whole session family rather than merely failing.
- Revoking a membership stops evidence access within one session lookup, proven by test.
- Password parameters are recorded per hash, and a hash under old parameters is upgraded on next login.
- No password, token or hash appears in any log, trace or error, enforced by the scanner in [SEC-08](../../delivery/tickets/19-security-and-privacy.md).
