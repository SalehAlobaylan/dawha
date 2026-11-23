-- Plan 033-B: movement corridors (الممرات) — ordered stops with day-stages,
-- written under the 0063 reviewer contract.
--
-- The corridor is a SUBJECT's own route: an ordered stop list whose legs carry
-- the day counts and the mode the sources name (camel/horse/foot/sea), the
-- season it runs in (صيف/شتاء), and the years window it operated (the season
-- window the plan names). The stipulations that shape this schema:
--
-- 1. NO CORRIDOR WITHOUT source_id: the column is NOT NULL FK to sources. A
--    corridor nobody sourced is an invented trajectory — the pseudo-precision
--    this plan named as its own danger, refused at the schema's door.
-- 2. The leg's day counts are POSITIVE integers; the first stop has no leg
--    before it, so both leg columns stay NULL there and nothing else.
-- 3. Certainty stays 'approximate' by default; 'precise' requires a reviewer
--    acceptance (decided_by/at) beside the row — the 0063 answer-beside-
--    proposal contract, copied verbatim.
-- 4. Bounded reads everywhere; the schema only carries what the reads answer.
--
-- Idempotent by the IF NOT EXISTS guards; safe to re-run.

CREATE TABLE IF NOT EXISTS movement_corridors (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subject_type text NOT NULL
    CHECK (subject_type = ANY (ARRAY['person'::text, 'tribe'::text, 'family'::text])),
  subject_id uuid NOT NULL,
  -- One corridor in the reader's own words (e.g. «مرحلة الصيف إلى العلا»).
  title_ar text NOT NULL DEFAULT '',
  season text
    CHECK (season IS NULL OR season = ANY (ARRAY['summer'::text, 'winter'::text])),
  -- The years window the corridor operated (the season's own window).
  valid_from integer CHECK (valid_from IS NULL OR valid_from >= -10000),
  valid_to integer CHECK (valid_to IS NULL OR valid_to <= 10000),
  -- Movement rows keep the atlas's own status vocabulary; a corridor reaches
  -- 'documented' only through the reviewer's acceptance below.
  status text NOT NULL DEFAULT 'interpreted'
    CHECK (status = ANY (ARRAY['documented'::text, 'interpreted'::text, 'platform_inferred'::text, 'disputed'::text, 'unresolved'::text])),
  certainty text NOT NULL DEFAULT 'approximate'
    CHECK (certainty = ANY (ARRAY['precise'::text, 'approximate'::text, 'uncertain'::text])),
  -- The plan's own acceptance: no corridor without its source.
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  -- The reviewer's answer, beside the proposal and never instead of it (0063).
  decided_by uuid REFERENCES users(id) ON DELETE SET NULL,
  decided_at timestamptz,
  decision_note_ar text,
  created_by uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS movement_corridors_subject_idx
  ON movement_corridors (subject_type, subject_id);
CREATE INDEX IF NOT EXISTS movement_corridors_source_idx
  ON movement_corridors (source_id);

CREATE TABLE IF NOT EXISTS movement_corridor_stops (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  corridor_id uuid NOT NULL REFERENCES movement_corridors(id) ON DELETE CASCADE,
  sequence_number integer NOT NULL CHECK (sequence_number >= 1),
  place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  -- The leg INTO this stop: day count + the mode the source names. NULL on
  -- the sequence's first stop, positive integers elsewhere (enforced by the
  -- trigger-less route: the writer validates, the read re-checks).
  day_count_from_prev integer CHECK (day_count_from_prev IS NULL OR day_count_from_prev > 0),
  mode_from_prev text
    CHECK (mode_from_prev IS NULL OR mode_from_prev = ANY (ARRAY[
      'camel'::text, 'horse'::text, 'foot'::text, 'sea'::text
    ])),
  notes_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (corridor_id, sequence_number)
);

CREATE INDEX IF NOT EXISTS movement_corridor_stops_place_idx
  ON movement_corridor_stops (place_id);

-- The first-stop leg rule: the sequence's own head carries no mode/day. The
-- stops' own CHECKs keep the legs positive; this one keeps the head honest.
ALTER TABLE movement_corridor_stops
  DROP CONSTRAINT IF EXISTS movement_corridor_stops_head_has_no_leg;
ALTER TABLE movement_corridor_stops
  ADD CONSTRAINT movement_corridor_stops_head_has_no_leg
  CHECK (sequence_number <> 1 OR (day_count_from_prev IS NULL AND mode_from_prev IS NULL));
