ALTER TABLE research_runs
  ADD COLUMN IF NOT EXISTS graph_operation text,
  ADD COLUMN IF NOT EXISTS graph_max_depth integer,
  ADD COLUMN IF NOT EXISTS graph_truncated boolean;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'research_runs_graph_operation_check') THEN
    ALTER TABLE research_runs
      ADD CONSTRAINT research_runs_graph_operation_check
      CHECK (graph_operation IS NULL OR graph_operation IN ('common_ancestor_path', 'evidence_connection', 'branch_claims', 'source_entities', 'geographic_path'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'research_runs_graph_max_depth_check') THEN
    ALTER TABLE research_runs
      ADD CONSTRAINT research_runs_graph_max_depth_check
      CHECK (graph_max_depth IS NULL OR graph_max_depth BETWEEN 1 AND 3);
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS research_runs_graph_operation_idx
  ON research_runs (graph_operation, created_at DESC);

ALTER TABLE research_run_contexts
  DROP CONSTRAINT IF EXISTS research_run_contexts_role_check;

ALTER TABLE research_run_contexts
  ADD CONSTRAINT research_run_contexts_role_check
  CHECK (role IN ('subject', 'question', 'filter', 'graph_start', 'graph_end'));

CREATE TABLE IF NOT EXISTS research_graph_paths (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
  path_key text NOT NULL,
  operation text NOT NULL CHECK (operation IN ('common_ancestor_path', 'evidence_connection', 'branch_claims', 'source_entities', 'geographic_path')),
  status text NOT NULL CHECK (status IN ('complete', 'partial', 'contested', 'structural', 'evidence_backed', 'truncated', 'empty', 'not_found', 'timeout', 'error')),
  explanation text NOT NULL,
  depth integer NOT NULL CHECK (depth >= 0 AND depth <= 3),
  truncated boolean NOT NULL DEFAULT false,
  evidence_backed boolean NOT NULL DEFAULT false,
  structural_only boolean NOT NULL DEFAULT true,
  tree_id uuid REFERENCES trees(id) ON DELETE SET NULL,
  tree_version_id uuid REFERENCES tree_versions(id) ON DELETE SET NULL,
  tree_version_number integer,
  tree_version_state text,
  algorithm_version text NOT NULL,
  nodes jsonb NOT NULL DEFAULT '[]'::jsonb,
  node_provenance jsonb NOT NULL DEFAULT '[]'::jsonb,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (run_id, path_key),
  CHECK (char_length(path_key) BETWEEN 1 AND 128),
  CHECK (char_length(explanation) <= 2000),
  CHECK (tree_version_state IS NULL OR char_length(tree_version_state) <= 64),
  CHECK (char_length(algorithm_version) BETWEEN 1 AND 64),
  CHECK (jsonb_typeof(nodes) = 'array'),
  CHECK (jsonb_typeof(node_provenance) = 'array'),
  CHECK (jsonb_typeof(metadata) = 'object'),
  CHECK (octet_length(nodes::text) <= 131072),
  CHECK (octet_length(node_provenance::text) <= 131072),
  CHECK (octet_length(metadata::text) <= 8192)
);

ALTER TABLE research_graph_paths
  ADD COLUMN IF NOT EXISTS tree_version_state text;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'research_graph_paths_tree_version_state_check') THEN
    ALTER TABLE research_graph_paths
      ADD CONSTRAINT research_graph_paths_tree_version_state_check
      CHECK (tree_version_state IS NULL OR char_length(tree_version_state) <= 64);
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS research_graph_paths_run_idx
  ON research_graph_paths (run_id, created_at, id);
CREATE INDEX IF NOT EXISTS research_graph_paths_operation_idx
  ON research_graph_paths (operation, created_at DESC);
CREATE INDEX IF NOT EXISTS research_graph_paths_tree_idx
  ON research_graph_paths (tree_id, tree_version_id, created_at DESC);

