-- 0026: the usage ledger and quotas, to ADR-0014.
--
-- The billing unit is a started session, metered exactly once. The ledger
-- is append-only: an entry is never updated or deleted, and the corrections
-- ADR-0014 defines - the sixty-second early abandon, an interruption that
-- was our fault - are credit entries beside the start, never edits of it.
-- An invoice is a sum over immutable rows, which is the only kind of number
-- finance and an audit can re-derive a year later.
--
-- Quota is enforced by reservation at start: the reserve transaction locks
-- the tenant's quota row, counts, and either inserts the start entry or
-- refuses. Nothing consults quota after a session has started, which is how
-- "a candidate is never interrupted mid-interview by a quota event" is a
-- structure rather than a promise.
--
-- Implements part of TEN-08.

CREATE SCHEMA IF NOT EXISTS billing;
GRANT USAGE ON SCHEMA billing TO prepeet_app;

CREATE TABLE billing.usage_entries (
    id          uuid        PRIMARY KEY,
    tenant_id   uuid        NOT NULL REFERENCES tenancy.tenants (id),
    session_id  uuid        NOT NULL,

    -- What happened: a billable start, or a credit against one.
    kind        text        NOT NULL CHECK (kind IN ('session_started', 'start_credited')),

    -- Why a credit exists. Empty for starts; a credit names its cause so
    -- the ledger explains the invoice without a person reconstructing it.
    reason      text        NOT NULL DEFAULT ''
                            CHECK ((kind = 'session_started') = (reason = '')),

    mode        text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    occurred_at timestamptz NOT NULL DEFAULT now(),

    -- Exactly-once metering: one start and at most one credit per session.
    UNIQUE (session_id, kind)
);

COMMENT ON TABLE billing.usage_entries IS
    'Append-only usage ledger, ADR-0014. A started session is one entry; '
    'corrections are credit entries, never edits. The invoice is a sum.';

CREATE INDEX usage_entries_tenant_idx ON billing.usage_entries (tenant_id, occurred_at DESC);

-- Append-only, structurally.
CREATE OR REPLACE FUNCTION billing.refuse_ledger_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'billing.usage_entries is append-only: corrections are credit entries, not edits';
END $$;

CREATE TRIGGER usage_entries_append_only
    BEFORE UPDATE OR DELETE ON billing.usage_entries
    FOR EACH ROW EXECUTE FUNCTION billing.refuse_ledger_change();

ALTER TABLE billing.usage_entries ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing.usage_entries FORCE ROW LEVEL SECURITY;

CREATE POLICY usage_tenant_isolation ON billing.usage_entries
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON billing.usage_entries TO prepeet_app;

CREATE TABLE billing.quotas (
    tenant_id      uuid        PRIMARY KEY REFERENCES tenancy.tenants (id),

    -- NULL means unlimited: absence of a limit is stated, not inferred
    -- from a missing row, so "no row" and "no limit" stay distinguishable.
    session_limit  integer     CHECK (session_limit IS NULL OR session_limit >= 0),

    -- Where the soft warning begins, as a fraction of the limit.
    warn_threshold numeric(3,2) NOT NULL DEFAULT 0.80
                               CHECK (warn_threshold > 0 AND warn_threshold < 1),

    updated_at     timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE billing.quotas IS
    'Per-tenant session quota, ADR-0014. The reserve transaction locks this '
    'row, which is what serialises concurrent starts at the boundary.';

ALTER TABLE billing.quotas ENABLE ROW LEVEL SECURITY;
ALTER TABLE billing.quotas FORCE ROW LEVEL SECURITY;

CREATE POLICY quotas_tenant_isolation ON billing.quotas
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE ON billing.quotas TO prepeet_app;
