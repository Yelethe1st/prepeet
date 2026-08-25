-- 0017: candidate documents - the CV, versioned.
--
-- Every upload is a new version; nothing is ever rewritten. The row is the
-- authoritative record - key, type, bytes, digest, upload state - per
-- data-architecture.md's rule that bucket listing is never an application
-- query, and the digest recorded here is what extraction and composition pin,
-- so a deleted or replaced CV can never silently rewrite a session composed
-- from an earlier version: the earlier version's record stands.
--
-- Candidate schema, so IAM-06's structural guards - forced owner policy with
-- the tenant-absence clause, no tenant column, the write tripwire - apply by
-- existing.
--
-- Implements part of PRO-02.

CREATE TABLE candidate.documents (
    id          uuid        PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES identity.users (id),

    -- What kind of document. One kind today; the CHECK grows with PRO tickets.
    kind        text        NOT NULL CHECK (kind IN ('cv')),

    -- Monotonic per person and kind. Replacement is version n+1 existing, not
    -- version n changing.
    version     integer     NOT NULL CHECK (version >= 1),

    storage_key text        NOT NULL,
    media_type  text        NOT NULL,
    size_bytes  bigint      NOT NULL CHECK (size_bytes > 0),

    -- The upload's lifecycle. PRO-02 requires failed and partial uploads to
    -- have their own recoverable states rather than one collapsed "broken":
    -- uploading can be completed or aborted, failed can be retried by a new
    -- version, and neither is mistaken for stored.
    state       text        NOT NULL DEFAULT 'uploading'
                            CHECK (state IN ('uploading', 'stored', 'failed', 'deleted')),

    -- The multipart upload in flight, for completing or aborting it.
    upload_id   text,

    -- The content digest, recorded at completion. Empty until stored.
    sha256      text,

    created_at  timestamptz NOT NULL DEFAULT now(),
    stored_at   timestamptz,
    deleted_at  timestamptz,

    UNIQUE (user_id, kind, version)
);

COMMENT ON TABLE candidate.documents IS
    'Candidate-owned documents, versioned. The row is the authoritative '
    'record; rows are never destroyed, so the digest a bundle pinned is '
    'always answerable even after the object itself is deleted.';

ALTER TABLE candidate.documents ENABLE ROW LEVEL SECURITY;
ALTER TABLE candidate.documents FORCE ROW LEVEL SECURITY;

CREATE POLICY documents_owner ON candidate.documents
    USING (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

CREATE TRIGGER documents_no_tenant_context
    BEFORE INSERT OR UPDATE OR DELETE ON candidate.documents
    FOR EACH ROW EXECUTE FUNCTION candidate.refuse_tenant_context();

GRANT SELECT, INSERT, UPDATE ON candidate.documents TO prepeet_app;
