-- 0012: the session lifecycle aggregate.
--
-- One table for both modes, because the lifecycle is one machine; the
-- authority model is where the modes part. A screening session belongs to a
-- tenant. A practice session belongs to the person alone, has no tenant at
-- all - enforced by CHECK, not convention - and is readable only in an
-- untenanted transaction acting as its owner, which is IAM-06's discipline
-- applied where practice and screening share a table.
--
-- Implements part of SES-01.

CREATE TABLE interview.sessions (
    id            uuid        PRIMARY KEY,
    mode          text        NOT NULL CHECK (mode IN ('practice', 'screening')),

    candidate_id  uuid        NOT NULL REFERENCES identity.users (id),

    -- The tenant, for screening. NULL for practice by CHECK rather than by
    -- discipline: a practice session that acquired a tenant would be exactly
    -- the linkage the separation forbids, so the shape refuses it.
    tenant_id     uuid        REFERENCES tenancy.tenants (id)
                              CHECK ((mode = 'practice') = (tenant_id IS NULL)),

    -- What the session will be composed against. The blueprint names the
    -- request; what actually runs is pinned by the bundle at readiness.
    blueprint_id  text        NOT NULL,

    state         text        NOT NULL DEFAULT 'draft'
                              CHECK (state IN (
                                  'draft', 'composing', 'ready', 'connecting', 'in_progress',
                                  'reconnecting', 'finalizing', 'evaluating', 'review_ready',
                                  'archived', 'cancelled', 'expired', 'composition_failed',
                                  'interrupted', 'finalization_failed', 'evaluation_failed')),

    -- Optimistic concurrency. Every transition names the version it read, and
    -- a stale write is refused rather than silently overwriting - SES-01's
    -- second criterion, enforced here rather than remembered in Go.
    version       integer     NOT NULL DEFAULT 1,

    -- The immutable bundle, set exactly once at readiness. The digest is what
    -- every later stage pins to; a session whose bundle changed after ready
    -- would make replay and review reconstruct a different interview.
    bundle_ref      text,
    bundle_digest   text,
    bundle_revision integer,

    -- The stable code of the failure that put the session in a *_failed
    -- state, for the operator who has to decide whether retry is worth it.
    failure_code  text,

    created_at       timestamptz NOT NULL DEFAULT now(),
    state_changed_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE interview.sessions IS
    'The session lifecycle aggregate. State changes only through the machine '
    'in internal/interview, version-guarded; a practice row has no tenant by '
    'CHECK and is invisible to any tenant-scoped transaction by policy.';

ALTER TABLE interview.sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE interview.sessions FORCE ROW LEVEL SECURITY;

-- Screening rows: the active tenant's, exactly like every tenant-owned table.
CREATE POLICY sessions_tenant ON interview.sessions
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

-- Practice rows: the owner, in a transaction carrying no tenant context at
-- all. The tenant-absence clause is IAM-06's tripwire expressed as policy:
-- a tenant-scoped code path cannot see practice sessions even when it also
-- knows who the candidate is.
CREATE POLICY sessions_practice_owner ON interview.sessions
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT, UPDATE ON interview.sessions TO prepeet_app;
