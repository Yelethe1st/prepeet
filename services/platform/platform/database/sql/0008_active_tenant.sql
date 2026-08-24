-- The active tenant, and the record of choosing one.
--
-- docs/architecture/authorization-model.md requires every request to operate
-- under exactly one explicit active tenant, never inferred from a resource
-- identifier. This is where "explicit" is stored.

-- On the session rather than in a cookie or a header, which is the decision
-- this column embodies.
--
-- A client-supplied tenant would be a claim the server has to verify on every
-- request, and the request that forgets to verify is a cross-tenant read. Here
-- the value was verified once, when it was set, by a server that checked the
-- membership; from then on it is read from a row the client cannot reach. It
-- also means revoking a session revokes the tenant selection with it, and that
-- a membership revoked mid-session is caught on the next selection rather than
-- living on in a token nobody can recall.
ALTER TABLE identity.sessions
    ADD COLUMN active_tenant_id uuid REFERENCES tenancy.tenants (id) ON DELETE SET NULL;

COMMENT ON COLUMN identity.sessions.active_tenant_id IS
    'The one tenant this session acts under, or NULL for a candidate who '
    'belongs to none. Set only after the membership was verified. ON DELETE '
    'SET NULL rather than CASCADE: deleting a workspace must not sign its '
    'members out of the product.';

-- ─────────────────────────────────────────────────────── audit
-- The append-only record of who did what.
--
-- Created here because tenant selection is the first thing that has to be
-- audited, per IAM-03. It is deliberately general: an audit table per feature
-- is an audit trail nobody can read in one query, which is the only way it is
-- ever read.
CREATE TABLE audit.events (
    id           uuid        PRIMARY KEY,
    -- Nullable, because some auditable acts have no tenant: signing in,
    -- selecting a tenant for the first time, a platform support action. A
    -- NOT NULL column here would force a fake tenant onto exactly the events
    -- most worth reading.
    tenant_id    uuid        REFERENCES tenancy.tenants (id) ON DELETE SET NULL,
    actor_id     uuid        REFERENCES identity.users (id) ON DELETE SET NULL,
    actor_type   text        NOT NULL CHECK (actor_type IN ('user', 'service', 'platform')),
    action       text        NOT NULL,
    -- What was acted on, as a type and an identifier rather than a foreign key,
    -- because an audit row must outlive the thing it describes. A row that
    -- cascaded away with its subject would be missing exactly when somebody
    -- asks what happened to it.
    subject_type text,
    subject_id   text,
    outcome      text        NOT NULL CHECK (outcome IN ('allowed', 'denied', 'failed')),
    -- Identifiers and small control values only, never restricted content. The
    -- same rule as telemetry and workflow payloads: this table is read by
    -- support and exported to tenants, and neither is a place for transcript
    -- text. See docs/operations/telemetry-conventions.md.
    detail       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    -- The correlation identifier from the request, so an audit row and a trace
    -- describe the same moment.
    request_id   text,
    occurred_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_events_tenant_idx ON audit.events (tenant_id, occurred_at DESC);
CREATE INDEX audit_events_actor_idx ON audit.events (actor_id, occurred_at DESC);

COMMENT ON TABLE audit.events IS
    'Append-only record of auditable acts. No update or delete grant: an audit '
    'trail that can be edited is not one.';

-- Append only, enforced by the grant rather than by convention. The application
-- role can write and read and cannot change or remove anything.
REVOKE UPDATE, DELETE ON audit.events FROM prepeet_app;

ALTER TABLE audit.events ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit.events FORCE ROW LEVEL SECURITY;

-- Tenant-scoped like everything else, with one addition: a row with no tenant
-- is readable by the actor it belongs to. Those are the sign-in and
-- tenant-selection events, which belong to a person rather than to a workspace,
-- and would otherwise be readable by nobody.
CREATE POLICY tenant_isolation ON audit.events
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY own_untenanted_events ON audit.events
    FOR SELECT
    USING (tenant_id IS NULL
           AND actor_id = NULLIF(current_setting('app.user_id', true), '')::uuid);

-- Writing an untenanted event needs its own policy, since the tenant policy's
-- WITH CHECK cannot match a NULL tenant. Restricted to the acting user, so this
-- is not a way to write an event as somebody else.
CREATE POLICY own_untenanted_writes ON audit.events
    FOR INSERT
    WITH CHECK (tenant_id IS NULL
                AND actor_id = NULLIF(current_setting('app.user_id', true), '')::uuid);
