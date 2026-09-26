-- Families, tribes, branches and places join the two-value visibility vocabulary
-- sources.visibility already uses in this schema. Until now they had no such
-- column, which is why internal/dictionary and internal/geography could serve every
-- row of them unconditionally: a row written by any write surface was public the
-- moment it landed, with no version and no publication step to point readers at.
--
-- The two halves of this migration are deliberately different values:
--
--   * the backfill is 'public', because that is exactly what the dictionary index,
--     the dictionary detail, the search index and the map serve today. Nothing a
--     reader can see today changes shape.
--   * the default for a new row is 'private', because a row nobody has reviewed is
--     research. The safe direction has to be the one a writer gets by forgetting an
--     argument, so the column default and the write surface both say 'private'
--     explicitly.
ALTER TABLE families ADD COLUMN visibility text NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private'));
ALTER TABLE tribes   ADD COLUMN visibility text NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private'));
ALTER TABLE branches ADD COLUMN visibility text NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private'));
ALTER TABLE places   ADD COLUMN visibility text NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'private'));

-- The backfill, written out rather than implied by the ADD COLUMN default, so the
-- intent survives a reader who only looks at the statement below and so a re-run
-- repairs a row that was changed by hand.
UPDATE families SET visibility = 'public' WHERE visibility IS DISTINCT FROM 'public';
UPDATE tribes   SET visibility = 'public' WHERE visibility IS DISTINCT FROM 'public';
UPDATE branches SET visibility = 'public' WHERE visibility IS DISTINCT FROM 'public';
UPDATE places   SET visibility = 'public' WHERE visibility IS DISTINCT FROM 'public';

ALTER TABLE families ALTER COLUMN visibility SET DEFAULT 'private';
ALTER TABLE tribes   ALTER COLUMN visibility SET DEFAULT 'private';
ALTER TABLE branches ALTER COLUMN visibility SET DEFAULT 'private';
ALTER TABLE places   ALTER COLUMN visibility SET DEFAULT 'private';

CREATE INDEX families_visibility_idx ON families (visibility, updated_at DESC);
CREATE INDEX tribes_visibility_idx   ON tribes   (visibility, updated_at DESC);
CREATE INDEX branches_visibility_idx ON branches (visibility, updated_at DESC);
CREATE INDEX places_visibility_idx   ON places   (visibility, updated_at DESC);
