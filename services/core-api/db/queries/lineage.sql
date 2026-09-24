-- name: GetPublishedTree :one
SELECT id, name_ar, description_ar, visibility, owner_id, updated_at
FROM trees
WHERE id = $1 AND visibility = 'public';

-- name: ListOpenQuestions :many
SELECT id, title_ar, description_ar, status, priority, updated_at
FROM open_questions
WHERE status IN ('open', 'under_investigation', 'reopened')
ORDER BY CASE priority WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END, updated_at DESC;

-- name: GetClaimEvidence :many
SELECT ce.id, ce.relation, ce.evidence_note_ar, ss.id AS source_statement_id, ss.statement_text_ar, ss.locator_ar
FROM claim_evidence ce
LEFT JOIN source_statements ss ON ss.id = ce.source_statement_id
WHERE ce.claim_id = $1
ORDER BY ce.created_at;
