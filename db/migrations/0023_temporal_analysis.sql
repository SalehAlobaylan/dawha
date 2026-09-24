CREATE TABLE temporal_analysis_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  requested_by uuid NOT NULL REFERENCES users(id),
  question_id uuid REFERENCES open_questions(id) ON DELETE SET NULL,
  tree_id uuid NOT NULL REFERENCES trees(id) ON DELETE CASCADE,
  tree_version_id uuid NOT NULL REFERENCES tree_versions(id) ON DELETE CASCADE,
  target_person_id uuid NOT NULL REFERENCES people(id),
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  report_status text CHECK (report_status IN ('succeeded', 'insufficient_reference', 'failed')),
  execution_mode text NOT NULL DEFAULT 'synchronous' CHECK (execution_mode IN ('synchronous')),
  algorithm_version text NOT NULL,
  qualification_policy_version text NOT NULL,
  min_reference_size integer NOT NULL CHECK (min_reference_size BETWEEN 3 AND 100),
  reference_population jsonb NOT NULL DEFAULT '{}'::jsonb,
  finding_count integer NOT NULL DEFAULT 0 CHECK (finding_count >= 0),
  error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  completed_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (jsonb_typeof(reference_population) = 'object'),
  CHECK (octet_length(reference_population::text) <= 32768),
  CHECK (algorithm_version = 'temporal-statistics-v1'),
  CHECK (qualification_policy_version = 'qualified-v1'),
  CHECK (error IS NULL OR char_length(error) <= 4000)
);

CREATE INDEX temporal_analysis_runs_status_idx
  ON temporal_analysis_runs (status, created_at DESC);

CREATE INDEX temporal_analysis_runs_actor_idx
  ON temporal_analysis_runs (requested_by, created_at DESC);

CREATE INDEX temporal_analysis_runs_scope_idx
  ON temporal_analysis_runs (tree_version_id, created_at DESC);

ALTER TABLE platform_findings
  ADD COLUMN IF NOT EXISTS temporal_run_id uuid REFERENCES temporal_analysis_runs(id) ON DELETE CASCADE;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'platform_findings_single_run_check') THEN
    ALTER TABLE platform_findings
      ADD CONSTRAINT platform_findings_single_run_check
      CHECK (run_id IS NULL OR temporal_run_id IS NULL);
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS platform_findings_temporal_run_check_idx
  ON platform_findings (temporal_run_id, check_key)
  WHERE temporal_run_id IS NOT NULL AND check_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS platform_findings_temporal_run_idx
  ON platform_findings (temporal_run_id, status, created_at DESC);
