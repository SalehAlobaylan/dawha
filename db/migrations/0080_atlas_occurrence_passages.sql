ALTER TABLE atlas_source_occurrences
    ADD COLUMN source_passage_id uuid REFERENCES source_passages(id) ON DELETE SET NULL;
