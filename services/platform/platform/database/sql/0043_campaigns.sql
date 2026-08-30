-- 0043: campaigns, the configuration they pin, and who may see them.
--
-- SCR-01, and the storage half of ADR-0020.
--
-- A campaign is the unit an invitation is issued under. It fixes one role,
-- one published rubric and calibration, one persona and plan, and the
-- jurisdiction determination it was opened against. Everything it fixes is
-- pinned by digest rather than by reference, because a reference resolves to
-- whatever is current and a digest resolves to what was actually chosen. That
-- is the whole of the ticket's second criterion: publishing a new rubric
-- version writes a new artifact row with a new digest, and a running campaign
-- keeps pointing at the digest it opened with, so nothing it has already
-- scored can move underneath it.
--
-- Implements SCR-01.

-- The legal determination for one jurisdiction, as ADR-0020 defines it.
--
-- Not tenant-scoped, because a determination is the law of a jurisdiction
-- rather than one tenant's data, and every tenant operating there is bound by
-- the same one. Immutable and versioned: a determination that changes is a new
-- row, so a campaign pinned to the old one is unaffected, exactly as it is for
-- artifacts.
CREATE TABLE recruiting.jurisdiction_determination (
    id               uuid        PRIMARY KEY,

    -- ISO 3166-1 alpha-2, or alpha-2 with a subdivision where a determination
    -- is narrower than a country. Text rather than an enum: the set of places
    -- this product operates in is a business fact that changes without a
    -- migration.
    jurisdiction     text        NOT NULL,
    version          integer     NOT NULL CHECK (version > 0),

    -- What a candidate may see of their own evaluation. Ordered here from most
    -- to least disclosing; the API enforces it on every read path rather than
    -- the UI hiding a link, per ADR-0020.
    result_disclosure text       NOT NULL CHECK (result_disclosure IN (
                                     'full_evaluation', 'evidence_without_band',
                                     'completion_status', 'submission_only')),

    -- Whether appeal is a legal right, a tenant's option, or our policy.
    appeal_status    text        NOT NULL CHECK (appeal_status IN (
                                     'right', 'tenant_option', 'platform_policy')),

    -- Who signed it off. A determination without a named human is not a
    -- determination, so this is NOT NULL with no default: DEC-11's first
    -- criterion is a person, and the schema refuses to pretend otherwise.
    approver         text        NOT NULL CHECK (length(trim(approver)) > 0),
    approved_at      timestamptz NOT NULL,

    created_at       timestamptz NOT NULL DEFAULT now(),

    UNIQUE (jurisdiction, version)
);

COMMENT ON TABLE recruiting.jurisdiction_determination IS
    'One jurisdiction''s answers to DEC-11: what a candidate may see of their '
    'own evaluation, and whether appeal is a right. Immutable and versioned; a '
    'campaign pins the version it opened under. A jurisdiction with no row '
    'here cannot have a campaign opened in it at all.';

CREATE OR REPLACE FUNCTION recruiting.refuse_determination_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'recruiting.jurisdiction_determination is immutable: a changed determination is a new version';
END $$;

CREATE TRIGGER jurisdiction_determination_immutable
    BEFORE UPDATE OR DELETE ON recruiting.jurisdiction_determination
    FOR EACH ROW EXECUTE FUNCTION recruiting.refuse_determination_change();

ALTER TABLE recruiting.jurisdiction_determination ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.jurisdiction_determination FORCE ROW LEVEL SECURITY;

-- Readable by everyone, writable by nobody through the application role. The
-- policy is deliberately unconditional rather than absent: an absent policy
-- under FORCE RLS hides every row, and a table nothing can read would fail
-- closed in a way that looks like a missing determination, which is the one
-- state this design gives a specific meaning to.
CREATE POLICY jurisdiction_determination_readable
    ON recruiting.jurisdiction_determination FOR SELECT USING (true);

GRANT SELECT ON recruiting.jurisdiction_determination TO prepeet_app;

-- A campaign. Tenant-scoped with no practice-owner policy, because a campaign
-- is screening by definition and practice never has one.
CREATE TABLE recruiting.campaign (
    id               uuid        PRIMARY KEY,
    tenant_id        uuid        NOT NULL,

    name             text        NOT NULL CHECK (length(trim(name)) > 0),

    -- draft accepts edits, open issues invitations, closed does neither. The
    -- transition into open is where every pin is checked, so a campaign that
    -- is open is one whose configuration was published at the moment it opened.
    status           text        NOT NULL DEFAULT 'draft'
                                 CHECK (status IN ('draft', 'open', 'closed')),

    -- The catalogue role this campaign interviews for. A reference rather than
    -- a digest: the role is what the campaign is about, not configuration it
    -- pins.
    role_reference   text        NOT NULL CHECK (length(trim(role_reference)) > 0),

    jurisdiction     text        NOT NULL,
    -- Pinned at open, never null once open. ADR-0020: no determination, no
    -- campaign.
    determination_id uuid        REFERENCES recruiting.jurisdiction_determination (id),

    opened_at        timestamptz,
    closed_at        timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,

    -- The two facts that make "open" mean something, enforced here rather than
    -- only in Go: an open campaign has a determination and an opened_at, and a
    -- draft has neither.
    CHECK ((status = 'draft') = (opened_at IS NULL)),
    CHECK ((status = 'draft') = (determination_id IS NULL)),
    CHECK ((status = 'closed') = (closed_at IS NOT NULL))
);

