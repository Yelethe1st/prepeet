-- 0048: the workspace's settings, as versions rather than as a value.
--
-- TEN-01 asks for two things a mutable settings row cannot give. An audit
-- trail has to show what a value was before it changed, and a campaign has
-- to be able to keep the defaults it was created under after somebody edits
-- them. Both are the same requirement: the previous document must still be
-- readable, exactly, forever.
--
-- So a change is a new row. The highest version for a tenant is the current
-- configuration; every earlier one stands, unedited, and a campaign that
-- pinned version 3 can re-read version 3 whatever version the workspace is
-- on now. That is "defaults apply to new campaigns only" as a property of
-- the schema rather than as a rule somebody has to keep remembering.
--
-- tenancy.tenant_settings from 0001 is left alone. It is an untyped
-- key/value bag with no version and an in-place UPDATE; versioning it would
-- change what every existing reader sees, and a per-key version is the same
-- version with more places to get it wrong.
--
-- Implements part of TEN-01.

CREATE TABLE tenancy.tenant_configuration (
    tenant_id  uuid        NOT NULL REFERENCES tenancy.tenants (id) ON DELETE CASCADE,

    -- Monotonic per tenant, starting at 1. It is the concurrency guard as
    -- well as the pin: two administrators saving at once collide on the
    -- primary key, so the second is refused rather than silently winning.
    version    integer     NOT NULL CHECK (version > 0),

    -- The whole document. jsonb rather than a column per field because the
    -- shape is a product surface that will grow, and a settings screen
    -- gaining a checkbox should not be a migration on a table whose whole
    -- purpose is that old rows never change.
    settings   jsonb       NOT NULL,

    changed_by uuid        NOT NULL REFERENCES identity.users (id),
    changed_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (tenant_id, version)
);

COMMENT ON TABLE tenancy.tenant_configuration IS
    'One row per saved version of a workspace''s settings. Append-only: the '
    'highest version is current, and every earlier one is what a campaign '
    'created at that version was created under. See TEN-01.';

-- Append-only, twice over, because the two guards fail differently. The
-- REVOKE stops the application role, which is what the api and worker
-- connect as. The trigger stops the table's owner, which the REVOKE cannot,
-- and the migrator owns every table here.
REVOKE UPDATE, DELETE ON tenancy.tenant_configuration FROM prepeet_app;

CREATE FUNCTION tenancy.refuse_configuration_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'tenancy.tenant_configuration is append-only: a settings change is a new version (TEN-01)'
        USING ERRCODE = 'insufficient_privilege';
END
$$;

CREATE TRIGGER tenant_configuration_append_only
    BEFORE UPDATE OR DELETE ON tenancy.tenant_configuration
    FOR EACH ROW EXECUTE FUNCTION tenancy.refuse_configuration_change();

ALTER TABLE tenancy.tenant_configuration ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenancy.tenant_configuration FORCE ROW LEVEL SECURITY;

-- With no tenant set the comparison is against NULL, which is not true, so an
-- unscoped read returns nothing rather than every workspace's settings.
CREATE POLICY tenant_isolation ON tenancy.tenant_configuration
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON tenancy.tenant_configuration TO prepeet_app;
