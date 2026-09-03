-- 0060: a campaign's job context and the requirements extracted from it.
--
-- SCR-03. The job description a screening interview draws on, and the
-- requirements pulled out of it, each linked to the exact span of the source it
-- came from so EVL-06 can report evidence against a requirement whose origin is
-- auditable rather than asserted.
--
-- Two tables. The job context is the source text, one per campaign, replaced
-- wholesale when a recruiter submits a new one. The requirements are what
-- extraction found in it, reviewable and correctable by the recruiter, and
-- frozen when the campaign opens: that freeze is what "pinned into the campaign
-- configuration" means here, and it is a trigger rather than a promise, exactly
-- as the campaign's artifact pins are.
--
-- Implements SCR-03.

CREATE TABLE recruiting.job_context (
    campaign_id      uuid        PRIMARY KEY REFERENCES recruiting.campaign (id) ON DELETE CASCADE,
    tenant_id        uuid        NOT NULL,

    -- The job description as the recruiter provided it. Extraction reads this
    -- and requirement spans are measured in its bytes, so it is stored verbatim
    -- rather than normalised: a span that indexed into cleaned-up text would
    -- point at bytes the recruiter never saw.
    source_text      text        NOT NULL CHECK (length(trim(source_text)) > 0),
    extraction_version text      NOT NULL,

    submitted_at     timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE recruiting.job_context IS
    'The job description a campaign draws on, stored verbatim so requirement '
    'spans index into the exact bytes the recruiter provided. One per campaign, '
    'replaced wholesale on resubmission.';

ALTER TABLE recruiting.job_context ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.job_context FORCE ROW LEVEL SECURITY;

CREATE POLICY job_context_tenant ON recruiting.job_context
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON recruiting.job_context TO prepeet_app;

-- One requirement extracted from the job context, with the span it came from.
CREATE TABLE recruiting.campaign_requirement (
    id           uuid        PRIMARY KEY,
    campaign_id  uuid        NOT NULL REFERENCES recruiting.campaign (id) ON DELETE CASCADE,
    tenant_id    uuid        NOT NULL,

    -- The requirement as it stands: extraction's text until a recruiter
    -- corrects it, then theirs. What EVL-06 reports evidence against.
    text         text        NOT NULL CHECK (length(trim(text)) > 0),

    -- The provenance: the half-open range [span_start, span_end) of the job
    -- context's source_text this requirement came from, in the same shape a
    -- candidate fact's span takes. A correction keeps the span, because where
    -- the requirement came from does not change when its wording is fixed.
    span_start   integer     NOT NULL CHECK (span_start >= 0),
    span_end     integer     NOT NULL CHECK (span_end > span_start),

    -- proposed is extraction's, corrected is the recruiter's edit, rejected is
    -- one they removed. Rejected rows stay, so the review is auditable, and are
    -- simply not pinned into what the campaign runs against.
    status       text        NOT NULL DEFAULT 'proposed'
                             CHECK (status IN ('proposed', 'corrected', 'rejected')),

    extraction_version text  NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE recruiting.campaign_requirement IS
    'A requirement extracted from a campaign''s job context, span-linked to its '
    'source, reviewable and correctable while the campaign is a draft and frozen '
    'when it opens.';

CREATE INDEX campaign_requirement_by_campaign
    ON recruiting.campaign_requirement (campaign_id, created_at);

-- The freeze that makes "pinned into the campaign configuration" real: once the
-- campaign is not a draft, its requirements cannot change, the same guarantee
-- campaign_pin makes for the artifacts. A draft builds and corrects its
-- requirements freely; opening is what fixes them, so what the interview was
-- judged against stays answerable afterwards.
CREATE OR REPLACE FUNCTION recruiting.refuse_requirement_change_after_open()
RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
    campaign_status text;
BEGIN
    SELECT status INTO campaign_status FROM recruiting.campaign
    WHERE id = COALESCE(NEW.campaign_id, OLD.campaign_id);
    IF campaign_status IS DISTINCT FROM 'draft' THEN
        RAISE EXCEPTION
            'recruiting.campaign_requirement is frozen once the campaign opens: a running campaign''s requirements are fixed';
    END IF;
    RETURN COALESCE(NEW, OLD);
END $$;

CREATE TRIGGER campaign_requirement_frozen_after_open
    BEFORE INSERT OR UPDATE OR DELETE ON recruiting.campaign_requirement
    FOR EACH ROW EXECUTE FUNCTION recruiting.refuse_requirement_change_after_open();

ALTER TABLE recruiting.campaign_requirement ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.campaign_requirement FORCE ROW LEVEL SECURITY;

CREATE POLICY campaign_requirement_tenant ON recruiting.campaign_requirement
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON recruiting.campaign_requirement TO prepeet_app;