CREATE TABLE IF NOT EXISTS research_graph_edges (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
  path_id uuid NOT NULL REFERENCES research_graph_paths(id) ON DELETE CASCADE,
  ordinal integer NOT NULL CHECK (ordinal BETWEEN 1 AND 200),
  edge_reference_id uuid,
  edge_type text NOT NULL,
  from_node_id uuid NOT NULL,
  to_node_id uuid NOT NULL,
  predicate text,
  status text,
  certainty text,
  source_id uuid REFERENCES sources(id) ON DELETE SET NULL,
  claim_id uuid REFERENCES claims(id) ON DELETE SET NULL,
  statement_id uuid REFERENCES source_statements(id) ON DELETE SET NULL,
  passage_id uuid REFERENCES source_passages(id) ON DELETE SET NULL,
  tree_relationship_id uuid REFERENCES tree_relationships(id) ON DELETE SET NULL,
  migration_event_id uuid REFERENCES migration_events(id) ON DELETE SET NULL,
  from_place_id uuid REFERENCES places(id) ON DELETE SET NULL,
  to_place_id uuid REFERENCES places(id) ON DELETE SET NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (path_id, ordinal),
  CHECK (char_length(edge_type) BETWEEN 1 AND 64),
  CHECK (char_length(predicate) <= 128),
  CHECK (char_length(status) <= 64),
  CHECK (char_length(certainty) <= 64),
  CHECK (jsonb_typeof(metadata) = 'object'),
  CHECK (octet_length(metadata::text) <= 8192)
);

CREATE INDEX IF NOT EXISTS research_graph_edges_run_idx
  ON research_graph_edges (run_id, path_id, ordinal);
CREATE INDEX IF NOT EXISTS research_graph_edges_source_idx
  ON research_graph_edges (source_id, created_at DESC);
CREATE INDEX IF NOT EXISTS research_graph_edges_claim_idx
  ON research_graph_edges (claim_id, created_at DESC);

CREATE TABLE IF NOT EXISTS research_graph_path_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
  path_id uuid NOT NULL REFERENCES research_graph_paths(id) ON DELETE CASCADE,
  ordinal integer NOT NULL CHECK (ordinal BETWEEN 1 AND 200),
  reference_id uuid NOT NULL,
  reference_type text NOT NULL,
  relation text,
  source_id uuid REFERENCES sources(id) ON DELETE SET NULL,
  claim_id uuid REFERENCES claims(id) ON DELETE SET NULL,
  statement_id uuid REFERENCES source_statements(id) ON DELETE SET NULL,
  passage_id uuid REFERENCES source_passages(id) ON DELETE SET NULL,
  review_status text,
  status text,
  certainty text,
  title_ar text,
  excerpt_ar text,
  locator_ar text,
  page_number integer,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (path_id, ordinal),
  UNIQUE (path_id, reference_type, reference_id, relation),
  CHECK (char_length(reference_type) BETWEEN 1 AND 64),
  CHECK (char_length(relation) <= 64),
  CHECK (char_length(review_status) <= 64),
  CHECK (char_length(status) <= 64),
  CHECK (char_length(certainty) <= 64),
  CHECK (char_length(title_ar) <= 1000),
  CHECK (char_length(excerpt_ar) <= 4000),
  CHECK (char_length(locator_ar) <= 1000),
  CHECK (jsonb_typeof(metadata) = 'object'),
  CHECK (octet_length(metadata::text) <= 8192)
);

CREATE INDEX IF NOT EXISTS research_graph_path_evidence_run_idx
  ON research_graph_path_evidence (run_id, path_id, ordinal);
CREATE INDEX IF NOT EXISTS research_graph_path_evidence_source_idx
  ON research_graph_path_evidence (source_id, created_at DESC);
CREATE INDEX IF NOT EXISTS research_graph_path_evidence_claim_idx
  ON research_graph_path_evidence (claim_id, created_at DESC);
