-- Use the application's UUID identity convention for revision references.
-- Keep the existing sequence for stable ordering and preserve every revision.
ALTER TABLE plate_caption_revisions DROP CONSTRAINT plate_caption_revisions_pkey;
ALTER TABLE plate_caption_revisions RENAME COLUMN id TO revision_order;
ALTER TABLE plate_caption_revisions ADD COLUMN id uuid NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE plate_caption_revisions ADD PRIMARY KEY (id);
