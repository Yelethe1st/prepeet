-- 0041: delivery analysis is practice only, in the schema.
--
-- 0035 allowed mode IN ('practice', 'screening') and carried a tenant policy
-- for the screening case, so a screening delivery analysis was a row the
-- database would happily store. Nothing produced one, which is not the same as
-- nothing being able to: the workflow forwarded whatever mode the completion
-- event carried, and a screening completion whose pinned policy included the
-- articulation stage would have written one.
--
-- The product says this boundary in candidate-facing copy: screening never
-- produces delivery coaching, and practice never reaches an employer. A row
-- that contradicts it is a row some future read exposes, so the constraint
-- says it too. Code refuses it twice over as well; this is the half that
-- cannot be forgotten by a new producer.
--
-- The tenant policy goes with it. With no screening rows there is nothing for
-- a tenant scope to match, and a policy that can never match is a policy that
-- misleads whoever reads the table next.
--
-- Implements part of ART-02 and ART-03's practice-only boundary.

DELETE FROM evaluation.articulation WHERE mode <> 'practice';

-- The original constraint was written inline, so PostgreSQL named it. Found
-- rather than guessed: a DROP of a name that does not exist would leave the
-- old rule in place and this migration would silently do half its job.
DO $$
DECLARE
    existing text;
BEGIN
    SELECT conname INTO existing
    FROM pg_constraint
    WHERE conrelid = 'evaluation.articulation'::regclass
      AND contype = 'c'
      AND pg_get_constraintdef(oid) LIKE '%screening%';

    IF existing IS NOT NULL THEN
        EXECUTE format('ALTER TABLE evaluation.articulation DROP CONSTRAINT %I', existing);
    END IF;
END $$;

ALTER TABLE evaluation.articulation
    ADD CONSTRAINT articulation_mode_practice CHECK (mode = 'practice');

DROP POLICY IF EXISTS articulation_tenant ON evaluation.articulation;

COMMENT ON TABLE evaluation.articulation IS
    'Delivery measurements per session with their assessability. Practice '
    'only, by CHECK and by having no tenant policy: screening never produces '
    'delivery coaching. Separate from evaluation.results by design so '
    'delivery can fail without touching content evaluation. Immutable.';
