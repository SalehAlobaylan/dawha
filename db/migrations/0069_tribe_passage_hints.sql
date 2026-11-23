-- Plan 031-C: record hints — a passage's tribe mentions as suggestion rows.
--
-- The ingest lanes (030-B/C/E) land passages quoting tribes; nothing yet ties
-- a passage to the dossier it mentions. This migration creates the SUGGESTION
-- rows that tie them — a (tribe, passage) hint, written by a machine matcher,
-- carried in a bounded queue, and NEVER applied on its own: an accepted hint
-- is a named reviewer's decision recorded beside the proposal (the 030 accept
-- path), and its effect is a documented record (a claim with its evidence),
-- which is what makes the dossier's cite-never-assert rule hold.
--
-- Dismissals are REVIEWER-SCOPED, not global: the hint rows carry the
-- suggestion; the decision table carries each reviewer's answer (dismiss =
-- hide for me, keep for others; accept = the receipt). The unique keys make
-- re-runs idempotent and make a dismissed suggestion returnable only through
-- a fresh match (a re-seeded passage or a new alias form).
CREATE TABLE tribe_passage_hints (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tribe_id uuid NOT NULL REFERENCES tribes(id) ON DELETE CASCADE,
  source_passage_id uuid NOT NULL REFERENCES source_passages(id) ON DELETE CASCADE,
  -- The forms that hit in this passage (normalized), kept as the receipt's
  -- own words: a hint cites what it matched, never just a count.
  matched_forms text[] NOT NULL DEFAULT '{}',
  status text NOT NULL DEFAULT 'suggested' CHECK (status IN ('suggested', 'accepted')),
  decided_by uuid REFERENCES users(id) ON DELETE SET NULL,
  decided_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tribe_id, source_passage_id)
);

CREATE INDEX tribe_passage_hints_queue_idx
  ON tribe_passage_hints (status, created_at, id);
CREATE INDEX tribe_passage_hints_tribe_idx
  ON tribe_passage_hints (tribe_id, status, created_at);

-- The decisions: append-only, per reviewer, per CARD (the hint queue's own
-- unit is a (tribe, source) card, so the decision names the pair it governed).
CREATE TABLE tribe_passage_hint_decisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  hint_id uuid NOT NULL REFERENCES tribe_passage_hints(id) ON DELETE CASCADE,
  tribe_id uuid NOT NULL REFERENCES tribes(id) ON DELETE CASCADE,
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  action text NOT NULL CHECK (action IN ('accept', 'dismiss')),
  note_ar text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tribe_passage_hint_decisions_card_idx
  ON tribe_passage_hint_decisions (tribe_id, source_id, reviewer_id, created_at);
CREATE INDEX tribe_passage_hint_decisions_reviewer_idx
  ON tribe_passage_hint_decisions (reviewer_id, created_at DESC);
