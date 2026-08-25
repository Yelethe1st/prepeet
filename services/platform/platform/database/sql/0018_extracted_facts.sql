-- 0018: extracted facts, with the provenance PRO-03 is about.
--
-- Every fact links to the exact span of the source document that produced
-- it, carries a confidence and the extractor version, and starts life as a
-- proposal: the candidate confirms, corrects or rejects it in PRO-04, and
-- until then it changes nothing. Text the extractor could not parse is a
-- fact too - kind 'unparsed' - because dropping it silently would present a
-- partial reading as a complete one.
--
-- The document row learns its extraction state, so a profile screen can say
-- "we read your CV", "we could not read this format" or "extraction failed,
-- your profile works without it" - the degradation criterion, visible.
--
-- Candidate schema: IAM-06's structural guards apply by existing.
--
-- Implements part of PRO-03.

ALTER TABLE candidate.documents
    ADD COLUMN extraction_state text NOT NULL DEFAULT 'none'
        CHECK (extraction_state IN ('none', 'pending', 'extracted', 'unsupported', 'failed'));

COMMENT ON COLUMN candidate.documents.extraction_state IS
    'Where extraction stands for this version. none until stored, pending '
    'while the workflow runs, then extracted, unsupported (a format the '
    'extractor honestly cannot read) or failed. Never blocks the profile.';

CREATE TABLE candidate.extracted_facts (
    id          uuid        PRIMARY KEY,
    user_id     uuid        NOT NULL REFERENCES identity.users (id),
    document_id uuid        NOT NULL REFERENCES candidate.documents (id),

    kind        text        NOT NULL CHECK (kind IN
                                ('role', 'skill', 'date_range', 'achievement', 'unparsed')),

    -- The fact's structured value, shaped per kind by the extraction schema
    -- version below.
    value       jsonb       NOT NULL,

    -- The provenance: the half-open byte range [span_start, span_end) of the
    -- extracted text that produced this fact. The exact source span is the
    -- first acceptance criterion, so it is NOT NULL: a fact that cannot say
    -- where it came from is not stored.
    span_start  integer     NOT NULL CHECK (span_start >= 0),
    span_end    integer     NOT NULL CHECK (span_end > span_start),

    confidence  numeric(3,2) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),

    -- Which extractor produced it, for reproducing or superseding a reading.
    extractor_version text  NOT NULL,

    -- The correction lifecycle. Everything starts proposed; PRO-04 moves it.
    status      text        NOT NULL DEFAULT 'proposed'
                            CHECK (status IN ('proposed', 'confirmed', 'corrected', 'rejected')),

    created_at  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE candidate.extracted_facts IS
    'What extraction read from a candidate document, span-linked and '
    'versioned. Proposals until the candidate acts on them; unparsed text is '
    'recorded rather than dropped.';

CREATE INDEX extracted_facts_by_document ON candidate.extracted_facts (document_id);

ALTER TABLE candidate.extracted_facts ENABLE ROW LEVEL SECURITY;
ALTER TABLE candidate.extracted_facts FORCE ROW LEVEL SECURITY;

CREATE POLICY facts_owner ON candidate.extracted_facts
    USING (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (user_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

CREATE TRIGGER facts_no_tenant_context
    BEFORE INSERT OR UPDATE OR DELETE ON candidate.extracted_facts
    FOR EACH ROW EXECUTE FUNCTION candidate.refuse_tenant_context();

GRANT SELECT, INSERT, UPDATE, DELETE ON candidate.extracted_facts TO prepeet_app;
