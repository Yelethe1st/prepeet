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
