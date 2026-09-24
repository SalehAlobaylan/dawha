ALTER TABLE research_graph_edges
  ADD COLUMN IF NOT EXISTS path_from_node_id uuid,
  ADD COLUMN IF NOT EXISTS path_to_node_id uuid;

ALTER TABLE research_graph_path_evidence
  DROP CONSTRAINT IF EXISTS research_graph_path_evidence_path_id_reference_type_referen_key;

CREATE UNIQUE INDEX IF NOT EXISTS research_graph_path_evidence_claim_aware_idx
  ON research_graph_path_evidence (
    path_id,
    reference_type,
    reference_id,
    COALESCE(relation, ''),
    COALESCE(claim_id, '00000000-0000-0000-0000-000000000000'::uuid)
  );
