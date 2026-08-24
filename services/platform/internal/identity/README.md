# internal/identity

## What this owns

Users, password credentials and sessions. Registration, authentication, session rotation and
revocation. Built rather than bought, per
[ADR-0003](../../../../docs/architecture/decisions/0003-identity-built-in-go.md), which makes password
handling and session revocation a standing obligation of this codebase rather than a vendor's problem.

## What this must never do

It never reveals whether an address exists. Registration answers identically for a new and an existing
address, login answers identically for a wrong password and an unknown user, and the unknown path
performs a dummy verification so the clock does not say what the body will not. A change that makes one
path faster or more informative than the other is a regression even if every test still passes.

It never lets re-registration take over an account. Registering an address that already exists does not
touch the stored password.

It never stores a token or a password in plaintext, and never puts either in an error, a log or a
`String` method.

## The session family, and why reuse is treated as theft

A family is one login. Each refresh appends a row and retires the previous one. Presenting a retired
refresh token means either a stolen token or a client bug, and we cannot tell which, so both revoke the
whole family. That deliberately logs out the legitimate client too.

The asymmetry is the reason: being logged out is a cheap failure, and an attacker keeping a foothold in
a system holding interview recordings is not.

A rotation carries the original `authenticated_at` forward. Refreshing is not proving who you are, so
it must never satisfy the step-up check in `platform/authz`.

## Identity is not tenant scoped

Neither table carries `tenant_id`. The same person practises privately and may screen for several
employers, and their practice history is never reachable from any employer authority. `tenancy.memberships`
is what connects a person to a tenant, and that table carries the tenant and its policy.
