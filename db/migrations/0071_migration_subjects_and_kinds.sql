-- Plan 033-A/D: migration_events' own vocabulary broadens additively, and the
-- events gain the classical taxonomy (المِناسِب).
--
-- A (subjects): 'tribe' and 'family' join 'person' in subject_type. The check
-- is ADDITIVE — every existing row is a person, so the constraint lands
-- without a data pass, and the person semantics (the queries, the subject
-- layer, the journeys) read the same shapes they always did. The vocabulary
-- is closed ON PURPOSE: a subject that is a person, a tribe or a family is
-- one the atlas can name and scope; anything else is a typo, not a concept
-- the map can anchor.
--
-- D (taxonomy): migration_events.kind carries the classical repertoire the
-- sources actually narrate (yawm / ghazwa / sariya / nafir / relocation /
-- famine / other). NULL stays legal — an event whose sources narrate a
-- movement without classifying it renders "حدث غير مُصنَّف بعد", which is
-- the honest row, not a lost one. The Arabic labels live in one vocabulary
-- file beside the atlas-periods artifact
-- (apps/web/src/lib/movement-events.ts); the CHECK carries the keys only.
--
-- Idempotent by the IF NOT EXISTS guards; safe to re-run.

ALTER TABLE migration_events
  DROP CONSTRAINT IF EXISTS migration_events_subject_type_check;
ALTER TABLE migration_events
  ADD CONSTRAINT migration_events_subject_type_check
  CHECK (subject_type = ANY (ARRAY['person'::text, 'tribe'::text, 'family'::text]));

ALTER TABLE migration_events
  ADD COLUMN IF NOT EXISTS kind text;
ALTER TABLE migration_events
  DROP CONSTRAINT IF EXISTS migration_events_kind_check;
ALTER TABLE migration_events
  ADD CONSTRAINT migration_events_kind_check
  CHECK (kind IS NULL OR kind = ANY (ARRAY[
    'yawm'::text, 'ghazwa'::text, 'sariya'::text,
    'nafir'::text, 'relocation'::text, 'famine'::text, 'other'::text
  ]));

CREATE INDEX IF NOT EXISTS migration_events_kind_idx ON migration_events (kind);
