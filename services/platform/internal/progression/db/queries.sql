-- The progression store's queries. sqlc generates the Go beside this
-- file; ADR-0010 records why no SQL lives in Go source.

-- name: InsertObservation :execrows
-- Idempotent per (evaluation, competency): the consumer's retry converges
-- on the history already written.
INSERT INTO progression.observations
    (id, candidate_id, mode, tenant_id, session_id, evaluation_id,
     competency_id, status, band, confidence, evidence_count,
     supporting, contradictory, unverified, gaps,
     rubric_reference, rubric_version, rubric_digest,
     aggregation_version, extraction_version, model_version, policy_version,
     supersedes, observed_at)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(candidate_id)::uuid, sqlc.arg(mode)::text,
        nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(session_id)::uuid, sqlc.arg(evaluation_id)::uuid,
        sqlc.arg(competency_id)::text, sqlc.arg(status)::text,
        sqlc.arg(band)::text, sqlc.arg(confidence)::text,
        sqlc.arg(evidence_count)::integer,
        sqlc.arg(supporting)::integer, sqlc.arg(contradictory)::integer,
        sqlc.arg(unverified)::integer, sqlc.arg(gaps)::integer,
        sqlc.arg(rubric_reference)::text, sqlc.arg(rubric_version)::text,
        sqlc.arg(rubric_digest)::text,
        sqlc.arg(aggregation_version)::text, sqlc.arg(extraction_version)::text,
        sqlc.arg(model_version)::text, sqlc.arg(policy_version)::text,
        nullif(sqlc.arg(supersedes)::text, '')::uuid,
        sqlc.arg(observed_at)::timestamptz)
ON CONFLICT (evaluation_id, competency_id) DO NOTHING;

-- name: ListObservations :many
-- The whole history for one candidate, oldest first: what a progression
-- chart plots, every version of every reading included.
SELECT id::text AS id, session_id::text AS session_id,
       evaluation_id::text AS evaluation_id, competency_id,
       status, band, confidence, evidence_count,
       supporting, contradictory, unverified, gaps,
       rubric_reference, rubric_version, rubric_digest,
       aggregation_version, extraction_version, model_version, policy_version,
       coalesce(supersedes::text, '')::text AS supersedes,
       observed_at, created_at
FROM progression.observations
ORDER BY observed_at, competency_id;
