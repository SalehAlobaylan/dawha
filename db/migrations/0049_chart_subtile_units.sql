-- Dense model outputs get one resumable 2x2 re-read at the source resolution.
-- Child units have their own checkpoints and reviewer coverage entries.
ALTER TABLE source_chart_analysis_units
  DROP CONSTRAINT IF EXISTS source_chart_analysis_units_unit_kind_check;

ALTER TABLE source_chart_analysis_units
  ADD CONSTRAINT source_chart_analysis_units_unit_kind_check
  CHECK (unit_kind IN ('tile', 'subtile', 'reading_batch'));

