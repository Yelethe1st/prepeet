-- 0011: the candidate schema's first table, and the separation it must carry.
--
-- Practice is private. A tenant's authority never reaches a candidate's
-- practice history, in either direction, through any route: an employer seeing
-- rehearsals a candidate believed were private is the failure this product
-- cannot have, and docs/architecture/authorization-model.md makes it the
-- second-highest quality attribute after trust itself.
--
-- This migration establishes how every candidate-owned table is shaped, and
-- the isolation suite enforces the shape structurally from here on:
--
--   1. Owner-scoped row-level security, forced. The policy compares
--      app.user_id and nothing else. There is no tenant path because there is
--      no tenant column, and the suite refuses a candidate table that grows
--      one.
--   2. A write tripwire. Row-level security cannot distinguish "the owner,
--      acting as themselves" from "the owner's identifier, reached through a
--      code path that also set tenant context" - the WITH CHECK passes in
--      both. The trigger below refuses any write while app.tenant_id is set,
--      because practice writes have no business inside tenant-scoped
--      transactions, and a code path that mixes the two is exactly the bug
--      IAM-06 exists to stop shipping.
--
-- Reads under tenant context cannot alarm this way - a policy returns empty
-- rather than raising - so the read half of the guarantee is carried by the
-- policy plus the adversarial suite in CI, which is a required gate.
--
-- Implements part of IAM-06.

CREATE TABLE candidate.practice_sessions (
    id           uuid        PRIMARY KEY,

    -- The owner. The only authority dimension this table has.
    user_id      uuid        NOT NULL REFERENCES identity.users (id),

    status       text        NOT NULL DEFAULT 'created'
                             CHECK (status IN ('created', 'completed', 'abandoned')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

COMMENT ON TABLE candidate.practice_sessions IS
    'A candidate''s own practice history. Owner-scoped, tenant-free, and both '
    'properties are enforced by the isolation suite rather than remembered. '
    'The session lifecycle tickets grow this table; they may not reshape its '
    'authority.';

ALTER TABLE candidate.practice_sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE candidate.practice_sessions FORCE ROW LEVEL SECURITY;

-- The whole policy. No tenant branch, no role branch: the person, or nothing.
CREATE POLICY practice_owner ON candidate.practice_sessions
    USING (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid)
    WITH CHECK (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid);

-- The tripwire. SECURITY DEFINER is not needed; the function reads only its
-- own transaction's settings. The exception message names the invariant and
-- the ticket, because whoever sees it in a log is looking at a stop-ship
-- defect, not an input error to retry.
CREATE FUNCTION candidate.refuse_tenant_context() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NULLIF(current_setting('app.tenant_id', true), '') IS NOT NULL THEN
        RAISE EXCEPTION 'practice data written under tenant authority: this is the practice/screening separation (IAM-06) failing, stop-ship'
            USING ERRCODE = 'insufficient_privilege';
    END IF;
    RETURN COALESCE(NEW, OLD);
END
$$;

CREATE TRIGGER practice_sessions_no_tenant_context
    BEFORE INSERT OR UPDATE OR DELETE ON candidate.practice_sessions
    FOR EACH ROW EXECUTE FUNCTION candidate.refuse_tenant_context();

-- DELETE is granted because candidate.practice.delete_own exists; it demands
-- step-up authentication at the policy layer, not here.
GRANT SELECT, INSERT, UPDATE, DELETE ON candidate.practice_sessions TO prepeet_app;
