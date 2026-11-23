CREATE TABLE region_boundary_reviews (
  candidate_id uuid NOT NULL REFERENCES external_region_candidates(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE RESTRICT,
  locator text NOT NULL CHECK (length(locator) BETWEEN 1 AND 500),
  boundary geometry(Polygon, 4326) NOT NULL,
  reviewed_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (candidate_id, reviewer_id),
  CHECK (ST_IsValid(boundary) AND NOT ST_IsEmpty(boundary) AND ST_NPoints(boundary) <= 160
    AND ST_XMin(boundary) >= -180 AND ST_XMax(boundary) <= 180
    AND ST_YMin(boundary) >= -90 AND ST_YMax(boundary) <= 90)
);

-- Earlier candidate-linked regions were derived from label crops.
ALTER TABLE historical_regions ADD COLUMN boundary_reviewed boolean NOT NULL DEFAULT false;
CREATE UNIQUE INDEX historical_regions_one_reviewed_region_per_candidate
  ON historical_regions(candidate_id) WHERE candidate_id IS NOT NULL AND boundary_reviewed;

CREATE TABLE plate_captions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  scan_sha256 text NOT NULL CHECK (scan_sha256 ~ '^[0-9a-f]{64}$'),
  catalog_order bigint GENERATED ALWAYS AS IDENTITY,
  x integer NOT NULL CHECK (x >= 0),
  y integer NOT NULL CHECK (y >= 0),
  printed_name text NOT NULL CHECK (length(printed_name) BETWEEN 1 AND 200),
  name_ar text NOT NULL CHECK (length(name_ar) BETWEEN 1 AND 200),
  UNIQUE (source_id, scan_sha256, x, y)
);
CREATE TABLE plate_caption_revisions (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  caption_id uuid NOT NULL REFERENCES plate_captions(id) ON DELETE CASCADE,
  name_ar text NOT NULL CHECK (length(name_ar) BETWEEN 1 AND 200),
  proposed_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  proposed_at timestamptz NOT NULL DEFAULT now(),
  published_by uuid REFERENCES users(id) ON DELETE RESTRICT,
  published_at timestamptz,
  CHECK ((published_by IS NULL) = (published_at IS NULL))
);
CREATE INDEX plate_caption_revision_history ON plate_caption_revisions(caption_id, id DESC);
