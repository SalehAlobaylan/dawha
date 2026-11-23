ALTER TABLE atlas_source_item_revisions ADD COLUMN scan_sha256 text
    CHECK (scan_sha256 ~ '^[0-9a-f]{64}$');
ALTER TABLE atlas_source_occurrences ADD COLUMN source_selector jsonb
    CHECK (jsonb_typeof(source_selector) = 'object');
ALTER TABLE atlas_source_occurrences DROP CONSTRAINT atlas_source_occurrences_reading_method_check;
ALTER TABLE atlas_source_occurrences ADD CHECK
    (reading_method IN ('original','transliteration','catalogue','transcript','ocr'));

CREATE TABLE atlas_reading_proposals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_revision_id uuid NOT NULL REFERENCES atlas_source_item_revisions(id) ON DELETE CASCADE,
    proposal_key text NOT NULL,
    raw jsonb NOT NULL CHECK (jsonb_typeof(raw) = 'object'),
    method text NOT NULL,
    engine text NOT NULL,
    frame jsonb NOT NULL CHECK (jsonb_typeof(frame) = 'object'),
    imported_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at timestamptz,
    decision text CHECK (decision IN ('publish','reject')),
    note_ar text,
    occurrence_id uuid REFERENCES atlas_source_occurrences(id) ON DELETE SET NULL,
    UNIQUE (item_revision_id, proposal_key)
);
CREATE INDEX atlas_reading_proposals_pending_idx ON atlas_reading_proposals (id) WHERE decision IS NULL;
