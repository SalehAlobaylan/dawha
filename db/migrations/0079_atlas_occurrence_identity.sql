CREATE TABLE atlas_occurrence_identity_decisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    decision_order bigint GENERATED ALWAYS AS IDENTITY,
    occurrence_id uuid NOT NULL REFERENCES atlas_source_occurrences(id) ON DELETE CASCADE,
    reviewer_id uuid NOT NULL REFERENCES users(id),
    tribe_id uuid REFERENCES tribes(id) ON DELETE SET NULL,
    note_ar text NOT NULL CHECK (length(note_ar) BETWEEN 1 AND 1000),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX atlas_occurrence_identity_history
    ON atlas_occurrence_identity_decisions (occurrence_id, decision_order DESC);
