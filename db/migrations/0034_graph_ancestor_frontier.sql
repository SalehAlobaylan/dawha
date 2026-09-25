ALTER TABLE research_runs
  DROP CONSTRAINT IF EXISTS research_runs_graph_operation_check;

ALTER TABLE research_runs
  ADD CONSTRAINT research_runs_graph_operation_check
  CHECK (graph_operation IS NULL OR graph_operation IN ('common_ancestor_path', 'evidence_connection', 'branch_claims', 'source_entities', 'geographic_path', 'shortest_relationship_path', 'connected_component', 'relationship_impact', 'branch_structure_comparison', 'ancestor_frontier'));

ALTER TABLE research_graph_paths
  DROP CONSTRAINT IF EXISTS research_graph_paths_operation_check;

ALTER TABLE research_graph_paths
  ADD CONSTRAINT research_graph_paths_operation_check
  CHECK (operation IN ('common_ancestor_path', 'evidence_connection', 'branch_claims', 'source_entities', 'geographic_path', 'shortest_relationship_path', 'connected_component', 'relationship_impact', 'branch_structure_comparison', 'ancestor_frontier'));

CREATE INDEX IF NOT EXISTS tree_relationships_parent_object_idx
  ON tree_relationships (tree_version_id, object_node_id, subject_node_id, id)
  WHERE predicate = 'parent_of';

CREATE TABLE research_graph_ancestor_frontiers (
  run_id uuid PRIMARY KEY REFERENCES research_runs(id) ON DELETE CASCADE,
  path_id uuid NOT NULL UNIQUE REFERENCES research_graph_paths(id) ON DELETE CASCADE,
  tree_id uuid NOT NULL REFERENCES trees(id) ON DELETE CASCADE,
  tree_version_id uuid NOT NULL,
  root_node_id uuid NOT NULL,
  algorithm_version text NOT NULL,
  input_fingerprint text NOT NULL,
  limits jsonb NOT NULL,
  summary jsonb NOT NULL,
  status text NOT NULL,
  truncated boolean NOT NULL DEFAULT false,
  truncation_reasons jsonb NOT NULL DEFAULT '[]'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tree_id, tree_version_id) REFERENCES tree_versions(tree_id, id) ON DELETE CASCADE,
  FOREIGN KEY (tree_version_id, root_node_id) REFERENCES tree_nodes(tree_version_id, id) ON DELETE CASCADE,
  CHECK (char_length(algorithm_version) BETWEEN 1 AND 64),
  CHECK (char_length(input_fingerprint) BETWEEN 1 AND 128),
  CHECK (jsonb_typeof(limits) = 'object'),
  CHECK (jsonb_typeof(summary) = 'object'),
  CHECK (jsonb_typeof(truncation_reasons) = 'array'),
  CHECK (status IN ('structural', 'partial', 'contested')),
  CHECK (octet_length(limits::text) <= 8192),
  CHECK (octet_length(summary::text) <= 32768),
  CHECK (octet_length(truncation_reasons::text) <= 4096)
);

CREATE INDEX research_graph_ancestor_frontiers_scope_idx
  ON research_graph_ancestor_frontiers (tree_id, tree_version_id, created_at DESC);
