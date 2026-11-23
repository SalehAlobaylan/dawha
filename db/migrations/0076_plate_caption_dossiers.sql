-- A caption can link to an explicitly identified tribe. Ambiguous names stay unlinked.
ALTER TABLE plate_captions ADD COLUMN tribe_id uuid REFERENCES tribes(id) ON DELETE SET NULL;
