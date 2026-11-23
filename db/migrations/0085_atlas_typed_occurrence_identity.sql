ALTER TABLE atlas_source_occurrences
    ADD COLUMN place_id uuid REFERENCES places(id) ON DELETE SET NULL,
    ADD COLUMN branch_id uuid REFERENCES branches(id) ON DELETE SET NULL,
    ADD CHECK (num_nonnulls(tribe_id, place_id, branch_id) <= 1);
ALTER TABLE atlas_occurrence_identity_decisions
    ADD COLUMN place_id uuid REFERENCES places(id) ON DELETE SET NULL,
    ADD COLUMN branch_id uuid REFERENCES branches(id) ON DELETE SET NULL,
    ADD CHECK (num_nonnulls(tribe_id, place_id, branch_id) <= 1);
