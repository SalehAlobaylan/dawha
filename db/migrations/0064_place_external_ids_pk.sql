-- The accepted crosswalk gets the uuid primary key every other swept table
-- carries (0063 gave it a composite unique instead). The test-support
-- cleanup traversal addresses a row by `id` or a uuid primary key; a table
-- with neither cannot be deleted or named, so a fixture row reachable from a
-- synthetic actor would block every sweep after the first one. The composite
-- unique stays: it is the crosswalk's identity.
ALTER TABLE place_external_ids ADD COLUMN IF NOT EXISTS id uuid DEFAULT gen_random_uuid();
ALTER TABLE place_external_ids DROP CONSTRAINT IF EXISTS place_external_ids_pkey;
ALTER TABLE place_external_ids ADD CONSTRAINT place_external_ids_pkey PRIMARY KEY (id);
