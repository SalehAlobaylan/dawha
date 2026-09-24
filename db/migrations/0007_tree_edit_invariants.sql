ALTER TABLE tree_nodes
  ADD CONSTRAINT tree_nodes_version_id_id_key UNIQUE (tree_version_id, id);

ALTER TABLE tree_relationships
  ADD CONSTRAINT tree_relationships_subject_version_fkey
    FOREIGN KEY (tree_version_id, subject_node_id)
    REFERENCES tree_nodes (tree_version_id, id)
    ON DELETE CASCADE;

ALTER TABLE tree_relationships
  ADD CONSTRAINT tree_relationships_object_version_fkey
    FOREIGN KEY (tree_version_id, object_node_id)
    REFERENCES tree_nodes (tree_version_id, id)
    ON DELETE CASCADE;

CREATE UNIQUE INDEX tree_versions_one_draft_idx
  ON tree_versions (tree_id)
  WHERE state = 'draft';
