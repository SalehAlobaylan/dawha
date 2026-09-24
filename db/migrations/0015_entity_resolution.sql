ALTER TABLE people
  ADD COLUMN merged_into_id uuid REFERENCES people(id) ON DELETE RESTRICT,
  ADD COLUMN merged_at timestamptz;

CREATE INDEX people_merged_into_idx ON people (merged_into_id) WHERE merged_into_id IS NOT NULL;

ALTER TABLE families
  ADD COLUMN identity_status text NOT NULL DEFAULT 'unreviewed' CHECK (identity_status IN ('unreviewed', 'reviewed', 'disputed', 'merged')),
  ADD COLUMN merged_into_id uuid REFERENCES families(id) ON DELETE RESTRICT,
  ADD COLUMN merged_at timestamptz;

CREATE INDEX families_merged_into_idx ON families (merged_into_id) WHERE merged_into_id IS NOT NULL;

CREATE TABLE entity_resolution_runs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  requested_by uuid NOT NULL REFERENCES users(id),
  entity_type text NOT NULL CHECK (entity_type IN ('person', 'family', 'all')),
  status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'succeeded', 'failed')),
  algorithm_version text NOT NULL,
  normalization_version text NOT NULL,
  model_version text,
  candidate_count integer NOT NULL DEFAULT 0,
  error text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX entity_resolution_runs_actor_idx ON entity_resolution_runs (requested_by, created_at DESC);
CREATE INDEX entity_resolution_runs_status_idx ON entity_resolution_runs (status, created_at DESC);

CREATE TABLE entity_resolution_blocks (
  run_id uuid NOT NULL REFERENCES entity_resolution_runs(id) ON DELETE CASCADE,
  entity_type text NOT NULL CHECK (entity_type IN ('person', 'family')),
  entity_id uuid NOT NULL,
  block_type text NOT NULL CHECK (block_type IN ('name', 'alias', 'father', 'grandfather', 'place', 'date', 'tree', 'branch')),
  block_key text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (run_id, entity_type, entity_id, block_type, block_key)
);

CREATE INDEX entity_resolution_blocks_lookup_idx ON entity_resolution_blocks (entity_type, block_type, block_key, run_id);

CREATE TABLE entity_resolution_candidates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES entity_resolution_runs(id) ON DELETE CASCADE,
  entity_type text NOT NULL CHECK (entity_type IN ('person', 'family')),
  left_entity_id uuid NOT NULL,
  right_entity_id uuid NOT NULL,
  left_name_ar text NOT NULL,
  right_name_ar text NOT NULL,
  match_class text NOT NULL CHECK (match_class IN ('likely_different', 'possible_match', 'strong_candidate')),
  score double precision NOT NULL CHECK (score >= 0 AND score <= 1),
  score_components jsonb NOT NULL DEFAULT '{}'::jsonb,
  matching_signals jsonb NOT NULL DEFAULT '[]'::jsonb,
  conflicting_signals jsonb NOT NULL DEFAULT '[]'::jsonb,
  explanation_ar text NOT NULL,
  review_status text NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending', 'approved', 'rejected', 'deferred', 'reopened', 'merged')),
  candidate_version integer NOT NULL DEFAULT 1,
  reviewed_by uuid REFERENCES users(id),
  reviewed_at timestamptz,
  review_note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (left_entity_id < right_entity_id),
  UNIQUE (run_id, entity_type, left_entity_id, right_entity_id)
);

CREATE INDEX entity_resolution_candidates_queue_idx ON entity_resolution_candidates (review_status, match_class, score DESC, created_at DESC);
CREATE INDEX entity_resolution_candidates_run_idx ON entity_resolution_candidates (run_id, score DESC);

CREATE TABLE entity_resolution_reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  candidate_id uuid NOT NULL REFERENCES entity_resolution_candidates(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id),
  decision text NOT NULL CHECK (decision IN ('approve', 'reject', 'defer', 'reopen')),
  note_ar text,
  candidate_version integer NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX entity_resolution_reviews_candidate_idx ON entity_resolution_reviews (candidate_id, created_at DESC);

CREATE TABLE entity_merges (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  candidate_id uuid NOT NULL REFERENCES entity_resolution_candidates(id),
  entity_type text NOT NULL CHECK (entity_type IN ('person', 'family')),
  survivor_id uuid NOT NULL,
  merged_id uuid NOT NULL,
  state text NOT NULL DEFAULT 'applied' CHECK (state IN ('applied', 'reversed')),
  before_snapshot jsonb NOT NULL,
  after_snapshot jsonb NOT NULL,
  requested_by uuid NOT NULL REFERENCES users(id),
  reason_ar text NOT NULL,
  applied_at timestamptz NOT NULL DEFAULT now(),
  reversed_by uuid REFERENCES users(id),
  reversed_at timestamptz,
  reversal_reason_ar text,
  CHECK (survivor_id <> merged_id)
);

CREATE UNIQUE INDEX entity_merges_active_pair_idx ON entity_merges (entity_type, survivor_id, merged_id) WHERE state = 'applied';
CREATE INDEX entity_merges_candidate_idx ON entity_merges (candidate_id, applied_at DESC);
