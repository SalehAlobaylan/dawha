ALTER TABLE source_files
  ADD COLUMN processing_status text NOT NULL DEFAULT 'queued' CHECK (processing_status IN ('queued', 'running', 'succeeded', 'failed')),
  ADD COLUMN processing_error text,
  ADD COLUMN processed_at timestamptz;

ALTER TABLE source_passages
  ADD COLUMN source_file_id uuid REFERENCES source_files(id) ON DELETE SET NULL,
  ADD COLUMN start_offset integer,
  ADD COLUMN end_offset integer;

CREATE INDEX source_passages_file_idx ON source_passages (source_file_id, sequence_number);

ALTER TABLE source_statements
  ADD COLUMN source_file_id uuid REFERENCES source_files(id) ON DELETE SET NULL;

CREATE INDEX source_statements_file_idx ON source_statements (source_file_id, created_at);

CREATE TABLE source_processing_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  source_file_id uuid NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
  job_id uuid REFERENCES jobs(id) ON DELETE SET NULL,
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
  stage text NOT NULL DEFAULT 'queued',
  page_count integer NOT NULL DEFAULT 0 CHECK (page_count >= 0),
  passage_count integer NOT NULL DEFAULT 0 CHECK (passage_count >= 0),
  candidate_count integer NOT NULL DEFAULT 0 CHECK (candidate_count >= 0),
  model_version text,
  error text,
  started_at timestamptz,
  completed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_file_id)
);

CREATE INDEX source_processing_runs_source_idx ON source_processing_runs (source_id, created_at DESC);
CREATE INDEX source_processing_runs_status_idx ON source_processing_runs (status, updated_at);

CREATE TABLE source_candidates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  source_file_id uuid NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
  source_passage_id uuid NOT NULL REFERENCES source_passages(id) ON DELETE CASCADE,
  source_statement_id uuid REFERENCES source_statements(id) ON DELETE SET NULL,
  candidate_type text NOT NULL CHECK (candidate_type IN ('entity', 'claim')),
  raw_text_ar text NOT NULL,
  normalized_text_ar text NOT NULL,
  subject_text_ar text,
  predicate_ar text,
  object_text_ar text,
  proposed_entity_type text,
  proposed_entity_id uuid,
  proposed_entity_name_ar text,
  proposed_match_score double precision,
  proposed_match_matched_on text,
  subject_entity_type text,
  subject_entity_id uuid,
  object_entity_type text,
  object_entity_id uuid,
  confidence double precision NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
  rationale_ar text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  model_version text,
  dedupe_key text NOT NULL,
  status text NOT NULL DEFAULT 'needs_review' CHECK (status IN ('unreviewed', 'accepted', 'rejected', 'needs_review')),
  reviewed_by uuid REFERENCES users(id),
  reviewed_at timestamptz,
  review_note_ar text,
  accepted_record_type text,
  accepted_record_id uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_file_id, dedupe_key)
);

CREATE INDEX source_candidates_source_idx ON source_candidates (source_id, status, created_at DESC);
CREATE INDEX source_candidates_file_idx ON source_candidates (source_file_id, created_at);
CREATE INDEX source_candidates_passage_idx ON source_candidates (source_passage_id);

CREATE TABLE source_candidate_reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  candidate_id uuid NOT NULL REFERENCES source_candidates(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id),
  decision text NOT NULL CHECK (decision IN ('accepted', 'rejected')),
  note_ar text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX source_candidate_reviews_candidate_idx ON source_candidate_reviews (candidate_id, created_at DESC);
