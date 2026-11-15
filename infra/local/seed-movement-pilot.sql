-- Plan 028-D: movement pilot seeds (رِحْل).
--
-- ~10 documented مسالِك for the demo person عبدالله بن محمد
-- (10000000-…-0001) between the demo database's actual public places
-- (db/seeds/001_demo.sql): الرياض, الأحساء, حجاز, العلا. Sourced, like the
-- dossier pilot, from the Phase-C upload after it lands:
--   psql ... -v source_id='<uuid>' -v reviewer_id='<uuid>' -f infra/local/seed-movement-pilot.sql
-- Defaults point at the demo database's synthetic source + demo reviewer so
-- the file runs offline. Every row is `documented` with certainty
-- `approximate`, `created_by` = reviewer — the route overlay already draws
-- platform_inferred migrations; this pilot feeds the documented subject
-- layer instead.
--
-- Ordered by time_from, the layer reads them as one itinerary; the ids are a
-- deterministic 'd2' series so a re-run is a no-op (ON CONFLICT DO NOTHING).

\if :{?source_id}
\else
\set source_id '30000000-0000-0000-0000-000000000002'
\endif
\if :{?reviewer_id}
\else
\set reviewer_id '00000000-0000-0000-0000-000000000001'
\endif

INSERT INTO migration_events
  (id, subject_type, subject_id, from_place_id, to_place_id, time_from, time_to, status, certainty, source_id, notes_ar, created_by)
VALUES
  ('a1000000-0000-0000-0000-00000000d201', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000003', '20000000-0000-0000-0000-000000000004', '1180-01-01', '1182-01-01', 'documented', 'approximate', :'source_id'::uuid, 'نص تجريبي: رحلة من حجاز إلى العلا.', :'reviewer_id'::uuid),
  ('a1000000-0000-0000-0000-00000000d202', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000004', '20000000-0000-0000-0000-000000000001', '1182-06-01', '1185-01-01', 'documented', 'approximate', :'source_id'::uuid, 'نص تجريبي: رحلة من العلا إلى الرياض.', :'reviewer_id'::uuid),
  ('a1000000-0000-0000-0000-00000000d203', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000002', '1185-03-01', '1187-01-01', 'documented', 'approximate', :'source_id'::uuid, 'نص تجريبي: رحلة من الرياض إلى الأحساء.', :'reviewer_id'::uuid),
  ('a1000000-0000-0000-0000-00000000d204', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000002', '20000000-0000-0000-0000-000000000001', '1187-06-01', '1189-01-01', 'documented', 'approximate', :'source_id'::uuid, 'نص تجريبي: عودة من الأحساء إلى الرياض.', :'reviewer_id'::uuid),
  ('a1000000-0000-0000-0000-00000000d205', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000003', '1190-01-01', '1192-01-01', 'documented', 'approximate', :'source_id'::uuid, 'نص تجريبي: رحلة من الرياض إلى حجاز.', :'reviewer_id'::uuid),
  ('a1000000-0000-0000-0000-00000000d206', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000003', '20000000-0000-0000-0000-000000000004', '1192-06-01', '1194-01-01', 'documented', 'approximate', :'source_id'::uuid, 'نص تجريبي: رحلة ثانية من حجاز إلى العلا.', :'reviewer_id'::uuid),
  ('a1000000-0000-0000-0000-00000000d207', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000004', '20000000-0000-0000-0000-000000000002', '1195-01-01', '1197-01-01', 'documented', 'approximate', :'source_id'::uuid, 'نص تجريبي: رحلة من العلا إلى الأحساء.', :'reviewer_id'::uuid),
  ('a1000000-0000-0000-0000-00000000d208', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000002', '20000000-0000-0000-0000-000000000003', '1198-01-01', '1200-01-01', 'documented', 'approximate', :'source_id'::uuid, 'نص تجريبي: رحلة من الأحساء إلى حجاز.', :'reviewer_id'::uuid),
  ('a1000000-0000-0000-0000-00000000d209', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000003', '20000000-0000-0000-0000-000000000001', '1201-01-01', '1203-01-01', 'documented', 'approximate', :'source_id'::uuid, 'نص تجريبي: عودة من حجاز إلى الرياض.', :'reviewer_id'::uuid),
  ('a1000000-0000-0000-0000-00000000d210', 'person', '10000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000001', '20000000-0000-0000-0000-000000000004', '1204-01-01', '1206-01-01', 'documented', 'approximate', :'source_id'::uuid, 'نص تجريبي: رحلة من الرياض إلى العلا.', :'reviewer_id'::uuid)
ON CONFLICT (id) DO NOTHING;
