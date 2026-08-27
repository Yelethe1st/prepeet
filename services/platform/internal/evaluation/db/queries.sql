-- The evidence store's queries. sqlc generates the Go beside this file;
-- ADR-0010 records why no SQL lives in Go source.

-- name: DeleteEvidence :exec
-- The idempotency half of storing a reading: spans for this session and
-- extractor version are replaced wholesale, so a retried stage converges.
DELETE FROM evaluation.evidence_spans
WHERE session_id = sqlc.arg(session_id)::uuid
  AND extraction_version = sqlc.arg(extraction_version)::text;

-- name: InsertEvidenceSpan :exec
INSERT INTO evaluation.evidence_spans
    (id, session_id, mode, candidate_id, tenant_id, competency_id, kind,
     segment_sequence, quote, char_start, char_end, start_ms, end_ms,
     extraction_version)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(session_id)::uuid, sqlc.arg(mode)::text,
        sqlc.arg(candidate_id)::uuid, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(competency_id)::text, sqlc.arg(kind)::text,
        sqlc.arg(segment_sequence)::integer, sqlc.arg(quote)::text,
        sqlc.arg(char_start)::integer, sqlc.arg(char_end)::integer,
        sqlc.arg(start_ms)::integer, sqlc.arg(end_ms)::integer,
        sqlc.arg(extraction_version)::text);

-- name: ListEvidence :many
SELECT id::text AS id, competency_id, kind, segment_sequence, quote,
       char_start, char_end, start_ms, end_ms, extraction_version, created_at
FROM evaluation.evidence_spans
WHERE session_id = sqlc.arg(session_id)::uuid
ORDER BY segment_sequence, char_start, competency_id, kind;

-- name: InsertResult :exec
INSERT INTO evaluation.results
    (id, session_id, mode, candidate_id, tenant_id,
     rubric_reference, rubric_version, rubric_digest,
     aggregation_version, extraction_version, model_version, policy_version,
     competencies, result_digest, covered_competencies, total_competencies,
     warnings)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(session_id)::uuid, sqlc.arg(mode)::text,
        sqlc.arg(candidate_id)::uuid, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(rubric_reference)::text, sqlc.arg(rubric_version)::text,
        sqlc.arg(rubric_digest)::text, sqlc.arg(aggregation_version)::text,
        sqlc.arg(extraction_version)::text, sqlc.arg(model_version)::text,
        sqlc.arg(policy_version)::text, sqlc.arg(competencies)::jsonb,
        sqlc.arg(result_digest)::text, sqlc.arg(covered_competencies)::integer,
        sqlc.arg(total_competencies)::integer, sqlc.arg(warnings)::jsonb);

-- name: GetResult :one
SELECT id::text AS id, session_id::text AS session_id,
       rubric_reference, rubric_version, rubric_digest,
       aggregation_version, extraction_version, model_version, policy_version,
       competencies, result_digest, covered_competencies, total_competencies,
       warnings, created_at
FROM evaluation.results
WHERE session_id = sqlc.arg(session_id)::uuid;

-- name: DeleteContradictions :exec
-- Wholesale with the spans, same reason: the retried stage converges.
DELETE FROM evaluation.contradictions
WHERE session_id = sqlc.arg(session_id)::uuid
  AND extraction_version = sqlc.arg(extraction_version)::text;

-- name: InsertContradiction :exec
INSERT INTO evaluation.contradictions
    (id, session_id, mode, candidate_id, tenant_id, topic,
     a_segment_sequence, a_quote, a_char_start, a_char_end, a_start_ms, a_end_ms,
     b_segment_sequence, b_quote, b_char_start, b_char_end, b_start_ms, b_end_ms,
     extraction_version)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(session_id)::uuid, sqlc.arg(mode)::text,
        sqlc.arg(candidate_id)::uuid, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(topic)::jsonb,
        sqlc.arg(a_segment_sequence)::integer, sqlc.arg(a_quote)::text,
        sqlc.arg(a_char_start)::integer, sqlc.arg(a_char_end)::integer,
        sqlc.arg(a_start_ms)::integer, sqlc.arg(a_end_ms)::integer,
        sqlc.arg(b_segment_sequence)::integer, sqlc.arg(b_quote)::text,
        sqlc.arg(b_char_start)::integer, sqlc.arg(b_char_end)::integer,
        sqlc.arg(b_start_ms)::integer, sqlc.arg(b_end_ms)::integer,
        sqlc.arg(extraction_version)::text);

-- name: ListContradictions :many
SELECT id::text AS id, topic,
       a_segment_sequence, a_quote, a_char_start, a_char_end, a_start_ms, a_end_ms,
       b_segment_sequence, b_quote, b_char_start, b_char_end, b_start_ms, b_end_ms,
       extraction_version, created_at
FROM evaluation.contradictions
WHERE session_id = sqlc.arg(session_id)::uuid
ORDER BY a_segment_sequence, a_char_start, b_segment_sequence, b_char_start;

-- name: InsertArticulation :exec
INSERT INTO evaluation.articulation
    (id, session_id, mode, candidate_id, tenant_id, status, warnings, analysis,
     calculation_version, policy_version, input_digest)
VALUES (sqlc.arg(id)::uuid, sqlc.arg(session_id)::uuid, sqlc.arg(mode)::text,
        sqlc.arg(candidate_id)::uuid, nullif(sqlc.arg(tenant_id)::text, '')::uuid,
        sqlc.arg(status)::text, sqlc.arg(warnings)::jsonb, sqlc.arg(analysis)::jsonb,
        sqlc.arg(calculation_version)::text, sqlc.arg(policy_version)::text,
        sqlc.arg(input_digest)::text);

-- name: GetArticulation :one
SELECT id::text AS id, session_id::text AS session_id, status, warnings, analysis,
       calculation_version, policy_version, input_digest, created_at
FROM evaluation.articulation
WHERE session_id = sqlc.arg(session_id)::uuid;
