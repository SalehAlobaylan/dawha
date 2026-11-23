-- Plan 031-E: مِسلك التكرار — the tribe-duplicate lane.
--
-- Two tribes whose names/aliases read alike are a CROSSWALK problem, not a
-- merge: plan 005's recorded deferral stands, and this lane's whole charter is
-- the negative space of a merge. The candidate pair carries its own receipts
-- (both matched forms + the similarity as measured, never a verdict); the
-- reviewer's allowed effects are exactly three — link the loser's name as an
-- alias of the chosen canonical, point the loser at the canonical (an
-- append-only entity_relationship row), or reject the pairing. NO statement
-- in this lane deletes or updates a `tribes` row; the rows stay put and the
-- relationships carry the meaning, so the lane's own undo is data, not
-- surgery.
CREATE TABLE tribe_duplicate_candidates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tribe_a_id uuid NOT NULL REFERENCES tribes(id) ON DELETE CASCADE,
  tribe_b_id uuid NOT NULL REFERENCES tribes(id) ON DELETE CASCADE,
  CHECK (tribe_a_id <> tribe_b_id),
  -- Both matched forms, kept verbatim: the receipt's own words.
  matched_a text NOT NULL DEFAULT '',
  matched_b text NOT NULL DEFAULT '',
  -- The similarity as the builder measured it (a character-trigram proxy for
  -- the pg_trgm similarity, in [0,1]) — a measured component, kept apart.
  name_similarity double precision NOT NULL
    CHECK (name_similarity >= 0 AND name_similarity <= 1),
  status text NOT NULL DEFAULT 'suggested'
    CHECK (status IN ('suggested', 'linked', 'pointed', 'rejected')),
  decided_by uuid REFERENCES users(id) ON DELETE SET NULL,
  decided_at timestamptz,
  decision_note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tribe_a_id, tribe_b_id)
);

CREATE INDEX tribe_duplicate_candidates_queue_idx
  ON tribe_duplicate_candidates (status, name_similarity DESC, created_at, id);

-- The decisions: append-only, the 0065 pattern. `diff` carries which side was
-- named canonical ({"canonical": "<tribe uuid>"}) beside the proposal, so the
-- reviewer's choice is recorded the way they made it.
CREATE TABLE tribe_duplicate_decisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  candidate_id uuid NOT NULL
    REFERENCES tribe_duplicate_candidates(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  action text NOT NULL
    CHECK (action IN ('link_alias', 'canonize', 'reject')),
  from_status text NOT NULL,
  to_status text NOT NULL,
  note_ar text NOT NULL DEFAULT '',
  diff jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(diff) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tribe_duplicate_decisions_candidate_idx
  ON tribe_duplicate_decisions (candidate_id, created_at, id);
