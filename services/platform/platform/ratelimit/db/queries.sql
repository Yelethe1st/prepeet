-- The rate limiter's queries. sqlc generates the Go in this directory from this
-- file; ADR-0008 records why they moved here from hand-written pgx.

-- name: Increment :one
-- One statement, not a read followed by a write. INSERT ... ON CONFLICT DO
-- UPDATE ... RETURNING increments and reads the new value atomically, so two
-- requests arriving together cannot both read the old count and both be
-- allowed. That race is the whole failure mode of a rate limiter.
INSERT INTO security_rate_limit_counters (key, window_start, count)
VALUES ($1, $2, 1)
ON CONFLICT (key, window_start)
DO UPDATE SET count = security_rate_limit_counters.count + 1
RETURNING count;

-- name: Sweep :execrows
-- Old windows are deleted rather than left, because the table is unlogged and
-- its only value is the current window; keeping the rest would grow a table
-- nobody reads.
DELETE FROM security_rate_limit_counters WHERE window_start < $1;
