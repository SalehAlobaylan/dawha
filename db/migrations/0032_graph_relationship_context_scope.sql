ALTER TABLE research_run_contexts
  DROP CONSTRAINT IF EXISTS research_run_contexts_scope_type_check;

ALTER TABLE research_run_contexts
  ADD CONSTRAINT research_run_contexts_scope_type_check
  CHECK (scope_type IN ('question', 'person', 'family', 'branch', 'tree', 'tree_version', 'source', 'place', 'claim', 'relationship'));
