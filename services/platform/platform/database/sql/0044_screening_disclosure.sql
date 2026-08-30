-- 0044: what a screening candidate was told, and what they agreed to.
--
-- SCR-02, and the consent half of ADR-0020.
--
-- The disclosure itself is a registry artifact rather than a table here, for
-- the reason 0024 gave for consent texts: the words a person agreed to must
-- resolve to identical words forever, and the registry is the one place that
-- already keeps that promise. A campaign pins its disclosure by digest exactly
-- as it pins its rubric, so "a disclosure change creates a new version and
-- never rewrites what someone already accepted" is not a rule anybody has to
-- enforce; it is what publishing a new artifact version already means.
--
-- What is new here is the acceptance side, which the registry cannot model:
-- who accepted which version, and which processing they agreed to separately.
--
-- Implements SCR-02.

ALTER TABLE content.artifacts
    DROP CONSTRAINT artifacts_artifact_type_check;

ALTER TABLE content.artifacts
    ADD CONSTRAINT artifacts_artifact_type_check CHECK (artifact_type IN (
        'persona', 'plan', 'rule_pack', 'rubric', 'role_standard',
        'prompt', 'model_policy', 'articulation_policy', 'catalogue',
        'consent_text', 'screening_disclosure'));

-- One candidate accepting one version of one campaign's disclosure.
--
-- Append-only. A candidate who is shown a new version and accepts it gets a
-- second row, and both remain, because "what had this person been told when
-- they sat the interview" is a question about a moment rather than about the
-- present.
CREATE TABLE recruiting.disclosure_acceptance (
    id           uuid        PRIMARY KEY,
    tenant_id    uuid        NOT NULL,
    campaign_id  uuid        NOT NULL REFERENCES recruiting.campaign (id),
    candidate_id uuid        NOT NULL,

    -- The exact version, by the identity that cannot drift. Both are stored:
    -- the digest is what makes the claim verifiable, the version is what makes
    -- it legible to a person reading an audit.
    disclosure_digest  text  NOT NULL CHECK (disclosure_digest ~ '^sha256:[0-9a-f]{64}$'),
    disclosure_version text  NOT NULL CHECK (length(trim(disclosure_version)) > 0),

    accepted_at  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE recruiting.disclosure_acceptance IS
    'Which candidate accepted which exact version of which campaign''s '
    'disclosure, and when. Append-only: a later acceptance is a new row, so '
    'what somebody was told at the time stays answerable after the text moves.';

CREATE INDEX disclosure_acceptance_by_candidate
    ON recruiting.disclosure_acceptance (campaign_id, candidate_id, accepted_at DESC);

CREATE OR REPLACE FUNCTION recruiting.refuse_acceptance_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'recruiting.disclosure_acceptance is append-only: a later acceptance is a new row';
END $$;

CREATE TRIGGER disclosure_acceptance_append_only
    BEFORE UPDATE OR DELETE ON recruiting.disclosure_acceptance
    FOR EACH ROW EXECUTE FUNCTION recruiting.refuse_acceptance_change();

ALTER TABLE recruiting.disclosure_acceptance ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.disclosure_acceptance FORCE ROW LEVEL SECURITY;

CREATE POLICY disclosure_acceptance_tenant ON recruiting.disclosure_acceptance
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON recruiting.disclosure_acceptance TO prepeet_app;

-- One decision about one kind of processing.
--
-- A row per purpose rather than a boolean on the acceptance, because that is
-- what unbundling means structurally. A single "I agree" covering both the
-- interview and model improvement cannot be declined by half, and a schema
-- that could only record one answer would make the requirement impossible to
-- satisfy however carefully the screen was written.
--
-- required says whether the interview can happen without it. It is stored on
-- the decision rather than looked up, because whether a purpose was required is
-- a fact about the moment of asking: reclassifying a purpose later must not
-- retroactively change what a past candidate is recorded as having been
-- obliged to accept.
CREATE TABLE recruiting.consent_decision (
    id           uuid        PRIMARY KEY,
    tenant_id    uuid        NOT NULL,
    campaign_id  uuid        NOT NULL REFERENCES recruiting.campaign (id),
    candidate_id uuid        NOT NULL,

    purpose      text        NOT NULL CHECK (length(trim(purpose)) > 0),
    required     boolean     NOT NULL,
    granted      boolean     NOT NULL,

    -- The disclosure version the decision was made against, so a decision is
    -- never orphaned from the words that described it.
    disclosure_digest text   NOT NULL CHECK (disclosure_digest ~ '^sha256:[0-9a-f]{64}$'),

    decided_at   timestamptz NOT NULL DEFAULT now(),

    -- Model improvement can never be a condition of taking an interview.
    -- ADR-0020 states it and responsible-hiring.md requires it; stating it here
    -- as well means a caller cannot make it required by passing the wrong flag.
    CONSTRAINT optional_processing_is_never_required
        CHECK (NOT (required AND purpose = 'model_improvement'))
);

COMMENT ON TABLE recruiting.consent_decision IS
    'One decision per processing purpose, which is what unbundling means: a '
    'candidate declining optional processing still has every required consent '
    'recorded and can sit the interview. Append-only; the latest row per '
    'purpose is the standing decision, which is also how withdrawal will be '
    'recorded.';

CREATE INDEX consent_decision_standing
    ON recruiting.consent_decision (campaign_id, candidate_id, purpose, decided_at DESC);

CREATE TRIGGER consent_decision_append_only
    BEFORE UPDATE OR DELETE ON recruiting.consent_decision
    FOR EACH ROW EXECUTE FUNCTION recruiting.refuse_acceptance_change();

ALTER TABLE recruiting.consent_decision ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.consent_decision FORCE ROW LEVEL SECURITY;

CREATE POLICY consent_decision_tenant ON recruiting.consent_decision
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON recruiting.consent_decision TO prepeet_app;
