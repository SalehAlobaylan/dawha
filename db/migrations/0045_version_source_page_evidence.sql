-- Keep each processing attempt's readings immutable. Human transcription
-- revisions point to the exact page evidence row the reviewer saw.
ALTER TABLE source_page_evidence
  DROP CONSTRAINT IF EXISTS source_page_evidence_run_id_page_number_key;

DROP INDEX IF EXISTS source_page_evidence_file_idx;

CREATE INDEX source_page_evidence_latest_file_page_idx
  ON source_page_evidence (source_file_id, page_number, created_at DESC, id DESC);
