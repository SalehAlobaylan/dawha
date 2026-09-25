ALTER TABLE research_runs
  DROP CONSTRAINT IF EXISTS research_runs_graph_operation_check;

ALTER TABLE research_runs
  ADD CONSTRAINT research_runs_graph_operation_check
  CHECK (graph_operation IS NULL OR graph_operation IN ('common_ancestor_path', 'evidence_connection', 'branch_claims', 'source_entities', 'geographic_path', 'shortest_relationship_path', 'connected_component', 'relationship_impact'));

ALTER TABLE research_graph_paths
  DROP CONSTRAINT IF EXISTS research_graph_paths_operation_check;

ALTER TABLE research_graph_paths
  ADD CONSTRAINT research_graph_paths_operation_check
  CHECK (operation IN ('common_ancestor_path', 'evidence_connection', 'branch_claims', 'source_entities', 'geographic_path', 'shortest_relationship_path', 'connected_component', 'relationship_impact'));

CREATE INDEX IF NOT EXISTS tree_relationships_parent_subject_idx
  ON tree_relationships (tree_version_id, subject_node_id, id)
  WHERE predicate = 'parent_of';
