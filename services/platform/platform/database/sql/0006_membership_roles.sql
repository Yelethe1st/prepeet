-- Membership roles.
--
-- IAM-01 needs to record who created a workspace, because that person is the
-- one who can administer it and a tenant nobody can administer is a support
-- ticket that cannot be resolved from inside the product.
--
-- Deliberately two values. The full role vocabulary and the permission matrix
-- behind it belong to TEN-02, which is a decision about what a recruiter may do
-- that this migration has no business pre-empting. Inventing 'admin' and
-- 'viewer' here would mean either TEN-02 inherits names chosen without the
-- analysis, or a migration to undo them.
--
-- 'owner' means the membership that created the tenant. 'member' means every
-- other membership. TEN-02 widens the check.
ALTER TABLE tenancy.memberships
    ADD COLUMN role text NOT NULL DEFAULT 'member'
        CHECK (role IN ('owner', 'member'));

COMMENT ON COLUMN tenancy.memberships.role IS
    'Scoped role within one tenant. Two values until TEN-02 defines the full '
    'matrix. Never a global role: the same person may be an owner of one '
    'tenant and a member of another, and nothing about a user is a role.';

-- Finding the owner of a tenant is what an operator does when a workspace needs
-- administering and nobody remembers who set it up.
CREATE INDEX memberships_owner_idx ON tenancy.memberships (tenant_id)
    WHERE role = 'owner';

-- No constraint that a tenant has exactly one owner, and that absence is
-- deliberate rather than an omission. "At least one" is not expressible without
-- a trigger, and "at most one" would be wrong: an organisation with a single
-- owner who leaves is exactly the situation a second owner exists to prevent.
