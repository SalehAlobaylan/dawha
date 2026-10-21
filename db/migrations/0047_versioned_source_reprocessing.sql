-- Processing attempts are immutable versions. A corrected human transcription
-- can be re-extracted without replacing the original OCR or its review history.
ALTER TABLE source_processing_runs
  DROP CONSTRAINT IF EXISTS source_processing_runs_source_file_id_key,
  ADD COLUMN run_kind text NOT NULL DEFAULT 'document'
    CHECK (run_kind IN ('document', 'reviewed_page')),
  ADD COLUMN pipeline_config jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(pipeline_config) = 'object'),
  ADD COLUMN triggered_by uuid REFERENCES users(id),
  ADD COLUMN input_page_review_revision_id uuid REFERENCES source_page_review_revisions(id);

ALTER TABLE source_passages
  ADD COLUMN source_processing_run_id uuid REFERENCES source_processing_runs(id) ON DELETE SET NULL,
  ADD COLUMN source_page_evidence_id uuid REFERENCES source_page_evidence(id) ON DELETE SET NULL,
  ADD COLUMN source_page_review_revision_id uuid REFERENCES source_page_review_revisions(id) ON DELETE SET NULL;

UPDATE source_passages p
SET source_processing_run_id = r.id
FROM source_processing_runs r
WHERE p.source_file_id = r.source_file_id
  AND p.source_processing_run_id IS NULL;

ALTER TABLE source_candidates
  ADD COLUMN source_processing_run_id uuid REFERENCES source_processing_runs(id) ON DELETE SET NULL;

UPDATE source_candidates c
SET source_processing_run_id = p.source_processing_run_id
FROM source_passages p
WHERE p.id = c.source_passage_id
  AND c.source_processing_run_id IS NULL;

ALTER TABLE source_statements
  ADD COLUMN source_processing_run_id uuid REFERENCES source_processing_runs(id) ON DELETE SET NULL;

UPDATE source_statements ss
SET source_processing_run_id = c.source_processing_run_id
FROM source_candidates c
WHERE c.source_statement_id = ss.id
  AND c.source_processing_run_id IS NOT NULL
  AND ss.source_processing_run_id IS NULL;

ALTER TABLE source_statements
  DROP CONSTRAINT IF EXISTS source_statements_review_status_check,
  ADD CONSTRAINT source_statements_review_status_check CHECK (review_status IN ('unreviewed', 'accepted', 'rejected', 'needs_review', 'superseded'));

ALTER TABLE source_candidates
  DROP CONSTRAINT IF EXISTS source_candidates_source_file_id_dedupe_key_key;

ALTER TABLE source_candidates
  DROP CONSTRAINT IF EXISTS source_candidates_status_check,
  ADD CONSTRAINT source_candidates_status_check CHECK (status IN ('unreviewed', 'accepted', 'rejected', 'needs_review', 'superseded'));

CREATE UNIQUE INDEX source_candidates_run_dedupe_idx
  ON source_candidates (source_file_id, source_processing_run_id, dedupe_key)
  WHERE source_processing_run_id IS NOT NULL;
CREATE UNIQUE INDEX source_candidates_legacy_dedupe_idx
  ON source_candidates (source_file_id, dedupe_key)
  WHERE source_processing_run_id IS NULL;

CREATE INDEX source_passages_processing_run_idx
  ON source_passages (source_processing_run_id, sequence_number);
CREATE INDEX source_passages_page_evidence_idx
  ON source_passages (source_page_evidence_id, source_page_review_revision_id);
CREATE INDEX source_candidates_processing_run_idx
  ON source_candidates (source_processing_run_id, created_at DESC);
CREATE INDEX source_processing_runs_file_idx
  ON source_processing_runs (source_file_id, created_at DESC, id DESC);
CREATE INDEX source_processing_runs_reviewed_revision_idx
  ON source_processing_runs (source_file_id, input_page_review_revision_id)
  WHERE run_kind = 'reviewed_page';

ALTER TABLE source_tree_drafts
  ADD COLUMN source_processing_run_id uuid REFERENCES source_processing_runs(id) ON DELETE SET NULL;

UPDATE source_tree_drafts d
SET source_processing_run_id = (
  SELECT p.run_id FROM source_tree_draft_revisions rev
  CROSS JOIN LATERAL jsonb_array_elements(rev.payload->'pages') page
  JOIN source_page_evidence p ON p.id = (page->>'pageEvidenceId')::uuid
  WHERE rev.draft_id = d.id
  ORDER BY rev.revision_number DESC, p.created_at DESC
  LIMIT 1
)
WHERE d.source_processing_run_id IS NULL;

DROP INDEX IF EXISTS source_tree_drafts_one_open_per_file_idx;
CREATE UNIQUE INDEX source_tree_drafts_one_open_per_run_idx
  ON source_tree_drafts (source_file_id, source_processing_run_id)
  WHERE state = 'draft' AND source_processing_run_id IS NOT NULL;
CREATE UNIQUE INDEX source_tree_drafts_legacy_open_per_file_idx
  ON source_tree_drafts (source_file_id)
  WHERE state = 'draft' AND source_processing_run_id IS NULL;
