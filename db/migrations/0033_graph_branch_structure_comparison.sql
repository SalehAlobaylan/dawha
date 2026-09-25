ALTER TABLE research_runs
  DROP CONSTRAINT IF EXISTS research_runs_graph_operation_check;

ALTER TABLE research_runs
  ADD CONSTRAINT research_runs_graph_operation_check
  CHECK (graph_operation IS NULL OR graph_operation IN ('common_ancestor_path', 'evidence_connection', 'branch_claims', 'source_entities', 'geographic_path', 'shortest_relationship_path', 'connected_component', 'relationship_impact', 'branch_structure_comparison'));

ALTER TABLE research_graph_paths
  DROP CONSTRAINT IF EXISTS research_graph_paths_operation_check;

ALTER TABLE research_graph_paths
  ADD CONSTRAINT research_graph_paths_operation_check
  CHECK (operation IN ('common_ancestor_path', 'evidence_connection', 'branch_claims', 'source_entities', 'geographic_path', 'shortest_relationship_path', 'connected_component', 'relationship_impact', 'branch_structure_comparison'));

ALTER TABLE tree_versions
  ADD CONSTRAINT tree_versions_tree_id_id_key UNIQUE (tree_id, id);

ALTER TABLE tree_nodes
  ADD CONSTRAINT tree_nodes_tree_version_id_id_key UNIQUE (tree_version_id, id);

ALTER TABLE research_run_contexts
  DROP CONSTRAINT IF EXISTS research_run_contexts_scope_type_check;

ALTER TABLE research_run_contexts
  ADD CONSTRAINT research_run_contexts_scope_type_check
  CHECK (scope_type IN ('question', 'person', 'family', 'branch', 'tree', 'tree_version', 'source', 'place', 'claim', 'relationship', 'tree_node'));

CREATE TABLE research_graph_comparisons (
  run_id uuid PRIMARY KEY REFERENCES research_runs(id) ON DELETE CASCADE,
  from_path_id uuid NOT NULL UNIQUE REFERENCES research_graph_paths(id) ON DELETE CASCADE,
  to_path_id uuid NOT NULL UNIQUE REFERENCES research_graph_paths(id) ON DELETE CASCADE,
  from_tree_id uuid NOT NULL REFERENCES trees(id) ON DELETE CASCADE,
  from_tree_version_id uuid NOT NULL,
  from_root_node_id uuid NOT NULL,
  to_tree_id uuid NOT NULL REFERENCES trees(id) ON DELETE CASCADE,
  to_tree_version_id uuid NOT NULL,
  to_root_node_id uuid NOT NULL,
  algorithm_version text NOT NULL,
  input_fingerprint text NOT NULL,
  limits jsonb NOT NULL,
  from_summary jsonb NOT NULL,
  to_summary jsonb NOT NULL,
  delta jsonb NOT NULL,
  status text NOT NULL,
  truncated boolean NOT NULL DEFAULT false,
  truncation_reasons jsonb NOT NULL DEFAULT '[]'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (from_tree_id, from_tree_version_id) REFERENCES tree_versions(tree_id, id) ON DELETE CASCADE,
  FOREIGN KEY (from_tree_version_id, from_root_node_id) REFERENCES tree_nodes(tree_version_id, id) ON DELETE CASCADE,
  FOREIGN KEY (to_tree_id, to_tree_version_id) REFERENCES tree_versions(tree_id, id) ON DELETE CASCADE,
  FOREIGN KEY (to_tree_version_id, to_root_node_id) REFERENCES tree_nodes(tree_version_id, id) ON DELETE CASCADE,
  CHECK (char_length(algorithm_version) BETWEEN 1 AND 64),
  CHECK (char_length(input_fingerprint) BETWEEN 1 AND 128),
  CHECK (jsonb_typeof(limits) = 'object'),
  CHECK (jsonb_typeof(from_summary) = 'object'),
  CHECK (jsonb_typeof(to_summary) = 'object'),
  CHECK (jsonb_typeof(delta) = 'object'),
  CHECK (jsonb_typeof(truncation_reasons) = 'array'),
  CHECK (status IN ('structural', 'partial', 'contested')),
  CHECK (octet_length(limits::text) <= 8192),
  CHECK (octet_length(from_summary::text) <= 32768),
  CHECK (octet_length(to_summary::text) <= 32768),
  CHECK (octet_length(delta::text) <= 8192),
  CHECK (octet_length(truncation_reasons::text) <= 4096)
);

CREATE INDEX research_graph_comparisons_from_scope_idx
  ON research_graph_comparisons (from_tree_id, from_tree_version_id, created_at DESC);

CREATE INDEX research_graph_comparisons_to_scope_idx
  ON research_graph_comparisons (to_tree_id, to_tree_version_id, created_at DESC);
