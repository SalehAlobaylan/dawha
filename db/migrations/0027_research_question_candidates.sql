CREATE TABLE research_question_candidates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES research_agent_runs(id) ON DELETE CASCADE,
  origin_question_id uuid REFERENCES open_questions(id) ON DELETE SET NULL,
  origin_gap_id uuid NOT NULL REFERENCES research_agent_gaps(id) ON DELETE CASCADE,
  origin_recommendation_id uuid REFERENCES research_agent_recommendations(id) ON DELETE SET NULL,
  title_ar text NOT NULL,
  description_ar text NOT NULL,
  priority text NOT NULL CHECK (priority IN ('low', 'normal', 'high')),
  status text NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed', 'converted', 'dismissed')),
  dedupe_key text NOT NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  generated_by uuid NOT NULL REFERENCES users(id),
  question_id uuid REFERENCES open_questions(id) ON DELETE SET NULL,
  reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
  reviewed_at timestamptz,
  review_note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (run_id, dedupe_key),
  CHECK (char_length(title_ar) BETWEEN 3 AND 200),
  CHECK (char_length(description_ar) BETWEEN 3 AND 2000),
  CHECK (char_length(review_note_ar) IS NULL OR char_length(review_note_ar) <= 2000),
  CHECK (jsonb_typeof(metadata) = 'object'),
  CHECK (octet_length(metadata::text) <= 16384)
);

CREATE INDEX research_question_candidates_run_idx
  ON research_question_candidates (run_id, status, created_at DESC);

CREATE INDEX research_question_candidates_review_queue_idx
  ON research_question_candidates (status, priority, created_at DESC);

CREATE UNIQUE INDEX research_question_candidates_question_idx
  ON research_question_candidates (question_id)
  WHERE question_id IS NOT NULL;

CREATE TABLE research_question_candidate_reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  candidate_id uuid NOT NULL REFERENCES research_question_candidates(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id),
  decision text NOT NULL CHECK (decision IN ('converted', 'dismissed')),
  question_id uuid REFERENCES open_questions(id) ON DELETE SET NULL,
  note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (char_length(note_ar) IS NULL OR char_length(note_ar) <= 2000)
);

CREATE INDEX research_question_candidate_reviews_candidate_idx
  ON research_question_candidate_reviews (candidate_id, created_at DESC);
