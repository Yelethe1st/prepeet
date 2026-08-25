-- 0022: the session's configuration, as the person chose it.
--
-- The wizard's selection - discipline, role, shape, minutes, persona - is
-- part of what the session IS, distinct from the blueprint that names what
-- to compose against and from the bundle that pins what actually ran. It is
-- stored whole at creation and never updated: changing your mind is a new
-- draft, and a session whose configuration drifted after composition would
-- have a bundle describing choices nobody made.
--
-- Implements part of CAT-04.

ALTER TABLE interview.sessions
    ADD COLUMN config jsonb NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN interview.sessions.config IS
    'The validated catalogue selection this session was created from. '
    'Written once at creation; the bundle, not this, is what ran.';

-- Config is written at creation and never after; the trigger makes the
-- never-after structural rather than remembered.
CREATE OR REPLACE FUNCTION interview.refuse_config_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.config IS DISTINCT FROM OLD.config THEN
        RAISE EXCEPTION 'interview.sessions.config is immutable after creation';
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER sessions_config_immutable
    BEFORE UPDATE ON interview.sessions
    FOR EACH ROW EXECUTE FUNCTION interview.refuse_config_change();
