CREATE TABLE research_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  question_id uuid REFERENCES open_questions(id) ON DELETE SET NULL,
  actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
  query text NOT NULL,
  normalized_query text NOT NULL,
  query_type text,
  status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'failed')),
  insufficient_evidence boolean NOT NULL DEFAULT false,
  model_version text,
  error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX research_runs_question_idx ON research_runs (question_id, created_at DESC);
CREATE INDEX research_runs_status_idx ON research_runs (status, created_at DESC);

CREATE TABLE research_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES research_runs(id) ON DELETE CASCADE,
  layer text NOT NULL CHECK (layer IN ('source_statement', 'research_claim', 'tree_interpretation', 'platform_finding', 'open_question')),
  reference_type text NOT NULL,
  reference_id uuid NOT NULL,
  source_id uuid REFERENCES sources(id) ON DELETE SET NULL,
  passage_id uuid REFERENCES source_passages(id) ON DELETE SET NULL,
  statement_id uuid REFERENCES source_statements(id) ON DELETE SET NULL,
  claim_id uuid REFERENCES claims(id) ON DELETE SET NULL,
  finding_id uuid REFERENCES platform_findings(id) ON DELETE SET NULL,
  question_id uuid REFERENCES open_questions(id) ON DELETE SET NULL,
  excerpt text NOT NULL,
  rank integer NOT NULL DEFAULT 0,
  lexical_score double precision,
  vector_score double precision,
  rerank_score double precision,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (run_id, layer, reference_id)
);

CREATE INDEX research_evidence_run_idx ON research_evidence (run_id, rank, created_at);

CREATE TABLE research_answers (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL UNIQUE REFERENCES research_runs(id) ON DELETE CASCADE,
  answer text NOT NULL,
  citations jsonb NOT NULL DEFAULT '[]'::jsonb,
  conflicts jsonb NOT NULL DEFAULT '[]'::jsonb,
  insufficient_evidence boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);
