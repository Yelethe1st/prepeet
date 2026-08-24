-- A person may always learn which workspaces they belong to.
--
-- The policy added in 0001 scopes memberships to one tenant, which is right for
-- every question asked from inside a workspace. It cannot answer the one asked
-- before a workspace has been chosen: "which tenants am I a member of", which is
-- what GET /me reports and what the tenant switcher is built on.
--
-- The tempting fixes are both wrong. Reading it as the migrator would put a
-- BYPASSRLS path into the request path, which is the thing ADR-0002 exists to
-- prevent. Scoping by user_id in the query alone would move the isolation from
-- the database into a WHERE clause somebody can forget, and the whole argument
-- for forced row-level security is that a forgotten WHERE clause is a
-- cross-tenant leak rather than a missing filter.
--
-- So it is a second policy. Permissive policies for the same command are
-- combined with OR, so a membership is readable when the transaction is scoped
-- to its tenant, as before, or when it belongs to the person the transaction is
-- acting as.
--
-- SELECT only, deliberately. Reading your own memberships is always allowed;
-- creating, changing or revoking one is a tenant administration decision that
-- TEN-02 owns, and this must not become a way to grant yourself access.
CREATE POLICY own_memberships_readable ON tenancy.memberships
    FOR SELECT
    USING (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid);

COMMENT ON POLICY own_memberships_readable ON tenancy.memberships IS
    'Lets a person read their own memberships without a tenant scope, which is '
    'the question asked before a tenant has been chosen. SELECT only: this is '
    'not a way to grant yourself membership.';

-- Naming a workspace requires reading it.
--
-- The policy above makes a person's memberships visible, and the first thing
-- anybody does with a membership is join to the tenant for its name. The policy
-- on tenants from 0001 scopes by app.tenant_id, so that join found nothing and
-- the membership disappeared with it. The two policies have to compose or
-- neither is usable before a tenant is chosen.
--
-- The subquery reads memberships, which is itself under the policy above. That
-- is intended rather than accidental: a tenant is readable exactly when the
-- actor holds a membership the database will already show them. There is no
-- recursion, because the memberships policy does not reference tenants.
--
-- SELECT only, for the same reason: this says which workspaces you may see, not
-- what you may do in one. Nothing here creates or changes a tenant.
CREATE POLICY own_tenants_readable ON tenancy.tenants
    FOR SELECT
    USING (EXISTS (
        SELECT 1
        FROM tenancy.memberships m
        WHERE m.tenant_id = tenants.id
          AND m.user_id = NULLIF(current_setting('app.user_id', true), '')::uuid
          AND m.status <> 'revoked'
    ));

COMMENT ON POLICY own_tenants_readable ON tenancy.tenants IS
    'Lets a person read the workspaces they are a member of without a tenant '
    'scope, which is what makes the tenant switcher and GET /me possible.';
