ALTER TABLE temporal_analysis_runs
  DROP CONSTRAINT IF EXISTS temporal_analysis_runs_tree_id_fkey,
  DROP CONSTRAINT IF EXISTS temporal_analysis_runs_tree_version_id_fkey;

ALTER TABLE temporal_analysis_runs
  ADD CONSTRAINT temporal_analysis_runs_tree_id_fkey
    FOREIGN KEY (tree_id) REFERENCES trees(id) ON DELETE RESTRICT,
  ADD CONSTRAINT temporal_analysis_runs_tree_version_id_fkey
    FOREIGN KEY (tree_version_id) REFERENCES tree_versions(id) ON DELETE RESTRICT;
