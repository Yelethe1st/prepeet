-- 0003: a user can read their own membership rows across tenants.
--
-- Found while implementing GET /me. The policy in 0001 scopes memberships to
-- the active tenant, which is right for every tenant-facing query and makes one
-- necessary read impossible: listing which tenants a person belongs to. That
-- list is what the tenant switcher in IAM-05 renders, and it cannot be answered
-- from inside a single tenant's scope.
--
-- PostgreSQL combines permissive policies with OR, so this adds a second way to
-- see a row without widening the first. A row is visible when it belongs to the
-- active tenant, or when it belongs to the person asking.
--
-- app.user_id is set alongside app.tenant_id, with SET LOCAL, for the same
-- reason: it must not survive the transaction and be inherited by whoever
-- borrows the connection next.

CREATE POLICY membership_self_read ON tenancy.memberships
    FOR SELECT
    USING (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid);

COMMENT ON POLICY membership_self_read ON tenancy.memberships IS
    'Lets a person see which tenants they belong to. Read only: a user may '
    'discover their own memberships but never create, change or revoke one, '
    'which stays with tenant.member_manage.';
