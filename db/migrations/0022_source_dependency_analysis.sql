ALTER TABLE source_dependencies
  ADD COLUMN IF NOT EXISTS algorithm_version text,
  ADD COLUMN IF NOT EXISTS signal_data jsonb NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS reviewed_by uuid REFERENCES users(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS reviewed_at timestamptz,
  ADD COLUMN IF NOT EXISTS review_note_ar text;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'source_dependencies_signal_data_object_check') THEN
    ALTER TABLE source_dependencies
      ADD CONSTRAINT source_dependencies_signal_data_object_check
      CHECK (jsonb_typeof(signal_data) = 'object');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'source_dependencies_no_self_check') THEN
    ALTER TABLE source_dependencies
      ADD CONSTRAINT source_dependencies_no_self_check
      CHECK (source_id <> depends_on_source_id);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'source_dependencies_algorithm_version_check') THEN
    ALTER TABLE source_dependencies
      ADD CONSTRAINT source_dependencies_algorithm_version_check
      CHECK (algorithm_version IS NULL OR char_length(algorithm_version) BETWEEN 1 AND 64);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'source_dependencies_review_note_check') THEN
    ALTER TABLE source_dependencies
      ADD CONSTRAINT source_dependencies_review_note_check
      CHECK (review_note_ar IS NULL OR char_length(review_note_ar) <= 2000);
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS source_dependency_reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  dependency_id uuid NOT NULL REFERENCES source_dependencies(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id),
  decision text NOT NULL CHECK (decision IN ('confirmed', 'rejected')),
  note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (note_ar IS NULL OR char_length(note_ar) <= 2000)
);

CREATE INDEX IF NOT EXISTS source_dependencies_source_status_idx
  ON source_dependencies (source_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS source_dependencies_target_status_idx
  ON source_dependencies (depends_on_source_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS source_dependency_reviews_dependency_idx
  ON source_dependency_reviews (dependency_id, created_at DESC);
