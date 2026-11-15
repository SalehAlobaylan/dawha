-- Plan 028-C: dossier pilot seeds (المَتْن).
--
-- FIRST read this header. The honest order of operations is:
--   1. A reviewer uploads a page excerpt (معجم البلدان or a comparable
--      classical text) through the journey-05 upload flow, so `sources` +
--      `source_files` carry real bytes, a checksum and a storage key.
--   2. This file is then pointed at that upload:
--        psql ... -v source_id='<uuid>' -v reviewer_id='<uuid>' -f infra/local/seed-dossier-pilot.sql
--      and the passage `text_ar` values below are replaced with the actual
--      excerpts the upload produced (sequence/page/locator already aligned).
--
-- The defaults below point at the demo database's synthetic source
-- (30000000-…-0002, CC-BY-annotated) and demo reviewer so the file is
-- runnable offline for development. The passage texts are pilot stand-ins,
-- not excerpts of any real book; their job is to prove the dossier shape
-- (association → passage → accepted statement → spatial_evidence) before
-- the volume upload lands. Nothing here publishes anything: every
-- association is hand-reviewed `documented` with certainty `approximate`,
-- exactly as the plan requires.
--
-- Associations come FIRST: spatial_evidence cannot exist without one
-- (0006 CHECK requires an association or a migration event).
--
-- The pilot places are the demo database's actual public places
-- (db/seeds/001_demo.sql): الرياض, الأحساء, حجاز, العلا. The plan text
-- names مكة/المدينة/الطائف/الهفوف, which do not exist in the demo seed;
-- the correction is recorded here rather than silently reseeded.

\if :{?source_id}
\else
\set source_id '30000000-0000-0000-0000-000000000002'
\endif
\if :{?reviewer_id}
\else
\set reviewer_id '00000000-0000-0000-0000-000000000001'
\endif

-- Per pilot place, one documented association by the demo person.
-- Ids are deterministic pilot ids under the 'd' series so a re-run is a
-- no-op (ON CONFLICT DO NOTHING) and a seed listing is recognisable.
INSERT INTO geographic_associations
  (id, entity_type, entity_id, place_id, relation_type, time_from, time_to, status, certainty, source_id, created_by)
