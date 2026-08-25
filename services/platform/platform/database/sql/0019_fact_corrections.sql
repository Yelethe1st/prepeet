-- 0019: the correction lifecycle's storage half.
--
-- PRO-04's rule is that correcting a fact records the correction without
-- destroying the original extraction: the extracted value column is never
-- rewritten, and the candidate's version lives beside it. What downstream
-- consumers read is the effective value - the correction where one exists,
-- the extraction otherwise, and nothing at all for a rejected fact - so a
-- corrected fact is the one used in composition from that point forward
-- while the original stays auditable underneath it.
--
-- Implements part of PRO-04.

ALTER TABLE candidate.extracted_facts
    ADD COLUMN corrected_value jsonb,
    ADD COLUMN reviewed_at timestamptz;

COMMENT ON COLUMN candidate.extracted_facts.corrected_value IS
    'The candidate''s version of the fact, present exactly when status is '
    'corrected. The extracted value is never rewritten; this lives beside it.';

COMMENT ON COLUMN candidate.extracted_facts.reviewed_at IS
    'When the candidate last acted on the fact. NULL while proposed.';

-- A corrected fact without a correction would be a status that lies, and a
-- correction on a fact in any other status would be one nothing reads.
ALTER TABLE candidate.extracted_facts
    ADD CONSTRAINT corrected_facts_carry_their_correction
    CHECK ((status = 'corrected') = (corrected_value IS NOT NULL));
