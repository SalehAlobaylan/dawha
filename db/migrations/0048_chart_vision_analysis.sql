-- Family-chart processing has a separate evidence path. OCR output from a chart
-- must not flow into the ordinary machine-generated candidate review queue.
ALTER TABLE source_files
  ADD COLUMN document_kind text NOT NULL DEFAULT 'document'
    CHECK (document_kind IN ('document', 'family_chart'));

CREATE TABLE source_chart_analysis_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  source_file_id uuid NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
  created_by uuid NOT NULL REFERENCES users(id),
  job_id uuid REFERENCES jobs(id) ON DELETE SET NULL,
  source_checksum_sha256 text NOT NULL CHECK (source_checksum_sha256 ~ '^[0-9a-f]{64}$'),
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'partial', 'failed')),
  quality_status text NOT NULL DEFAULT 'unqualified'
    CHECK (quality_status IN ('unqualified', 'manual_required', 'qualified_for_assisted_review')),
  pipeline_config jsonb NOT NULL CHECK (jsonb_typeof(pipeline_config) = 'object'),
  model_version text NOT NULL,
  page_count integer NOT NULL DEFAULT 0 CHECK (page_count >= 0),
  provider_call_count integer NOT NULL DEFAULT 0 CHECK (provider_call_count >= 0),
  input_image_bytes bigint NOT NULL DEFAULT 0 CHECK (input_image_bytes >= 0),
  current_review_revision integer NOT NULL DEFAULT 0 CHECK (current_review_revision >= 0),
  error_code text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);

CREATE INDEX source_chart_analysis_file_idx
  ON source_chart_analysis_runs (source_file_id, created_at DESC, id DESC);
CREATE UNIQUE INDEX source_chart_analysis_one_active_file_idx
  ON source_chart_analysis_runs (source_file_id)
  WHERE status IN ('queued', 'running');

-- Units are independently checkpointed so a retry resumes at failed tiles or
-- reading batches without repeating completed vision calls.
CREATE TABLE source_chart_analysis_units (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES source_chart_analysis_runs(id) ON DELETE CASCADE,
  unit_key text NOT NULL CHECK (length(unit_key) BETWEEN 1 AND 120),
  unit_kind text NOT NULL CHECK (unit_kind IN ('tile', 'reading_batch')),
  page_number integer NOT NULL CHECK (page_number > 0),
  input_geometry jsonb NOT NULL CHECK (jsonb_typeof(input_geometry) = 'object'),
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
  output_payload jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(output_payload) = 'object'),
  error_code text,
  attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (run_id, unit_key)
);
CREATE INDEX source_chart_analysis_units_run_idx
  ON source_chart_analysis_units (run_id, page_number, unit_kind, unit_key);

-- Reviewer changes are append-only and optimistic-concurrency protected by
-- revision_number. Gold annotations for benchmark work are stored separately.
CREATE TABLE source_chart_analysis_review_revisions (
  run_id uuid NOT NULL REFERENCES source_chart_analysis_runs(id) ON DELETE CASCADE,
  revision_number integer NOT NULL CHECK (revision_number > 0),
  reviewer_id uuid NOT NULL REFERENCES users(id),
  payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id, revision_number)
);