VALUES
  ('a0000000-0000-0000-0000-00000000d001', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', 'documented_in', '1180-01-01', '1210-12-31', 'documented', 'approximate', :'source_id'::uuid, :'reviewer_id'::uuid),
  ('a0000000-0000-0000-0000-00000000d002', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000002', 'documented_in', '1180-01-01', '1210-12-31', 'documented', 'approximate', :'source_id'::uuid, :'reviewer_id'::uuid),
  ('a0000000-0000-0000-0000-00000000d003', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000003', 'documented_in', '1180-01-01', '1210-12-31', 'documented', 'approximate', :'source_id'::uuid, :'reviewer_id'::uuid),
  ('a0000000-0000-0000-0000-00000000d004', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000004', 'documented_in', '1180-01-01', '1210-12-31', 'documented', 'approximate', :'source_id'::uuid, :'reviewer_id'::uuid)
ON CONFLICT (id) DO NOTHING;

-- One passage per pilot place, plus one needs_review passage for العلا so
-- the moderation band has something to show. Sequence numbers 901+ keep
-- them off the demo seed's seq 1 under the (source_id, sequence_number)
-- unique constraint. normalized_text_ar is the normalizer's output for the
-- text beside it (see 001_demo.sql's header note on the same discipline).
INSERT INTO source_passages
  (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES
  ('40000000-0000-0000-0000-00000000d001', :'source_id'::uuid, 901, 901, 'ص 901',
   'نص تجريبي: يذكر النص الرياض موضعاً مذكوراً لمسارات المالك.',
   'نص تجريبي يذكر النص الرياض موضعا مذكورا لمسارات المالك'),
  ('40000000-0000-0000-0000-00000000d002', :'source_id'::uuid, 902, 902, 'ص 902',
   'نص تجريبي: يذكر النص الأحساء موضعاً مذكوراً لمسارات المالك.',
   'نص تجريبي يذكر النص الاحساء موضعا مذكورا لمسارات المالك'),
  ('40000000-0000-0000-0000-00000000d003', :'source_id'::uuid, 903, 903, 'ص 903',
   'نص تجريبي: يذكر النص حجاز إقليماً مذكوراً لمسارات المالك.',
   'نص تجريبي يذكر النص حجاز اقليما مذكورا لمسارات المالك'),
  ('40000000-0000-0000-0000-00000000d004', :'source_id'::uuid, 904, 904, 'ص 904',
   'نص تجريبي: يذكر النص العلا موضعاً مذكوراً لمسارات المالك.',
   'نص تجريبي يذكر النص العلا موضعا مذكورا لمسارات المالك'),
  ('40000000-0000-0000-0000-00000000d005', :'source_id'::uuid, 905, 905, 'ص 905',
   'نص تجريبي بانتظار المراجعة: يذكر النص العلا موضعاً يحتاج إلى تدقيق.',
   'نص تجريبي بانتظار المراجعه يذكر النص العلا موضعا يحتاج الي تدقيق')
ON CONFLICT (id) DO NOTHING;

-- The accepted statements that name the same passages. extraction_method
-- lives on the statement (0003), and the pilot is `manual` because a
-- reviewer read these by hand.
INSERT INTO source_statements
  (id, source_id, source_passage_id, statement_text_ar, locator_ar, extraction_method, review_status, created_by)
VALUES
  ('50000000-0000-0000-0000-00000000d001', :'source_id'::uuid, '40000000-0000-0000-0000-00000000d001', 'يذكر النص أن عبدالله مذكور في الرياض.', 'ص 901', 'manual', 'accepted', :'reviewer_id'::uuid),
  ('50000000-0000-0000-0000-00000000d002', :'source_id'::uuid, '40000000-0000-0000-0000-00000000d002', 'يذكر النص أن عبدالله مذكور في الأحساء.', 'ص 902', 'manual', 'accepted', :'reviewer_id'::uuid),
  ('50000000-0000-0000-0000-00000000d003', :'source_id'::uuid, '40000000-0000-0000-0000-00000000d003', 'يذكر النص أن عبدالله مذكور في حجاز.', 'ص 903', 'manual', 'accepted', :'reviewer_id'::uuid),
  ('50000000-0000-0000-0000-00000000d004', :'source_id'::uuid, '40000000-0000-0000-0000-00000000d004', 'يذكر النص أن عبدالله مذكور في العلا.', 'ص 904', 'manual', 'accepted', :'reviewer_id'::uuid),
  ('50000000-0000-0000-0000-00000000d005', :'source_id'::uuid, '40000000-0000-0000-0000-00000000d005', 'مسودة على العلا تحتاج مراجعة.', 'ص 905', 'manual', 'needs_review', :'reviewer_id'::uuid)
ON CONFLICT (id) DO NOTHING;

-- The dossier rows: accepted for الرياض/الأحساء/حجاز/العلا, and one
-- needs_review for العلا behind the moderation band.
INSERT INTO spatial_evidence
  (id, geographic_association_id, source_passage_id, review_status, created_by)
VALUES
  ('a2000000-0000-0000-0000-00000000d001', 'a0000000-0000-0000-0000-00000000d001', '40000000-0000-0000-0000-00000000d001', 'accepted', :'reviewer_id'::uuid),
  ('a2000000-0000-0000-0000-00000000d002', 'a0000000-0000-0000-0000-00000000d002', '40000000-0000-0000-0000-00000000d002', 'accepted', :'reviewer_id'::uuid),
  ('a2000000-0000-0000-0000-00000000d003', 'a0000000-0000-0000-0000-00000000d003', '40000000-0000-0000-0000-00000000d003', 'accepted', :'reviewer_id'::uuid),
  ('a2000000-0000-0000-0000-00000000d004', 'a0000000-0000-0000-0000-00000000d004', '40000000-0000-0000-0000-00000000d004', 'accepted', :'reviewer_id'::uuid),
  ('a2000000-0000-0000-0000-00000000d005', 'a0000000-0000-0000-0000-00000000d004', '40000000-0000-0000-0000-00000000d005', 'needs_review', :'reviewer_id'::uuid)
ON CONFLICT (id) DO NOTHING;
