-- Keep human transcription separate from machine readings. Review revisions
-- are append-only and remain bound to the exact physical-page evidence version.
CREATE TABLE source_page_review_revisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_page_evidence_id uuid NOT NULL REFERENCES source_page_evidence(id) ON DELETE CASCADE,
  revision_number integer NOT NULL CHECK (revision_number > 0),
  reviewer_id uuid NOT NULL REFERENCES users(id),
  status text NOT NULL CHECK (status IN ('transcribed', 'partial', 'unreadable')),
  transcription_text_ar text NOT NULL DEFAULT '',
  note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_page_evidence_id, revision_number),
  CHECK ((status = 'unreadable' AND transcription_text_ar = '') OR
         (status IN ('transcribed', 'partial') AND length(btrim(transcription_text_ar)) > 0))
);

CREATE INDEX source_page_review_revisions_latest_idx
  ON source_page_review_revisions (source_page_evidence_id, revision_number DESC);
