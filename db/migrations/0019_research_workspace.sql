CREATE TABLE research_run_contexts (
  run_id uuid NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
  scope_type text NOT NULL CHECK (scope_type IN ('question', 'person', 'family', 'branch', 'tree', 'tree_version', 'source', 'place', 'claim')),
  scope_id uuid NOT NULL,
  role text NOT NULL DEFAULT 'subject' CHECK (role IN ('subject', 'question', 'filter')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id, scope_type, scope_id, role)
);

CREATE INDEX research_run_contexts_scope_idx ON research_run_contexts (scope_type, scope_id, created_at DESC);
