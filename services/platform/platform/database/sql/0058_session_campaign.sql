-- 0058: a screening session belongs to a campaign, and its candidate can see it.
--
-- The first half of the screening interview run. Until now interview.sessions
-- carried a tenant but not the campaign the interview is for, so nothing tied a
-- screening session to the configuration it must be composed against or the
-- determination its result is disclosed under. This adds that link and makes it
-- a schema fact: a screening session has a campaign and a practice session never
-- does, checked here rather than remembered in Go.
--
-- The second half is access. 0012 gave screening sessions one policy, the
-- tenant's, because the recruiter and the composition run act as the tenant. But
-- the candidate who sits the interview is not a member of that tenant and acts
-- in no tenant at all, so under that policy alone they cannot see their own
-- screening session. This adds the missing owner policy, the screening analogue
-- of the practice-owner policy beside it: the candidate, acting as themselves in
-- an untenanted transaction, may read the screening session that is theirs.
--
-- It is a read policy on purpose. Creating and transitioning a screening session
-- is the run's work and lands with the later layers that decide, carefully, what
-- a candidate may change and when; giving the candidate SELECT now is what the
-- run needs first and grants nothing a candidate should not already have, which
-- is sight of their own interview.
--
-- Implements part of SCR-05 and SES-01, and the storage the SCR-06 fulfilment
-- and SCR-07 disclosure both build on.

ALTER TABLE interview.sessions
    ADD COLUMN campaign_id uuid REFERENCES recruiting.campaign (id);

-- Screening has a campaign; practice, and any mode added later, does not. The
-- equivalence mirrors the tenant_id CHECK from 0012: the two facts that make a
-- mode what it is are enforced by the shape, so a screening row with no campaign
-- or a practice row with one cannot be written at all.
ALTER TABLE interview.sessions
    ADD CONSTRAINT sessions_screening_has_campaign
    CHECK ((mode = 'screening') = (campaign_id IS NOT NULL));

COMMENT ON COLUMN interview.sessions.campaign_id IS
    'The campaign a screening session runs for, whose pinned configuration it '
    'is composed against and whose determination its result is disclosed under. '
    'NULL for practice by CHECK, exactly as tenant_id is.';

-- The candidate's sight of their own screening session, as the practice-owner
-- policy is their sight of their own practice session. Keyed to app.user_id and
-- guarded by the absence of a tenant, so a tenant-scoped code path reads
-- screening sessions through the tenant policy and never through this one, and a
-- candidate reads only their own.
CREATE POLICY sessions_screening_candidate ON interview.sessions
    FOR SELECT
    USING (mode = 'screening'
           AND candidate_id = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);
