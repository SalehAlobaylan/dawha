ALTER TABLE research_runs
  DROP CONSTRAINT IF EXISTS research_runs_graph_operation_check;

ALTER TABLE research_runs
  ADD CONSTRAINT research_runs_graph_operation_check
  CHECK (graph_operation IS NULL OR graph_operation IN ('common_ancestor_path', 'evidence_connection', 'branch_claims', 'source_entities', 'geographic_path', 'shortest_relationship_path', 'connected_component', 'relationship_impact', 'branch_structure_comparison', 'ancestor_frontier', 'source_dependency_neighborhood'));

ALTER TABLE research_graph_paths
  DROP CONSTRAINT IF EXISTS research_graph_paths_operation_check;

ALTER TABLE research_graph_paths
  ADD CONSTRAINT research_graph_paths_operation_check
  CHECK (operation IN ('common_ancestor_path', 'evidence_connection', 'branch_claims', 'source_entities', 'geographic_path', 'shortest_relationship_path', 'connected_component', 'relationship_impact', 'branch_structure_comparison', 'ancestor_frontier', 'source_dependency_neighborhood'));

CREATE INDEX IF NOT EXISTS source_dependencies_active_source_idx
  ON source_dependencies (source_id, depends_on_source_id, id)
  WHERE status IN ('needs_review', 'confirmed');

CREATE TABLE research_graph_source_dependency_neighborhoods (
  run_id uuid PRIMARY KEY REFERENCES research_runs(id) ON DELETE CASCADE,
  path_id uuid NOT NULL UNIQUE REFERENCES research_graph_paths(id) ON DELETE CASCADE,
  root_source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  algorithm_version text NOT NULL,
  input_fingerprint text NOT NULL,
  edge_set_fingerprint text NOT NULL,
  limits jsonb NOT NULL,
  summary jsonb NOT NULL,
  status text NOT NULL,
  truncated boolean NOT NULL DEFAULT false,
  truncation_reasons jsonb NOT NULL DEFAULT '[]'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (char_length(algorithm_version) BETWEEN 1 AND 64),
  CHECK (char_length(input_fingerprint) BETWEEN 1 AND 128),
  CHECK (char_length(edge_set_fingerprint) BETWEEN 1 AND 128),
  CHECK (jsonb_typeof(limits) = 'object'),
  CHECK (jsonb_typeof(summary) = 'object'),
  CHECK (jsonb_typeof(truncation_reasons) = 'array'),
  CHECK (status IN ('structural', 'partial', 'contested')),
  CHECK (octet_length(limits::text) <= 8192),
  CHECK (octet_length(summary::text) <= 32768),
  CHECK (octet_length(truncation_reasons::text) <= 4096)
);

CREATE INDEX research_graph_source_dependency_neighborhoods_root_idx
  ON research_graph_source_dependency_neighborhoods (root_source_id, created_at DESC);
