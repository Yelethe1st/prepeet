-- 0025: the full tenant role vocabulary.
--
-- 0006 deliberately shipped two values - owner and member - and named TEN-02
-- as the ticket that widens them. This is that widening, taken from the
-- prototype's own permission matrix: recruiter, hiring_manager, viewer and
-- admin, beside the owner anchored to whoever created the workspace. The
-- bundles behind each name live in the capability contract
-- (packages/contracts/authz), which is the artifact legal and security read;
-- this migration only teaches the schema the vocabulary.
--
-- 'member' becomes 'recruiter': its bundle always was the recruiting work,
-- and a rename is honest where inventing a parallel value would leave two
-- names for one authority.
--
-- Implements part of TEN-02.

ALTER TABLE tenancy.memberships
    DROP CONSTRAINT memberships_role_check;

-- The rename must see every row. The table is under FORCE row security with
-- a tenant-scoped policy, and a migration connection has no tenant context:
-- without lifting FORCE for the statement, a non-superuser migrator would
-- update nothing and report success. Lifted and restored in this same
-- transaction, so no other statement ever runs in the gap.
ALTER TABLE tenancy.memberships NO FORCE ROW LEVEL SECURITY;
UPDATE tenancy.memberships SET role = 'recruiter' WHERE role = 'member';
ALTER TABLE tenancy.memberships FORCE ROW LEVEL SECURITY;

ALTER TABLE tenancy.memberships
    ADD CONSTRAINT memberships_role_check
    CHECK (role IN ('owner', 'admin', 'recruiter', 'hiring_manager', 'viewer'));

-- The column default follows the rename, or an INSERT that relies on it
-- would violate the new CHECK it was written to satisfy.
ALTER TABLE tenancy.memberships ALTER COLUMN role SET DEFAULT 'recruiter';

COMMENT ON COLUMN tenancy.memberships.role IS
    'Scoped role within one tenant, from TEN-02''s vocabulary. The capability '
    'bundle behind each name is the authz contract''s. Never a global role: '
    'the same person may be an owner of one tenant and a viewer of another.';
