-- 0020: admit the catalogue as an artifact type.
--
-- CAT-03 makes the interview catalogue - disciplines, roles, shapes,
-- personas and their valid combinations - registry content rather than
-- code, so "add a profession" is a published version and never a deploy.
-- One document rather than one artifact per collection, because the
-- combination rules cross the collections and splitting them would let the
-- halves version apart.
--
-- Implements part of CAT-03.

ALTER TABLE content.artifacts
    DROP CONSTRAINT artifacts_artifact_type_check;

ALTER TABLE content.artifacts
    ADD CONSTRAINT artifacts_artifact_type_check CHECK (artifact_type IN (
        'persona', 'plan', 'rule_pack', 'rubric', 'role_standard',
        'prompt', 'model_policy', 'articulation_policy', 'catalogue'));
