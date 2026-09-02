-- 0057: token-scoped access to an invitation.
--
-- SCR-05. A candidate accepting an invitation is not acting as the tenant and
-- is not signed in: they arrive holding a token and nothing else. So the
-- recruiter's tenant policy from 0056, which scopes the row to the active
-- tenant, matches nothing for them, and the whole acceptance path would read
-- an empty table.
--
-- What authorizes the candidate is the token itself. It was emailed to them,
-- it is 32 random bytes, and only its hash is stored; presenting it proves they
-- are the person the invitation was sent to. So the access this policy grants
-- is exactly that: the one row whose stored hash equals the hash the caller has
-- set on the transaction. You can reach a row only if you already hold the
-- secret that produced its hash, which is the same thing the token is.
--
-- This is the practice-owner policy's shape applied to a party who has no user
-- id yet: 0012 lets a practice session's owner reach it in an untenanted
-- transaction carrying app.user_id; this lets an invitation's holder reach it
-- in an untenanted transaction carrying app.invitation_token_hash. Both replace
-- the tenant with a narrower proof of who is asking.
--
-- Permissive, so it is OR-ed with the tenant policy rather than replacing it: a
-- recruiter with a tenant set still sees their tenant's invitations, and a
-- candidate with only the token set sees the one the token names. Neither can
-- see the other's view, because a caller sets one context or the other, never
-- both, and a caller who sets neither sees nothing.
--
-- Implements part of SCR-05.

CREATE POLICY invitation_by_token ON recruiting.invitation
    USING (token_hash = NULLIF(current_setting('app.invitation_token_hash', true), ''))
    WITH CHECK (token_hash = NULLIF(current_setting('app.invitation_token_hash', true), ''));
