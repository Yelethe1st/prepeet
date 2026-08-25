-- 0013: the artifact registry.
--
-- Personas, plans, rule packs, rubrics, role standards, prompts and policies,
-- as immutable published versions with a pointer naming the current one.
-- ADR-0011 decides the shape; the two properties enforced here rather than
-- remembered are the ones reproducibility hangs on:
--
--   1. A published row's content cannot change, by trigger. A rubric edited
--      after publication changes what a candidate was judged by after the
--      fact, which is the failure the registry exists to make impossible.
--   2. Sessions pin digests, so the pointer moving - publication, rollback -
--      affects only future compositions. The pointer is the only mutable
--      thing here, and its moves are audited.
--
-- Implements part of CAT-01.

CREATE TABLE content.artifacts (
    id             uuid        PRIMARY KEY,

    artifact_type  text        NOT NULL CHECK (artifact_type IN (
                                   'persona', 'plan', 'rule_pack', 'rubric', 'role_standard',
                                   'prompt', 'model_policy', 'articulation_policy')),

    -- The stable name compositions resolve, such as persona/friendly-structured.
    reference      text        NOT NULL,

    -- Semantic version of the content; schema_version is the version of the
    -- shape the body conforms to. Both pin into bundles.
    version        text        NOT NULL,
    schema_version text        NOT NULL,

    -- SHA-256 of the canonical body, computed and verified in Go. The digest
    -- is the identity sessions pin; everything else is addressing.
    digest         text        NOT NULL,
    body           jsonb       NOT NULL,

    status         text        NOT NULL DEFAULT 'draft'
                               CHECK (status IN ('draft', 'validating', 'approved',
                                                 'published', 'deprecated', 'retired')),

    -- NULL for platform artifacts, which every tenant's compositions read.
    -- Tenant-authored artifacts (TEN-04) are tenant-scoped by policy.
    tenant_id      uuid        REFERENCES tenancy.tenants (id),

    -- Provenance. The publisher must differ from the drafter - ADR-0011's
    -- separation of duties - which the store enforces because a CHECK cannot
    -- compare against the future.
    created_by     uuid        NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    published_by   uuid,
    published_at   timestamptz,

    -- One version of one reference, per catalogue. NULLS NOT DISTINCT so two
    -- platform artifacts cannot share a reference and version by both having
    -- no tenant.
    UNIQUE NULLS NOT DISTINCT (tenant_id, reference, version)
);

COMMENT ON TABLE content.artifacts IS
    'Versioned, digest-addressed interview artifacts. Published rows are '
    'immutable by trigger; sessions pin digests; the pointer table names what '
    'is current. See ADR-0011.';

-- The immutability tripwire. Identity and content are frozen at publication;
-- status may still move, because deprecation and retirement are the lifecycle
-- continuing, not the content changing.
CREATE FUNCTION content.refuse_published_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status IN ('published', 'deprecated', 'retired') AND (
        NEW.body IS DISTINCT FROM OLD.body
        OR NEW.digest IS DISTINCT FROM OLD.digest
        OR NEW.version IS DISTINCT FROM OLD.version
        OR NEW.schema_version IS DISTINCT FROM OLD.schema_version
        OR NEW.reference IS DISTINCT FROM OLD.reference
        OR NEW.artifact_type IS DISTINCT FROM OLD.artifact_type
        OR NEW.published_by IS DISTINCT FROM OLD.published_by
        OR NEW.published_at IS DISTINCT FROM OLD.published_at
    ) THEN
        RAISE EXCEPTION 'published artifacts are immutable: a change is a new version (ADR-0011)'
            USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN NEW;
END
$$;

CREATE TRIGGER artifacts_published_immutable
    BEFORE UPDATE ON content.artifacts
    FOR EACH ROW EXECUTE FUNCTION content.refuse_published_mutation();

-- Published rows never leave, either: history is the audit answer.
CREATE FUNCTION content.refuse_published_delete() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.status <> 'draft' THEN
        RAISE EXCEPTION 'only drafts may be deleted: everything after validation is history (ADR-0011)'
            USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN OLD;
END
$$;

CREATE TRIGGER artifacts_history_kept
    BEFORE DELETE ON content.artifacts
    FOR EACH ROW EXECUTE FUNCTION content.refuse_published_delete();

-- What compositions should use next. The only mutable surface, and rollback
-- is nothing but this row moving backwards.
CREATE TABLE content.artifact_pointers (
    reference   text        NOT NULL,
    tenant_id   uuid        REFERENCES tenancy.tenants (id),
    artifact_id uuid        NOT NULL REFERENCES content.artifacts (id),
    updated_by  uuid        NOT NULL,
    updated_at  timestamptz NOT NULL DEFAULT now(),

    UNIQUE NULLS NOT DISTINCT (tenant_id, reference)
);

COMMENT ON TABLE content.artifact_pointers IS
    'Which published version a reference currently resolves to. Moves on '
    'publication and rollback, audited each time; never read by anything that '
    'already holds a digest.';

-- Reads: the platform catalogue is everyone's; a tenant's artifacts are its
-- own. Writes are gated by capability at the service layer - the policy's one
-- job here is that no tenant reads or writes another's.
ALTER TABLE content.artifacts ENABLE ROW LEVEL SECURITY;
ALTER TABLE content.artifacts FORCE ROW LEVEL SECURITY;
CREATE POLICY artifacts_visibility ON content.artifacts
    USING (tenant_id IS NULL
           OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id IS NULL
           OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

ALTER TABLE content.artifact_pointers ENABLE ROW LEVEL SECURITY;
ALTER TABLE content.artifact_pointers FORCE ROW LEVEL SECURITY;
CREATE POLICY pointers_visibility ON content.artifact_pointers
    USING (tenant_id IS NULL
           OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id IS NULL
           OR tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON content.artifacts TO prepeet_app;
GRANT SELECT, INSERT, UPDATE ON content.artifact_pointers TO prepeet_app;
