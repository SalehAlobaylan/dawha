-- Plan 030-E: the promotion lane's own provenance + idempotence.
--
-- The promoted historical region points BACK at the candidate that earned it,
-- and the candidate carries the promotion's answer (the once-only guard):
-- a second promote of the same candidate returns the SAME region id, never
-- a duplicate row.
--
-- Idempotent by the IF NOT EXISTS guards; safe to re-run.
ALTER TABLE historical_regions
  ADD COLUMN IF NOT EXISTS candidate_id uuid REFERENCES external_region_candidates(id);

ALTER TABLE external_region_candidates
  ADD COLUMN IF NOT EXISTS promoted_region_id uuid REFERENCES historical_regions(id);

-- The one promoted candidate may name one region — provenance rule 5.
ALTER TABLE external_region_candidates
  DROP CONSTRAINT IF EXISTS external_region_candidates_promoted_region_id_key;
ALTER TABLE external_region_candidates
  ADD CONSTRAINT external_region_candidates_promoted_region_id_key
  UNIQUE (promoted_region_id);
