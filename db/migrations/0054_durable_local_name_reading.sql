-- Machine preparation is checkpointed independently of human decisions. These
-- tables contain hypotheses and private artifacts, never evaluation truth.
CREATE TABLE source_name_reading_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_file_id uuid NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  requested_by uuid REFERENCES users(id) ON DELETE SET NULL,
  source_checksum text NOT NULL,
  config_hash text NOT NULL CHECK (length(config_hash)=64),
  config jsonb NOT NULL CHECK (jsonb_typeof(config)='object'),
  suggestion_id uuid REFERENCES source_tree_suggestions(id) ON DELETE CASCADE,
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','running','partial','succeeded','failed','cancelled')),
  stage text NOT NULL DEFAULT 'queued' CHECK (stage IN ('queued','preparing','detecting','reading','complete')),
  job_id uuid REFERENCES jobs(id) ON DELETE SET NULL,
  generation integer NOT NULL DEFAULT 1 CHECK (generation>0),
  cancel_requested boolean NOT NULL DEFAULT false,
  page_count integer NOT NULL DEFAULT 0,
  detected integer NOT NULL DEFAULT 0,
  named integer NOT NULL DEFAULT 0,
  unreadable integer NOT NULL DEFAULT 0,
  unavailable integer NOT NULL DEFAULT 0,
  cache_hits integer NOT NULL DEFAULT 0,
  error_code text NOT NULL DEFAULT '',
  timings jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX source_name_reading_one_active ON source_name_reading_runs(source_file_id,config_hash) WHERE status IN ('queued','running');
CREATE INDEX source_name_reading_latest ON source_name_reading_runs(source_file_id,created_at DESC);

-- A frame is the original raster's native pixels or the PDF's actual analysis
-- resolution. It is never inferred from a UI preview or 300-DPI OCR evidence.
CREATE TABLE source_name_page_frames (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_file_id uuid NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
  source_checksum text NOT NULL,
  page_number integer NOT NULL CHECK (page_number>0),
  coordinate_space text NOT NULL CHECK (coordinate_space IN ('native_raster','pdf_analysis')),
  analysis_dpi integer NOT NULL CHECK (analysis_dpi IN (0,240)),
  width_px integer NOT NULL CHECK (width_px>0 AND width_px<=250000),
  height_px integer NOT NULL CHECK (height_px>0 AND height_px<=250000),
  page_count integer NOT NULL CHECK (page_count>0),
  renderer_version text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((coordinate_space='native_raster' AND analysis_dpi=0) OR (coordinate_space='pdf_analysis' AND analysis_dpi=240)),
  UNIQUE(source_file_id,source_checksum,page_number,renderer_version)
);
ALTER TABLE source_tree_suggestions ADD COLUMN name_reading_run_id uuid REFERENCES source_name_reading_runs(id) ON DELETE SET NULL;

CREATE TABLE source_name_reading_units (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES source_name_reading_runs(id) ON DELETE CASCADE,
  unit_key text NOT NULL,
  stage text NOT NULL CHECK (stage IN ('detect','read')),
  frame_id uuid NOT NULL REFERENCES source_name_page_frames(id) ON DELETE CASCADE,
  suggestion_id uuid REFERENCES source_tree_suggestions(id) ON DELETE CASCADE,
  payload jsonb NOT NULL CHECK (jsonb_typeof(payload)='object'),
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','succeeded','failed')),
  error_code text NOT NULL DEFAULT '',
  attempts integer NOT NULL DEFAULT 0,
  duration_ms bigint NOT NULL DEFAULT 0 CHECK (duration_ms>=0),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(run_id,stage,unit_key)
);
CREATE INDEX source_name_units_pending ON source_name_reading_units(run_id,stage,status);

CREATE TABLE source_name_crop_artifacts (
  fingerprint text PRIMARY KEY CHECK (length(fingerprint)=64),
  source_file_id uuid NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
  frame_id uuid NOT NULL REFERENCES source_name_page_frames(id) ON DELETE CASCADE,
  descriptor jsonb NOT NULL CHECK (jsonb_typeof(descriptor)='object'),
  tight_key text NOT NULL,
  context_key text NOT NULL,
  byte_size bigint NOT NULL CHECK (byte_size>0 AND byte_size<=20000000),
  expires_at timestamptz NOT NULL DEFAULT now()+interval '30 days',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX source_name_artifact_retention ON source_name_crop_artifacts(source_file_id,created_at);

CREATE TABLE source_name_readings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES source_name_reading_runs(id) ON DELETE CASCADE,
  unit_id uuid NOT NULL REFERENCES source_name_reading_units(id) ON DELETE CASCADE,
  suggestion_id uuid NOT NULL REFERENCES source_tree_suggestions(id) ON DELETE CASCADE,
  artifact_fingerprint text NOT NULL,
  attempt integer NOT NULL CHECK (attempt>0),
  reader_version text NOT NULL,
  preprocessing_version text NOT NULL,
  outcome text NOT NULL CHECK (outcome IN ('readable','unreadable','reading_unavailable','manual_review')),
  text_ar text NOT NULL DEFAULT '',
  alternatives jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(alternatives)='array'),
  variants jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(variants)='array'),
  quality jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(quality)='object'),
  measured_agreement double precision NOT NULL DEFAULT 0 CHECK (measured_agreement BETWEEN 0 AND 1),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(unit_id,attempt)
);
CREATE INDEX source_name_readings_history ON source_name_readings(suggestion_id,created_at DESC);
