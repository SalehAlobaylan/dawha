-- The public-membership predicates added in plan 001 read these joins on every
-- anonymous dictionary, search and claim read. They are supporting indexes only:
-- no column, no constraint and no published-tree semantics change, so no backfill
-- query is required.
CREATE INDEX IF NOT EXISTS tree_nodes_person_idx
  ON tree_nodes (person_id);

CREATE INDEX IF NOT EXISTS trees_visibility_idx
  ON trees (visibility);

CREATE INDEX IF NOT EXISTS claim_counter_evidence_claim_idx
  ON claim_counter_evidence (claim_id);
