CREATE TABLE atlas_claim_receipts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    occurrence_id uuid NOT NULL REFERENCES atlas_source_occurrences(id) ON DELETE CASCADE,
    claim_id uuid NOT NULL REFERENCES claims(id) ON DELETE CASCADE,
    evidence_id uuid NOT NULL REFERENCES claim_evidence(id) ON DELETE CASCADE,
    UNIQUE (claim_id, occurrence_id),
    UNIQUE (evidence_id)
);
