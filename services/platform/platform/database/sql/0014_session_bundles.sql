-- 0014: the composed bundle, persisted beside its session.
--
-- The bundle document records every pinned artifact - type, reference,
-- version, schema version, digest - and the composition's provenance. Go
-- persists it because Go owns trusted product state; Python composing it and
-- also storing it would make Python an owner. Written once at readiness and
-- immutable from that moment, because invariant 5 says session configuration
-- cannot change after start and the bundle IS the configuration.
--
-- Implements part of CAT-02.

CREATE TABLE interview.session_bundles (
    session_id uuid        PRIMARY KEY REFERENCES interview.sessions (id),
    digest     text        NOT NULL,
    body       jsonb       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE interview.session_bundles IS
    'The immutable composed bundle for one session: every pinned artifact '
    'version and digest, written once at readiness. Review, replay and audit '
    'read the session''s configuration from here.';

-- Immutable and permanent, by trigger: an edited bundle rewrites what a
-- session ran against after the fact, and a deleted one leaves a review with
-- nothing to reconstruct from.
CREATE FUNCTION interview.refuse_bundle_change() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'session bundles are immutable and permanent (CAT-02): recompose before start creates a new revision'
        USING ERRCODE = 'insufficient_privilege';
END
$$;

CREATE TRIGGER session_bundles_frozen
    BEFORE UPDATE OR DELETE ON interview.session_bundles
    FOR EACH ROW EXECUTE FUNCTION interview.refuse_bundle_change();

-- The session's own authority, inherited: visible exactly where the session
-- row is visible, which the policies express by delegating to it.
ALTER TABLE interview.session_bundles ENABLE ROW LEVEL SECURITY;
ALTER TABLE interview.session_bundles FORCE ROW LEVEL SECURITY;
CREATE POLICY bundles_via_session ON interview.session_bundles
    USING (EXISTS (SELECT 1 FROM interview.sessions s WHERE s.id = session_id))
    WITH CHECK (EXISTS (SELECT 1 FROM interview.sessions s WHERE s.id = session_id));

GRANT SELECT, INSERT ON interview.session_bundles TO prepeet_app;
