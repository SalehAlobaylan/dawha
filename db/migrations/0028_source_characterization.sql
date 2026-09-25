CREATE TABLE source_characterization_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  requested_by uuid NOT NULL REFERENCES users(id),
  question_id uuid REFERENCES open_questions(id) ON DELETE SET NULL,
  status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'failed')),
  report_status text CHECK (report_status IN ('succeeded', 'insufficient_evidence', 'failed')),
  review_status text NOT NULL DEFAULT 'needs_review' CHECK (review_status IN ('needs_review', 'confirmed', 'dismissed')),
  execution_mode text NOT NULL DEFAULT 'synchronous' CHECK (execution_mode = 'synchronous'),
  algorithm_version text NOT NULL,
  qualification_policy_version text NOT NULL,
  model_version text,
  input_fingerprint text NOT NULL,
  scope jsonb NOT NULL DEFAULT '{}'::jsonb,
  report jsonb NOT NULL DEFAULT '{}'::jsonb,
  finding_count integer NOT NULL DEFAULT 0 CHECK (finding_count >= 0),
  error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  started_at timestamptz,
  completed_at timestamptz,
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_id, input_fingerprint),
  CHECK (jsonb_typeof(scope) = 'object'),
  CHECK (jsonb_typeof(report) = 'object'),
  CHECK (octet_length(scope::text) <= 16384),
  CHECK (octet_length(report::text) <= 131072),
  CHECK (algorithm_version = 'source-characterization-v1'),
  CHECK (qualification_policy_version = 'source-characterization-qualified-v1'),
  CHECK (error IS NULL OR char_length(error) <= 4000)
);

CREATE INDEX source_characterization_runs_source_idx
  ON source_characterization_runs (source_id, created_at DESC);

CREATE INDEX source_characterization_runs_status_idx
  ON source_characterization_runs (status, created_at DESC);

CREATE TABLE source_characterization_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES source_characterization_runs(id) ON DELETE CASCADE,
  attribute_key text NOT NULL,
  relation text NOT NULL CHECK (relation IN ('describes', 'supports', 'qualifies', 'contradicts')),
  source_passage_id uuid REFERENCES source_passages(id) ON DELETE SET NULL,
  source_statement_id uuid REFERENCES source_statements(id) ON DELETE SET NULL,
  claim_id uuid REFERENCES claims(id) ON DELETE SET NULL,
  place_id uuid REFERENCES places(id) ON DELETE SET NULL,
  source_dependency_id uuid REFERENCES source_dependencies(id) ON DELETE SET NULL,
  related_source_id uuid REFERENCES sources(id) ON DELETE SET NULL,
  excerpt_ar text NOT NULL,
  position integer NOT NULL DEFAULT 0 CHECK (position >= 0),
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX source_characterization_evidence_run_idx
  ON source_characterization_evidence (run_id, attribute_key, position, created_at);

CREATE INDEX source_characterization_evidence_related_source_idx
  ON source_characterization_evidence (related_source_id, created_at DESC);

CREATE TABLE source_characterization_reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES source_characterization_runs(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id),
  decision text NOT NULL CHECK (decision IN ('confirm', 'dismiss', 'reopen')),
  note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (char_length(note_ar) IS NULL OR char_length(note_ar) <= 2000)
);

CREATE INDEX source_characterization_reviews_run_idx
  ON source_characterization_reviews (run_id, created_at DESC);
