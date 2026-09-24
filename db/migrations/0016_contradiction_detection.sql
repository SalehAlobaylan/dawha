CREATE TABLE contradiction_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  requested_by uuid NOT NULL REFERENCES users(id),
  job_id uuid REFERENCES jobs(id),
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  algorithm_version text NOT NULL,
  finding_count integer NOT NULL DEFAULT 0,
  error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  completed_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX contradiction_runs_status_idx ON contradiction_runs (status, created_at DESC);
CREATE INDEX contradiction_runs_actor_idx ON contradiction_runs (requested_by, created_at DESC);

ALTER TABLE platform_findings
  ADD COLUMN run_id uuid REFERENCES contradiction_runs(id) ON DELETE SET NULL,
  ADD COLUMN check_key text,
  ADD COLUMN created_by uuid REFERENCES users(id),
  ADD COLUMN reviewed_by uuid REFERENCES users(id),
  ADD COLUMN reviewed_at timestamptz,
  ADD COLUMN review_note_ar text;

CREATE UNIQUE INDEX platform_findings_run_check_idx ON platform_findings (run_id, check_key) WHERE run_id IS NOT NULL AND check_key IS NOT NULL;
CREATE INDEX platform_findings_run_idx ON platform_findings (run_id, status, created_at DESC);
CREATE INDEX platform_findings_review_idx ON platform_findings (status, updated_at DESC);

CREATE TABLE platform_finding_reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  finding_id uuid NOT NULL REFERENCES platform_findings(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id),
  decision text NOT NULL CHECK (decision IN ('dismiss', 'confirm', 'investigate', 'reopen')),
  note_ar text,
  question_id uuid REFERENCES open_questions(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX platform_finding_reviews_finding_idx ON platform_finding_reviews (finding_id, created_at DESC);
