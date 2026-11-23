-- Plan 030-F: the tile-task queue over a plate's proposals.
--
-- A plate is a sheet; a reviewer cannot answer 74 labels in one sitting, and
-- an unbounded flat queue would hand them seven screens of context at once.
-- The plate is therefore divided into bounded squares of tiles, one task per
-- square holding proposals — Tasking-Manager's task grid, scaled to a plate.
-- A reviewer claims a tile, reads the crops inside it through the 030-E
-- reviewer lane, and submits; a second reviewer independently does the same;
-- the tile retires when two passes agree, and returns to the queue when they
-- do not. That is the two-person retirement stipulation 2 pins, unit-tested
-- in the service: the same reviewer twice is never a confirmation.
--
-- The tile's candidate membership is deliberately NOT stored: it is derived
-- from the crop boxes themselves (a centre-in-square test against the tile's
-- own box), so a later lane run that appends proposals is picked up by the
-- existing tiles without a re-sync, and no member list can ever be stale.
-- What IS stored is the tile's square in plate pixels, its source, and the
-- claim/submit bookkeeping — append-only in the events table per the 0065
-- pattern.
CREATE TABLE region_tile_tasks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  plate_key text NOT NULL CHECK (plate_key <> ''),
  -- Deterministic grid id, e.g. tile:admiralty-1916-tribal:r0c3, derived from
  -- the tile index and the plate key; unique per source.
  tile_id text NOT NULL CHECK (tile_id <> ''),
  -- The tile's square in the plate's own full-resolution pixels:
  -- {"x","y","w","h"}, clipped to the plate's neatline frame.
  box_px jsonb NOT NULL CHECK (jsonb_typeof(box_px) = 'object'),
  status text NOT NULL DEFAULT 'open'
    CHECK (status IN ('open', 'in_progress', 'submitted', 'retired')),
  -- claimed_by/at is the CURRENT seat: set by claim, cleared by release and
  -- by retirement. A submitted tile holds no seat — the second pass claims it.
  claimed_by uuid REFERENCES users(id) ON DELETE SET NULL,
  claimed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_id, tile_id),
  CONSTRAINT region_tile_tasks_box_inside CHECK (box_px->>'w' <> ''
    AND box_px->>'h' <> '' AND box_px->>'x' <> '' AND box_px->>'y' <> '')
);

CREATE INDEX region_tile_tasks_queue_idx
  ON region_tile_tasks (status, created_at, id);
CREATE INDEX region_tile_tasks_plate_idx
  ON region_tile_tasks (plate_key, status, id);
CREATE INDEX region_tile_tasks_claim_idx
  ON region_tile_tasks (claimed_by, updated_at DESC);

-- The append-only tile trail: claims, deferrals (release), submits, the
-- retirement confirmations and the invalidations that return a tile to the
-- queue with the disagreement named. A submit's snapshot carries the
-- submitter's own per-candidate verdicts as they decided them through the
-- candidate lane (accept/correct/reject/defer, readings included) — derived
-- by the service from the decision history, carried here once, never re-typed
-- and never a second source of truth for a candidate's answer.
CREATE TABLE region_tile_task_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  task_id uuid NOT NULL REFERENCES region_tile_tasks(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  action text NOT NULL
    CHECK (action IN ('claim', 'release', 'submit', 'confirm', 'invalidate', 'reopen')),
  from_status text NOT NULL,
  to_status text NOT NULL,
  note_ar text NOT NULL DEFAULT '',
  snapshot jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(snapshot) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX region_tile_task_events_task_idx
  ON region_tile_task_events (task_id, created_at, id);
CREATE INDEX region_tile_task_events_reviewer_idx
  ON region_tile_task_events (reviewer_id, created_at DESC);
