-- 0059: an accepted invitation remembers who accepted it.
--
-- SCR-05 recorded that an invitation was accepted, but not by whom: the
-- recipient is an email address, and acceptance resolved it to a candidate
-- account without writing that account back. The screening session an accepted
-- candidate is about to start needs to prove they accepted, so acceptance now
-- records the candidate it resolved to.
--
-- Nullable, and set only on the accept transition, because it means exactly one
-- thing: the account that accepted. A live, declined, revoked or superseded
-- invitation has no accepting candidate, and a column that was populated before
-- acceptance would be asserting one that does not exist yet.
--
-- The second half is access. The candidate reaching for their own screening
-- session has no tenant, so the tenant policy on this table does not admit them,
-- and the token is spent so the token policy no longer does either. This adds
-- the owner policy that lets a candidate, acting as themselves in an untenanted
-- transaction, read the invitation they accepted, which is how the session
-- creation path proves their authority to start. It is a read policy: accepting
-- is the token flow's, and nothing a candidate does later rewrites which
-- invitation they accepted.
--
-- Implements part of SCR-05, and the authority SCR-06's session-bound
-- accommodations build on.

ALTER TABLE recruiting.invitation
    ADD COLUMN accepted_candidate uuid REFERENCES identity.users (id);

-- The candidate it resolved to exists only once the invitation is accepted:
-- present exactly when the outcome is 'accepted', absent for every other state.
ALTER TABLE recruiting.invitation
    ADD CONSTRAINT invitation_accepted_has_candidate
    CHECK ((outcome = 'accepted') = (accepted_candidate IS NOT NULL));

COMMENT ON COLUMN recruiting.invitation.accepted_candidate IS
    'The candidate account acceptance resolved the recipient to, set on the '
    'accept transition and null before it. How a candidate proves, later, that '
    'they accepted an invitation to a campaign.';

-- The candidate's sight of the invitation they accepted, keyed to app.user_id
-- and guarded by the absence of a tenant, so a tenant-scoped recruiter reads
-- invitations through the tenant policy and never this one, and a candidate
-- reads only the one they themselves accepted.
CREATE POLICY invitation_accepted_candidate ON recruiting.invitation
    FOR SELECT
    USING (accepted_candidate = NULLIF(current_setting('app.user_id', true), '')::uuid
           AND NULLIF(current_setting('app.tenant_id', true), '') IS NULL);
