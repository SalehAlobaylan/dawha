ALTER TABLE tree_forks
  DROP CONSTRAINT IF EXISTS tree_forks_tree_id_parent_version_id_key;

ALTER TABLE tree_forks
  ADD CONSTRAINT tree_forks_parent_version_forker_key
  UNIQUE (parent_tree_id, parent_version_id, forked_by);
