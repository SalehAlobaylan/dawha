-- Preserve physical-page readings and geometry independently from the passages
-- used for search. OCR is evidence, and a later model run must not erase which
-- engines read the original page or where the text was found.
CREATE TABLE source_page_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id uuid NOT NULL REFERENCES source_processing_runs(id) ON DELETE CASCADE,
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  source_file_id uuid NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
  page_number integer NOT NULL CHECK (page_number > 0),
  page_width double precision NOT NULL DEFAULT 0 CHECK (page_width >= 0),
  page_height double precision NOT NULL DEFAULT 0 CHECK (page_height >= 0),
  extraction_status text NOT NULL CHECK (extraction_status IN ('readable', 'unreadable')),
  selected_method text NOT NULL,
  text_ar text NOT NULL DEFAULT '',
  readings jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(readings) = 'array'),
  regions jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(regions) = 'array'),
  region_count integer NOT NULL DEFAULT 0 CHECK (region_count >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (run_id, page_number)
);

CREATE INDEX source_page_evidence_file_idx ON source_page_evidence (source_file_id, page_number);
CREATE INDEX source_page_evidence_source_idx ON source_page_evidence (source_id, page_number);
