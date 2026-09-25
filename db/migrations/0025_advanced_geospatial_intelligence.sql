CREATE TABLE geospatial_intelligence_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  requested_by uuid NOT NULL REFERENCES users(id),
  question_id uuid REFERENCES open_questions(id) ON DELETE SET NULL,
  entity_type text NOT NULL CHECK (entity_type IN ('person', 'source', 'place')),
  entity_id uuid NOT NULL,
  tree_id uuid REFERENCES trees(id) ON DELETE RESTRICT,
  tree_version_id uuid REFERENCES tree_versions(id) ON DELETE RESTRICT,
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  report_status text CHECK (report_status IN ('succeeded', 'insufficient_evidence', 'failed')),
  execution_mode text NOT NULL DEFAULT 'synchronous' CHECK (execution_mode = 'synchronous'),
  algorithm_version text NOT NULL,
  qualification_policy_version text NOT NULL,
  scope jsonb NOT NULL DEFAULT '{}'::jsonb,
  report jsonb NOT NULL DEFAULT '{}'::jsonb,
  finding_count integer NOT NULL DEFAULT 0 CHECK (finding_count >= 0),
  error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  completed_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (jsonb_typeof(scope) = 'object'),
  CHECK (jsonb_typeof(report) = 'object'),
  CHECK (octet_length(scope::text) <= 16384),
  CHECK (octet_length(report::text) <= 131072),
  CHECK ((tree_id IS NULL AND tree_version_id IS NULL) OR (tree_id IS NOT NULL AND tree_version_id IS NOT NULL)),
  CHECK (algorithm_version = 'geospatial-intelligence-v1'),
  CHECK (qualification_policy_version = 'qualified-geography-v1'),
  CHECK (error IS NULL OR char_length(error) <= 4000)
);

CREATE INDEX geospatial_intelligence_runs_status_idx
  ON geospatial_intelligence_runs (status, created_at DESC);

CREATE INDEX geospatial_intelligence_runs_entity_idx
  ON geospatial_intelligence_runs (entity_type, entity_id, created_at DESC);

CREATE INDEX geospatial_intelligence_runs_scope_idx
  ON geospatial_intelligence_runs (tree_version_id, created_at DESC);

ALTER TABLE platform_findings
  ADD COLUMN IF NOT EXISTS geospatial_run_id uuid REFERENCES geospatial_intelligence_runs(id) ON DELETE RESTRICT;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'platform_findings_single_analysis_run_check') THEN
    ALTER TABLE platform_findings
      ADD CONSTRAINT platform_findings_single_analysis_run_check
      CHECK (num_nonnulls(run_id, temporal_run_id, geospatial_run_id) <= 1);
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS platform_findings_geospatial_run_check_idx
  ON platform_findings (geospatial_run_id, check_key)
  WHERE geospatial_run_id IS NOT NULL AND check_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS platform_findings_geospatial_run_idx
  ON platform_findings (geospatial_run_id, status, created_at DESC);
