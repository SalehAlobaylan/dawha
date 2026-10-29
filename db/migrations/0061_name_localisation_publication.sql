-- Detection is published as a complete file snapshot. Failed/cancelled scans
-- keep the previous queue; raw machine proposals and human decisions survive.
ALTER TABLE source_tree_suggestions
  ADD COLUMN localisation_pending boolean NOT NULL DEFAULT false;

ALTER TABLE source_name_review_events
  DROP CONSTRAINT source_name_review_events_event_type_check;
ALTER TABLE source_name_review_events
  ADD CONSTRAINT source_name_review_events_event_type_check
  CHECK (event_type IN ('reopened','decision','region_revised','split','merge','redetected'));

CREATE INDEX source_tree_suggestions_published_pending_idx
  ON source_tree_suggestions(source_file_id,created_at,id)
  WHERE status='suggested' AND superseded_at IS NULL AND NOT localisation_pending;
