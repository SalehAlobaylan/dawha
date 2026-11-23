-- Phase 030-E: machine-proposed region labels read off a georeferenced plate.
--
-- A plate label is a *proposal*, not a place. The machine can find ink that
-- looks like a text line and project its centre through the plate's published
-- georeference; it cannot say that the text is a tribal name, which tribe it
-- names, or where that tribe's boundary ran. Those are the reviewer's answers,
-- and they live beside the proposal (decided_by/at/note_ar and the append-only
-- history table), never instead of it.
--
-- The contract copies external_place_candidates (0063): source, occurrence,
-- measured score components kept apart, and the reviewer's answer in its own
-- columns. It deliberately does NOT create a historical_regions row on accept —
-- that promotion belongs to a later review surface, and until then nothing in
-- this table may be rendered as a fact.
--
-- Polygon geometry is optional and capped at the same 160 vertices the atlas
-- already enforces (geography.MaxRegionRingVertices). A label box alone is a
-- point; a boundary trace, when one exists, is a polygon and is capped here in
-- the schema so no consumer can be handed an over-long ring.
CREATE TABLE external_region_candidates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  plate_key text NOT NULL CHECK (plate_key <> ''),
  occurrence_id text NOT NULL CHECK (occurrence_id <> ''),
  -- The box the proposal came from, in the plate's own full-resolution pixel
  -- coordinates: {"x","y","w","h"}. Kept so a reviewer can be shown the crop
  -- that produced the proposal, and so a re-run can be compared with the read.
  crop_px jsonb NOT NULL CHECK (jsonb_typeof(crop_px) = 'object'),
  -- The label box centre, projected through the plate's published transform.
  -- A proposal's position is only as good as that transform's held-out error;
  -- transform_rms_km records the floor below.
  coordinates geometry(Point, 4326),
  -- A traced boundary, when the lane produces one. NULL for a label-only
  -- proposal, which is every row the first lane writes.
  region_geometry geometry(Polygon, 4326),
  -- The reviewer's reading of the crop. Empty until a human reads it; a
  -- machine must never write this column.
  label_text_ar text NOT NULL DEFAULT '',
  label_read_by uuid REFERENCES users(id) ON DELETE SET NULL,
  label_read_at timestamptz,
  era_label text NOT NULL DEFAULT '',
  -- Score components, kept apart on purpose: a single confidence number would
  -- hide which measurement drove the ranking. label_confidence answers "does
  -- this box look like a printed text line", not "is this a tribe".
  ink_density double precision CHECK (ink_density >= 0 AND ink_density <= 1),
  component_count integer CHECK (component_count >= 0),
  label_confidence double precision
    CHECK (label_confidence >= 0 AND label_confidence <= 1),
  transform_rms_km double precision CHECK (transform_rms_km > 0),
  rank integer CHECK (rank IS NULL OR rank >= 0),
  status text NOT NULL DEFAULT 'suggested'
    CHECK (status IN ('suggested', 'accepted', 'corrected', 'rejected', 'deferred')),
  decided_by uuid REFERENCES users(id) ON DELETE SET NULL,
  decided_at timestamptz,
  decision_note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_id, occurrence_id),
  CONSTRAINT external_region_candidates_vertex_cap CHECK (
    region_geometry IS NULL OR ST_NPoints(region_geometry) <= 160
  )
);

CREATE INDEX external_region_candidates_queue_idx
  ON external_region_candidates (status, created_at, id);
CREATE INDEX external_region_candidates_source_idx
  ON external_region_candidates (source_id, rank, id);

-- Append-only decision trail, per the 0065 pattern. `diff` carries a
-- correction (a re-read label, a corrected ring) beside the proposal, so the
-- original machine reading is never overwritten by a reviewer's answer.
CREATE TABLE external_region_candidate_decisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  candidate_id uuid NOT NULL
    REFERENCES external_region_candidates(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  action text NOT NULL
    CHECK (action IN ('accept', 'correct', 'reject', 'defer', 'reopen')),
  from_status text NOT NULL,
  to_status text NOT NULL,
  note_ar text NOT NULL DEFAULT '',
  diff jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(diff) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX external_region_candidate_decisions_candidate_idx
  ON external_region_candidate_decisions (candidate_id, created_at, id);
CREATE INDEX external_region_candidate_decisions_reviewer_idx
  ON external_region_candidate_decisions (reviewer_id, created_at DESC);
