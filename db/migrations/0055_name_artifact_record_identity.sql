-- Keep the content fingerprint unique, while giving private artifact records
-- the same UUID identity used by retention and relational fixture cleanup.
ALTER TABLE source_name_crop_artifacts DROP CONSTRAINT source_name_crop_artifacts_pkey;
ALTER TABLE source_name_crop_artifacts ADD COLUMN id uuid NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE source_name_crop_artifacts ADD PRIMARY KEY(id);
ALTER TABLE source_name_crop_artifacts ADD UNIQUE(fingerprint);
