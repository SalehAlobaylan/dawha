-- New evidence only. Historical decisions and queued runs retain an explicit
-- unknown geometry instead of reconstructing it from today's rectangle.
ALTER TABLE source_tree_suggestion_decisions
  ADD COLUMN geometry_revision integer CHECK (geometry_revision >= 0);

ALTER TABLE source_name_reading_runs
  ADD COLUMN target_geometry jsonb CHECK (
    target_geometry IS NULL OR
    (suggestion_id IS NOT NULL AND jsonb_typeof(target_geometry) = 'object')
  );
