-- 0024: admit consent texts as an artifact type.
--
-- The words a person consents against are content with the strongest
-- versioning requirement in the product: a session stores the version it
-- presented, and that pointer must resolve to identical words forever.
-- The registry is the one place that already keeps that promise.
--
-- Implements part of CAT-05.

ALTER TABLE content.artifacts
    DROP CONSTRAINT artifacts_artifact_type_check;

ALTER TABLE content.artifacts
    ADD CONSTRAINT artifacts_artifact_type_check CHECK (artifact_type IN (
        'persona', 'plan', 'rule_pack', 'rubric', 'role_standard',
        'prompt', 'model_policy', 'articulation_policy', 'catalogue',
        'consent_text'));
