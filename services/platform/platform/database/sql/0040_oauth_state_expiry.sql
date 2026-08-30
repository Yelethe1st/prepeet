-- 0040: let the OAuth state sweep use an index.
--
-- 0039 indexed expires_at only for rows that had not been used, because the
-- read it was imagined for was "find the live ones". The sweep that actually
-- runs deletes every state past its expiry, used or not, and a partial index
-- cannot serve it: the planner falls back to a sequential scan of a table that
-- grows with every sign-in anybody starts.
--
-- Deleting used states is safe, and the reason is worth stating. Replay
-- detection only has to hold inside the ten-minute window: a callback arriving
-- after that is refused for being expired, and one arriving after the row is
-- gone is refused for being unknown. The two refusals are deliberately the
-- same sentence, so the person cannot tell which happened and there is nothing
-- for the row to protect once its window has closed.
--
-- Implements part of IAM-08.

DROP INDEX IF EXISTS identity.oauth_states_expiry_idx;

CREATE INDEX oauth_states_expiry_idx ON identity.oauth_states (expires_at);

COMMENT ON INDEX identity.oauth_states_expiry_idx IS
    'The sweep deletes by expiry across every state, used or not, so the '
    'index covers every row rather than only the live ones.';
