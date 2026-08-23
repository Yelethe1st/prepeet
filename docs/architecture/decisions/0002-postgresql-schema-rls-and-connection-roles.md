# ADR-0002: PostgreSQL schema layout, row-level security and connection roles

**Status:** Accepted  
**Owner:** olabode omoyele  
**Decision date:** 2026-08-24  
**Review date:** 2027-02-24  
**Supersedes:** None  
**Superseded by:** None

Implements [DEC-05](../../delivery/tickets/01-decisions-and-adrs.md).

## Context

Cross-tenant leakage is a stop-ship condition in
[release-criteria.md](../../delivery/release-criteria.md), and a leak between a candidate's private
practice history and an employer's screening authority is worse than an ordinary data leak: it would
mean an employer seeing rehearsals the candidate believed were private.

Application authorization is the first line of defence and is the one that decides. This ADR is about
the second line: what happens when a handler forgets a `WHERE tenant_id = ?`, which is a mistake every
team makes eventually.

The choice is between separating tenants physically, by database or by schema, and separating them
logically inside shared tables with row-level security enforcing the boundary.

## Decision

**One database, one shared schema per module, with row-level security on every tenant-owned table.**

Tables live in schemas named for the module that owns them: `identity`, `tenancy`, `candidate`,
`content`, `interview`, `media`, `evaluation`, `recruiting`, `progression`, `billing`, `integration`,
`audit`. Schema is an ownership boundary, not a tenant boundary. A module reads and writes only its own
schema, enforced by [PLT-04](../../delivery/tickets/02-platform-foundation.md).

**Every tenant-owned table carries `tenant_id` and is created with its RLS policy in the same
migration that creates the table.** A table without a policy is a table that leaks, and adding the
policy later means shipping a window in which it leaks.

**Tenant context is set per transaction, never per connection.** The application sets
`app.tenant_id` with `SET LOCAL` inside the transaction, so the value cannot outlive it and cannot be
inherited by the next request that borrows the same pooled connection. A pooled connection carrying a
previous request's tenant is precisely the bug RLS is meant to catch, so the mechanism must not create
it.

**Policies deny when no tenant is set**, and the expression has to be written carefully to achieve it:

```sql
USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
```

The `NULLIF` is not decoration. A setting that was never set reads as NULL, so the bare call looks
sufficient, and it is until the first tenant-scoped transaction on that connection ends: `SET LOCAL`
then reverts the setting to the empty string rather than to NULL, and `''::uuid` raises. Without
`NULLIF` the next unscoped query on a pooled connection fails with a cast error instead of quietly
returning no rows. This was found by the isolation tests rather than by reading, and it is recorded
here because someone writing the next policy from this document would otherwise reintroduce it.

With `NULLIF`, an unset context makes the comparison NULL, which is not true, so an unscoped query
returns nothing rather than everything. Forgetting to set the context fails closed.

**Three roles, none of which can bypass RLS.**

| Role | Used by | Rights |
|---|---|---|
| `prepeet_migrator` | `cmd/migrate` only | Owns the schemas, applies DDL. Not used at runtime |
| `prepeet_app` | api and worker | DML on module tables. `NOSUPERUSER`, `NOBYPASSRLS` |
| `prepeet_readonly` | Analytics and support reads | `SELECT` only, still subject to RLS |

Table owners bypass their own policies in PostgreSQL unless told otherwise, so every tenant-owned table
is created with `FORCE ROW LEVEL SECURITY`. Without it the migrator's ownership would silently defeat
the whole mechanism.

**Migrations are forward only, numbered, applied in a transaction, and checksummed.** An already
applied migration that changes on disk is refused rather than reapplied, because two environments
running different SQL under the same version number is a difference nobody will notice until it
matters.

**Practice and screening separation is not left to `tenant_id` alone.** Practice data is candidate
owned and has no tenant, so it is not reachable through a tenant policy at all. That separation is
structural rather than conditional, and [IAM-06](../../delivery/tickets/04-identity-and-authorization.md)
tests it adversarially.

## Alternatives considered

**A database per tenant.** The strongest isolation, and the reason it is rejected is operational rather
than theoretical: migrations, connection pooling, backup and restore, and cross-tenant platform
analytics all multiply by tenant count. It also cannot express a candidate who screens for several
employers, which is an ordinary case here rather than an edge one. It remains the answer for a single
buyer demanding physical isolation, priced separately.

**A schema per tenant.** Lighter than a database per tenant and still multiplies migration work by
tenant count, while giving up query-time enforcement: nothing stops a handler selecting from the wrong
schema. It trades a mechanical guarantee for a naming convention.

**Application-only scoping, with no RLS.** Simpler and faster, and it relies on every developer
remembering every time. The failure mode is silent and the blast radius is a candidate's recording in
an employer's hands.

**Session-level `SET` rather than `SET LOCAL`.** Marginally fewer statements per request, and it leaks
tenant context across pooled connections. Rejected outright.

## Consequences

Positive. A forgotten `WHERE tenant_id = ?` returns nothing instead of another tenant's rows.
Cross-tenant isolation becomes testable as a property of the database rather than as a claim about
code review, which is what [SEC-02](../../delivery/tickets/19-security-and-privacy.md) needs.

Negative. Every query pays a policy evaluation, and `tenant_id` has to lead the composite indexes or
the planner will do the wrong thing at volume. Every transaction must set the context, so a code path
that opens a transaction without it fails in an unhelpful way until the failure is made explicit.
Platform-wide analytics need a deliberate, audited path rather than an ambient one.

Security. This is defence in depth, not the primary control. Authorization still decides; RLS limits
the damage when it is bypassed by mistake. It does nothing against a compromised application role that
sets the context to any tenant it likes, which is why elevation is separately audited under
[IAM-07](../../delivery/tickets/04-identity-and-authorization.md).

Cost. One cluster rather than one per tenant, which is the cheaper end of the options.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| A new table ships without RLS | The migration test suite fails on any tenant-owned table lacking a forced policy, so it cannot merge |
| Tenant context leaks between pooled requests | `SET LOCAL` only, verified by a test that reuses a connection across two tenants |
| RLS is defeated by an owner or superuser connection | `FORCE ROW LEVEL SECURITY` everywhere, and the app role is created `NOSUPERUSER NOBYPASSRLS` |
| Policy evaluation degrades at volume | `tenant_id` leads every composite index; query plans are reviewed before the practice release gate |
| An applied migration is edited | Checksums are recorded and verified on every run |

## Reversibility and migration

Cheap to strengthen, expensive to weaken. Moving to a database per tenant later means exporting per
tenant and repointing connections, which is real work but bounded and can be done for one tenant at a
time. Removing RLS would be trivial and is a decision nobody should make.

## Validation

- A test proves cross-tenant `SELECT`, `INSERT`, `UPDATE`, `DELETE` and listing all fail under the app role.
- A test proves a query with no tenant context returns nothing rather than everything.
- A test proves the app role is neither superuser nor `BYPASSRLS`.
- A test proves tenant context does not survive across transactions on a reused connection.
- A test fails the build if any tenant-owned table lacks forced row-level security.
- Migrations apply from empty and from the previous release, and an edited applied migration is refused.
