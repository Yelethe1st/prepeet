-- 0023: the recording preference and the consent it was given against.
--
-- CAT-05: at composition the candidate chooses what the session keeps -
-- audio and transcript, or transcript only - and the choice is stored with
-- the version of the consent text it was made against, because "what did
-- this person agree to" must stay answerable after the text changes
-- (retention-and-deletion.md: policy is versioned and snapshotted where it
-- affects consent/session data). RTC-05 honours the preference at capture.
--
-- Both are written at creation and never after, joining config under the
-- immutability trigger: a preference that drifted after composition would
-- have the recording pipeline honouring a choice nobody made.
--
-- The legacy default is transcript_only: for any row created before this
-- column existed, the data-minimising reading is the only defensible one.

ALTER TABLE interview.sessions
    ADD COLUMN recording_preference text NOT NULL DEFAULT 'transcript_only'
        CHECK (recording_preference IN ('audio_and_transcript', 'transcript_only')),
    ADD COLUMN consent_version text NOT NULL DEFAULT '';

COMMENT ON COLUMN interview.sessions.recording_preference IS
    'What this session keeps, chosen by the candidate at composition and '
    'honoured by media capture. Immutable after creation.';

COMMENT ON COLUMN interview.sessions.consent_version IS
    'The published version of consent/practice-recording the choice was made '
    'against. Empty only for sessions that predate the column.';

CREATE OR REPLACE FUNCTION interview.refuse_config_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.config IS DISTINCT FROM OLD.config THEN
        RAISE EXCEPTION 'interview.sessions.config is immutable after creation';
    END IF;
    IF NEW.recording_preference IS DISTINCT FROM OLD.recording_preference
       OR NEW.consent_version IS DISTINCT FROM OLD.consent_version THEN
        RAISE EXCEPTION 'interview.sessions recording consent is immutable after creation';
    END IF;
    RETURN NEW;
END $$;
