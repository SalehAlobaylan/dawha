-- Reopening, correction revisions and geometry repair are additions to the
-- ledger, never rewrites of an earlier human decision. The current status on
-- source_tree_suggestions is operational queue state; every prior answer stays
-- in source_tree_suggestion_decisions and every action gets an event.
ALTER TABLE source_tree_suggestions
  ADD COLUMN review_revision integer NOT NULL DEFAULT 0
    CHECK (review_revision >= 0),
  ADD COLUMN geometry_revision integer NOT NULL DEFAULT 0
    CHECK (geometry_revision >= 0),
  ADD COLUMN superseded_at timestamptz,
  ADD COLUMN superseded_by_event_id uuid;

-- The revision of a decision is the review cycle it closed. Existing decisions
-- keep 0 so an old client's first-decision contract remains readable; new
-- decisions always carry a positive revision.
ALTER TABLE source_tree_suggestion_decisions
  ADD COLUMN review_revision integer NOT NULL DEFAULT 0
    CHECK (review_revision >= 0),
  ADD COLUMN supersedes_decision_id uuid REFERENCES source_tree_suggestion_decisions(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX source_tree_suggestion_decisions_revision_idx
  ON source_tree_suggestion_decisions (suggestion_id, review_revision)
  WHERE review_revision > 0;

-- Geometry is immutable evidence too. The original detection rectangle is the
-- first row's previous_region when a reviewer has adjusted it; it is never
-- reconstructed from a later rendering.
CREATE TABLE source_name_region_revisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  suggestion_id uuid NOT NULL REFERENCES source_tree_suggestions(id) ON DELETE CASCADE,
  revision integer NOT NULL CHECK (revision > 0),
  previous_region jsonb NOT NULL CHECK (jsonb_typeof(previous_region) = 'object'),
  region jsonb NOT NULL CHECK (jsonb_typeof(region) = 'object'),
  coordinate_space text NOT NULL CHECK (coordinate_space IN ('native_raster', 'pdf_analysis')),
  analysis_dpi integer NOT NULL DEFAULT 0 CHECK (analysis_dpi >= 0),
  source_checksum text NOT NULL CHECK (length(source_checksum) = 64),
  frame_width integer NOT NULL CHECK (frame_width > 0),
  frame_height integer NOT NULL CHECK (frame_height > 0),
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
  UNIQUE (suggestion_id, revision)
);
CREATE INDEX source_name_region_revisions_suggestion_idx
  ON source_name_region_revisions (suggestion_id, revision);

CREATE TABLE source_name_review_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  suggestion_id uuid NOT NULL REFERENCES source_tree_suggestions(id) ON DELETE CASCADE,
  actor_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  event_type text NOT NULL
    CHECK (event_type IN ('reopened', 'decision', 'region_revised', 'split', 'merge')),
  revision integer NOT NULL CHECK (revision >= 0),
  previous_decision_id uuid REFERENCES source_tree_suggestion_decisions(id) ON DELETE SET NULL,
  decision_id uuid REFERENCES source_tree_suggestion_decisions(id) ON DELETE SET NULL,
  geometry_revision_id uuid REFERENCES source_name_region_revisions(id) ON DELETE SET NULL,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(metadata) = 'object'),
  created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX source_name_review_events_suggestion_idx
  ON source_name_review_events (suggestion_id, created_at, id);

ALTER TABLE source_tree_suggestions
  ADD CONSTRAINT source_tree_suggestions_superseded_by_event_fk
  FOREIGN KEY (superseded_by_event_id) REFERENCES source_name_review_events(id) ON DELETE SET NULL;

-- The queue is the pending operational set. Superseded geometry remains in the
-- history table and every decision ledger, but it is not reviewed again by
-- default.
CREATE INDEX source_tree_suggestions_pending_idx
  ON source_tree_suggestions (source_file_id, review_revision, created_at, id)
  WHERE status = 'suggested' AND superseded_at IS NULL;
CREATE INDEX source_tree_suggestions_deferred_idx
  ON source_tree_suggestions (source_file_id, decided_at DESC, id)
  WHERE status = 'deferred';
