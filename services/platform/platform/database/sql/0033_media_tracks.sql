-- 0033: media tracks, the recording's durable record.
--
-- RTC-05 per ADR-0013. Recording is server-side SFU egress: two Opus/WebM
-- tracks (the candidate's audio and the interviewer's synthesized audio,
-- never a mix) written to the object store under the session's key layout.
-- One row per track records where the artifact lives, which egress wrote
-- it, and what state it reached. A transcript-only session never gets rows
-- here: audio that must not outlive the session is audio that never
-- becomes durable at all.
--
-- The state ladder is one-way: recording, then finalized (the object was
-- read back and its digest recorded) or missing (it was not there, or did
-- not verify). Finalized and missing rows are immutable; a failed
-- recording is a fact to show, never a row to tidy.
--
-- Implements part of RTC-05.

CREATE TABLE interview.media_tracks (
    id             uuid        PRIMARY KEY,
    session_id     uuid        NOT NULL REFERENCES interview.sessions (id),
    mode           text        NOT NULL CHECK (mode IN ('practice', 'screening')),
    candidate_id   uuid        NOT NULL,
    tenant_id      uuid        CHECK ((mode = 'practice') = (tenant_id IS NULL)),

    track          text        NOT NULL CHECK (track IN ('candidate', 'interviewer')),
    storage_key    text        NOT NULL,
    egress_id      text        NOT NULL,

    state          text        NOT NULL DEFAULT 'recording'
                               CHECK (state IN ('recording', 'finalized', 'missing')),

    -- Set at finalization, from reading the stored object back: the
    -- reconciliation proof, not the recorder's claim.
    digest         text        NOT NULL DEFAULT '',
    size_bytes     bigint      NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),

    created_at     timestamptz NOT NULL DEFAULT now(),
    resolved_at    timestamptz,

    -- One egress per session and track: a reconnection joins the same
    -- room and must never start a second recording of it.
    UNIQUE (session_id, track)
);

COMMENT ON TABLE interview.media_tracks IS
    'One egress track per session and side. State moves recording to '
    'finalized (object read back, digest recorded) or missing, once, and '
    'then the row is immutable. Transcript-only sessions have no rows.';

CREATE OR REPLACE FUNCTION interview.refuse_resolved_track_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.state <> 'recording' THEN
        RAISE EXCEPTION 'interview.media_tracks: a % track is immutable', OLD.state;
    END IF;
    RETURN NEW;
END $$;

CREATE TRIGGER media_tracks_resolve_once
    BEFORE UPDATE OR DELETE ON interview.media_tracks
    FOR EACH ROW EXECUTE FUNCTION interview.refuse_resolved_track_change();

ALTER TABLE interview.media_tracks ENABLE ROW LEVEL SECURITY;
ALTER TABLE interview.media_tracks FORCE ROW LEVEL SECURITY;

CREATE POLICY media_tracks_tenant ON interview.media_tracks
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

CREATE POLICY media_tracks_practice_owner ON interview.media_tracks
    USING (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL)
    WITH CHECK (mode = 'practice'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);

GRANT SELECT, INSERT, UPDATE ON interview.media_tracks TO prepeet_app;
