-- A SUGGESTION is one machine reading of one place on a page, waiting for a
-- person's decision. It is not a person, a claim, or a fact.
--
-- The distinction the whole import rests on is here: a suggestion carries
-- WHERE it came from (the page, the region, the method and the model), WHAT
-- the machine read (suggested_text_ar, which may be empty when the machine
-- could not read it), and what the REVIEWER decided (decided_text_ar). The
-- reviewer's answer is stored beside the machine's reading and never instead
-- of it, so the two can always be compared.
--
-- Confidence is MEASURED, not reported. It is the share of the reading views
-- that agreed on the weakest character, which is a physical quantity. A model
-- asked how sure it is returns a number that is at times worse than chance at
-- separating a correct reading from a wrong one.
CREATE TABLE source_tree_suggestions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  source_file_id uuid NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
  source_page_evidence_id uuid REFERENCES source_page_evidence(id) ON DELETE CASCADE,
  processing_run_id uuid REFERENCES source_processing_runs(id) ON DELETE SET NULL,
  occurrence_id text NOT NULL CHECK (occurrence_id ~ '^[A-Za-z0-9_-]{1,80}$'),
  page_number integer NOT NULL CHECK (page_number > 0),
  kind text NOT NULL DEFAULT 'name' CHECK (kind IN ('name')),
  -- The reading as the machine gave it. Empty when it could not read the place.
  suggested_text_ar text NOT NULL DEFAULT '',
  -- The region on the page, normalised to the top-left corner, so that the
  -- reviewer sees the crop the reading came from and not a name in the dark.
  region jsonb NOT NULL CHECK (jsonb_typeof(region) = 'object'),
  -- The measured agreement between the reading views.
  confidence double precision NOT NULL DEFAULT 0
    CHECK (confidence >= 0 AND confidence <= 1),
  view_count integer NOT NULL DEFAULT 1 CHECK (view_count > 0),
  reading_method text NOT NULL,
  reading_model text NOT NULL,
  status text NOT NULL DEFAULT 'suggested'
    CHECK (status IN ('suggested', 'accepted', 'corrected', 'rejected', 'deferred')),
  -- What the reviewer decided, kept beside the machine's reading and never
  -- instead of it.
  decided_text_ar text,
  decided_by uuid REFERENCES users(id) ON DELETE SET NULL,
  decided_at timestamptz,
  decision_note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_file_id, occurrence_id)
);

-- The review queue: the suggestions still to decide, the most certain first.
CREATE INDEX source_tree_suggestions_queue_idx
  ON source_tree_suggestions (source_file_id, status, confidence DESC, created_at, id);
CREATE INDEX source_tree_suggestions_page_idx
  ON source_tree_suggestions (source_file_id, page_number, created_at, id);

-- The decisions are append-only and are the measurement.
--
-- Every correction a reviewer makes is a labelled example, and it is the only
-- kind of label this feature will get at volume: the answer key is produced by
-- using the feature, not by a separate labelling project. A rejected or
-- deferred suggestion is data too - it is the machine being wrong, and it
-- stays.
CREATE TABLE source_tree_suggestion_decisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  suggestion_id uuid NOT NULL REFERENCES source_tree_suggestions(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id),
  decision text NOT NULL CHECK (decision IN ('accepted', 'corrected', 'rejected', 'deferred')),
  suggested_text_ar text NOT NULL DEFAULT '',
  decided_text_ar text,
  note_ar text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX source_tree_suggestion_decisions_suggestion_idx
  ON source_tree_suggestion_decisions (suggestion_id, created_at, id);
CREATE INDEX source_tree_suggestion_decisions_reviewer_idx
  ON source_tree_suggestion_decisions (reviewer_id, created_at DESC);
