CREATE TABLE research_agent_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  requested_by uuid NOT NULL REFERENCES users(id),
  question_id uuid REFERENCES open_questions(id) ON DELETE SET NULL,
  query text NOT NULL,
  normalized_query text NOT NULL,
  entity_type text NOT NULL CHECK (entity_type IN ('person', 'family', 'branch', 'place', 'source')),
  entity_id uuid NOT NULL,
  tree_id uuid REFERENCES trees(id) ON DELETE RESTRICT,
  tree_version_id uuid REFERENCES tree_versions(id) ON DELETE RESTRICT,
  status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'failed')),
  resolution text NOT NULL DEFAULT 'unresolved' CHECK (resolution IN ('succeeded', 'unresolved')),
  execution_mode text NOT NULL DEFAULT 'synchronous' CHECK (execution_mode = 'synchronous'),
  planner_version text NOT NULL,
  algorithm_version text NOT NULL,
  qualification_policy_version text NOT NULL,
  report jsonb NOT NULL DEFAULT '{}'::jsonb,
  step_count integer NOT NULL DEFAULT 0 CHECK (step_count >= 0),
  evidence_count integer NOT NULL DEFAULT 0 CHECK (evidence_count >= 0),
  gap_count integer NOT NULL DEFAULT 0 CHECK (gap_count >= 0),
  recommendation_count integer NOT NULL DEFAULT 0 CHECK (recommendation_count >= 0),
  error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  completed_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((tree_id IS NULL AND tree_version_id IS NULL) OR (tree_id IS NOT NULL AND tree_version_id IS NOT NULL)),
  CHECK (jsonb_typeof(report) = 'object'),
  CHECK (octet_length(report::text) <= 131072),
  CHECK (planner_version = 'research-agent-v1'),
  CHECK (algorithm_version = 'research-agent-evidence-v1'),
  CHECK (qualification_policy_version = 'read-only-qualified-evidence-v1'),
  CHECK (error IS NULL OR char_length(error) <= 4000)
);

CREATE INDEX research_agent_runs_status_idx
  ON research_agent_runs (status, created_at DESC);

CREATE INDEX research_agent_runs_question_idx
  ON research_agent_runs (question_id, created_at DESC);

CREATE INDEX research_agent_runs_entity_idx
  ON research_agent_runs (entity_type, entity_id, created_at DESC);

CREATE TABLE research_agent_steps (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES research_agent_runs(id) ON DELETE CASCADE,
  step_order integer NOT NULL CHECK (step_order > 0),
  stage text NOT NULL CHECK (stage IN ('decompose_question', 'search_sources', 'search_graph', 'inspect_geography', 'inspect_chronology', 'compare_claims', 'inspect_source_dependency', 'retrieve_counter_evidence', 'generate_evidence_package', 'identify_missing_evidence', 'recommend_next_investigation')),
  tool_name text NOT NULL,
  status text NOT NULL CHECK (status IN ('succeeded', 'unresolved', 'failed', 'skipped')),
  input jsonb NOT NULL DEFAULT '{}'::jsonb,
  output jsonb NOT NULL DEFAULT '{}'::jsonb,
  evidence_count integer NOT NULL DEFAULT 0 CHECK (evidence_count >= 0),
  error text,
  started_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  UNIQUE (run_id, step_order)
);

CREATE INDEX research_agent_steps_run_idx
  ON research_agent_steps (run_id, step_order);

CREATE TABLE research_agent_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES research_agent_runs(id) ON DELETE CASCADE,
  step_id uuid NOT NULL REFERENCES research_agent_steps(id) ON DELETE CASCADE,
  layer text NOT NULL CHECK (layer IN ('source_statement', 'research_claim', 'tree_interpretation', 'platform_finding', 'geographic_signal', 'temporal_signal', 'source_dependency', 'graph_structure')),
  stance text NOT NULL CHECK (stance IN ('supports', 'counter_evidence', 'context', 'hypothesis')),
  reference_type text NOT NULL,
  reference_id uuid NOT NULL,
  source_id uuid REFERENCES sources(id) ON DELETE SET NULL,
  statement_id uuid REFERENCES source_statements(id) ON DELETE SET NULL,
  claim_id uuid REFERENCES claims(id) ON DELETE SET NULL,
  excerpt text NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (run_id, reference_type, reference_id, stance)
);

CREATE INDEX research_agent_evidence_run_idx
  ON research_agent_evidence (run_id, stance, created_at);

CREATE TABLE research_agent_gaps (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES research_agent_runs(id) ON DELETE CASCADE,
  kind text NOT NULL,
  description_ar text NOT NULL,
  severity text NOT NULL CHECK (severity IN ('low', 'medium', 'high')),
  status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'reviewed', 'dismissed')),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX research_agent_gaps_run_idx
  ON research_agent_gaps (run_id, severity, created_at);

CREATE TABLE research_agent_recommendations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES research_agent_runs(id) ON DELETE CASCADE,
  action text NOT NULL,
  rationale_ar text NOT NULL,
  priority text NOT NULL CHECK (priority IN ('low', 'normal', 'high')),
  status text NOT NULL DEFAULT 'suggested' CHECK (status IN ('suggested', 'accepted', 'dismissed')),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX research_agent_recommendations_run_idx
  ON research_agent_recommendations (run_id, priority, created_at);
