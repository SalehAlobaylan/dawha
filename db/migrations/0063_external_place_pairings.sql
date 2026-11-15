-- Phase 028-F: the pairing-adjudication lane and its crosswalk.
--
-- A machine reading (external_place_candidates) is not a fact. The table
-- copies the 0051 suggestion contract verbatim: the proposal carries where it
-- came from and what the machine read; the reviewer's answer lives beside it
-- (decided_by/at/note_ar) and never instead of it. The accepted crosswalk
-- (place_external_ids) is the only one of the two that any other surface may
-- render as fact, and only after review.
CREATE TABLE external_place_candidates (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  external_key text NOT NULL CHECK (external_key <> ''),
  occurrence_id text NOT NULL CHECK (occurrence_id <> ''),
  our_place_id uuid REFERENCES places(id) ON DELETE SET NULL,
  -- The external reading as reported: bare names plus a link out. A text
  -- column, not jsonb, because the review renders it verbatim and never
  -- queries inside it.
  normalized_candidates text NOT NULL DEFAULT '',
  coordinates geometry(Point, 4326),
  -- Measured components, kept apart on purpose: a single vibe number would
  -- hide which evidence drove the ranking. name_similarity is pg_trgm
  -- similarity against the place's normalized name; distance_km is the
  -- PostGIS distance to the same place. `rank` within the source's queue is
  -- derived from them by the builder, not by the UI.
  name_similarity double precision NOT NULL DEFAULT 0
    CHECK (name_similarity >= 0 AND name_similarity <= 1),
  distance_km double precision,
  rank integer CHECK (rank IS NULL OR rank >= 0),
  -- Coordinate delta proposed by the machine: straight-line kilometres between
  -- the external pin and the place's pin. It is a proposal, never an
  -- overwrite; see the review API.
  coordinate_delta_km double precision,
  status text NOT NULL DEFAULT 'suggested'
    CHECK (status IN ('suggested', 'accepted', 'alias_only', 'rejected', 'deferred')),
  -- The reviewer's answer, stored beside the proposal and never instead of it.
  decided_by uuid REFERENCES users(id) ON DELETE SET NULL,
  decided_at timestamptz,
  decision_note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_id, occurrence_id)
);

CREATE INDEX external_place_candidates_queue_idx
  ON external_place_candidates (status, created_at, id);
CREATE INDEX external_place_candidates_place_idx
  ON external_place_candidates (our_place_id, status);

-- The accepted crosswalk. One row per (place, source, external key): the
-- reviewer's decision that this external record *is* this place, with the
-- provenance of that decision. Read-only on the place surface once written.
CREATE TABLE place_external_ids (
  place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  external_key text NOT NULL CHECK (external_key <> ''),
  decided_by uuid REFERENCES users(id) ON DELETE SET NULL,
  decided_at timestamptz,
  decision_note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (place_id, source_id, external_key)
);

CREATE INDEX place_external_ids_place_idx ON place_external_ids (place_id);
CREATE INDEX place_external_ids_key_idx ON place_external_ids (source_id, external_key);