COMMENT ON TABLE recruiting.campaign IS
    'The unit an invitation is issued under: one role, one pinned '
    'configuration, one jurisdiction determination. Opening is the moment '
    'every pin is checked and frozen.';

CREATE INDEX campaign_by_tenant ON recruiting.campaign (tenant_id, status, created_at);

ALTER TABLE recruiting.campaign ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.campaign FORCE ROW LEVEL SECURITY;

CREATE POLICY campaign_tenant ON recruiting.campaign
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE ON recruiting.campaign TO prepeet_app;

-- What a campaign pinned, one row per kind of artifact.
--
-- A child table rather than a column per artifact type, because the set of
-- things a campaign pins grows: SCR-02 adds the disclosure and SCR-06 the
-- accommodation policy. The unique constraint on (campaign_id, artifact_type)
-- is what makes "one rubric, one calibration" a schema fact rather than a
-- convention somebody has to remember.
CREATE TABLE recruiting.campaign_pin (
    campaign_id   uuid        NOT NULL REFERENCES recruiting.campaign (id) ON DELETE CASCADE,
    tenant_id     uuid        NOT NULL,

    artifact_type text        NOT NULL,
    artifact_id   uuid        NOT NULL,

    -- The identity that matters. content.artifacts says the digest is what
    -- sessions pin, and a campaign pins the same thing for the same reason:
    -- a reference resolves to whatever is current, a digest to what was chosen.
    -- content.DigestOf produces "sha256:" and 64 hex characters, and this
    -- refuses anything else rather than accepting a reference that was passed
    -- where a digest was meant. The first draft of this constraint checked for
    -- 64 characters and would have rejected every real digest the platform
    -- produces, which the first integration test caught.
    digest        text        NOT NULL CHECK (digest ~ '^sha256:[0-9a-f]{64}$'),
    reference     text        NOT NULL,
    version       text        NOT NULL,

    pinned_at     timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (campaign_id, artifact_type)
);

COMMENT ON TABLE recruiting.campaign_pin IS
    'One row per artifact a campaign fixed, identified by digest. Publishing a '
    'new version of an artifact writes a new row in content.artifacts with a '
    'new digest and leaves every pin here untouched.';

CREATE OR REPLACE FUNCTION recruiting.refuse_pin_change()
RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION
        'recruiting.campaign_pin is immutable: a campaign''s configuration is fixed when it opens';
END $$;

-- Only UPDATE and DELETE are refused. A draft campaign builds its pins by
-- inserting them, and opening is what makes the set final; the trigger is what
-- stops an open campaign's configuration being edited afterwards, which is the
-- ticket's second criterion said the other way round.
CREATE TRIGGER campaign_pin_immutable
    BEFORE UPDATE OR DELETE ON recruiting.campaign_pin
    FOR EACH ROW EXECUTE FUNCTION recruiting.refuse_pin_change();

ALTER TABLE recruiting.campaign_pin ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.campaign_pin FORCE ROW LEVEL SECURITY;

CREATE POLICY campaign_pin_tenant ON recruiting.campaign_pin
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT ON recruiting.campaign_pin TO prepeet_app;

-- Which recruiter may work on which campaign.
--
-- The ticket's third criterion is that recruiter access is scoped per campaign
-- and enforced server-side. Membership of the tenant is not enough: a tenant
-- may run campaigns for several roles and teams, and a recruiter on one is not
-- thereby a recruiter on all of them. This table is that scope.
--
-- Be precise about where the guarantee actually lives, because an earlier
-- version of this comment was not. recruiting.campaign carries a tenant policy
-- and nothing narrower, so row-level security alone does NOT scope a campaign
-- to a recruiter: it scopes it to the tenant. The per-campaign guarantee comes
-- from reading campaigns only through the join in CampaignsForRecruiter, which
-- yields nothing to a caller who forgets rather than everything.
--
-- That makes the join load-bearing, so a plain by-id read of this table is a
-- bypass wearing ordinary clothes. One existed here, generated and uncalled,
-- and was removed rather than left for the next caller to reach for. A future
-- by-id read belongs in a query that carries the recruiter with it.
--
-- platform/authz declares ScopeCampaign for six permissions and no production
-- code consults it yet, so treat that catalogue as intent rather than as a
-- second line of defence.
CREATE TABLE recruiting.campaign_recruiter (
    campaign_id uuid        NOT NULL REFERENCES recruiting.campaign (id) ON DELETE CASCADE,
    tenant_id   uuid        NOT NULL,
    user_id     uuid        NOT NULL,

    granted_at  timestamptz NOT NULL DEFAULT now(),
    granted_by  uuid        NOT NULL,

    PRIMARY KEY (campaign_id, user_id)
);

COMMENT ON TABLE recruiting.campaign_recruiter IS
    'Per-campaign recruiter access. Tenant membership admits somebody to the '
    'workspace; this admits them to one campaign within it.';

CREATE INDEX campaign_recruiter_by_user ON recruiting.campaign_recruiter (tenant_id, user_id);

ALTER TABLE recruiting.campaign_recruiter ENABLE ROW LEVEL SECURITY;
ALTER TABLE recruiting.campaign_recruiter FORCE ROW LEVEL SECURITY;

CREATE POLICY campaign_recruiter_tenant ON recruiting.campaign_recruiter
    USING (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, DELETE ON recruiting.campaign_recruiter TO prepeet_app;
