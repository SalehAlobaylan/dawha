ALTER TABLE sources
  ADD COLUMN visibility text NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private'));

CREATE INDEX sources_visibility_idx ON sources (visibility, updated_at DESC);
