-- The plan 014 retrieval measurement corpus.
--
-- It exists because the corpus db/seeds/001_demo.sql leaves behind is three
-- passages, and a comparison of three retrieval paths over three passages cannot
-- tell them apart. This file adds a corpus a labelled question set can be asked
-- against: 42 passages over 13 sources, 9 people, 3 families, 3 places, 2
-- published-tree versions' worth of nodes, 21 claims and 4 recorded migrations.
-- All 42 passages carry an accepted statement and are therefore retrievable.
--
-- WHY IT IS NOT IN db/seeds/
--
-- It was, and that was wrong. `make db-seed` applies everything in db/seeds/, so
-- putting it there put 42 synthetic passages of geography into every seeded
-- environment: the developer's demo database, the E2E stack, and
-- `make verify-full`. The first thing that broke was
-- apps/web/e2e/journeys/09-research-query.spec.ts, which asks a question about
-- nothing in the corpus and asserts the refusal path. Its query was a seven
-- character nonsense word, and the passage legs score on pg_trgm CHARACTER
-- trigrams with `WHERE lexical_score > 0` as the only filter - so against three
-- short passages it matched nothing and against forty-two long ones it shared a
-- trigram with three of them and the product answered. The journey was right and
-- the corpus was in the wrong place: measurement scaffolding had become the
-- product's demo data, and it was the demo data's job to be small.
--
-- So the corpus lives here, next to the labels that judge it, and
-- `make retrieval-report` provisions it into an isolated schema
-- (`dawha_retrieval_measurement`) and drops that schema when it finishes. The
-- command works on a database that has only db/seeds/001_demo.sql applied, adds
-- nothing to the database it is pointed at, and cannot leak into the E2E stack.
-- It is measurement data, so it is owned and provisioned by the measurement.
--
-- What a reader needs to know about it:
--
--   * `normalized_text_ar` is what `identity.NormalizeArabicName(text_ar)`
--     produces, not a hand-written approximation. Both passage legs compare a
--     normalized query against that column and the embedding backfill embeds that
--     exact text, so an approximation here is a row no query can ever match.
--     `TestRetrievalCorpusIsNormalizedByTheSameFunction` fails if any row here
--     disagrees with the normalizer.
--
--   * every source is tagged `{"synthetic":true}`, which is the marker
--     `cmd/embedding-backfill` scopes itself to. The measurement provisions this
--     file into its own schema, so there is nothing else in scope anyway, and the
--     marker is what keeps an ad-hoc `make db-embed` from reaching past it.
--
--   * every passage carries an accepted statement, because both passage legs
--     require one (`WHERE ... AND ss.id IS NOT NULL`). A passage without one is
--     invisible to the measurement, which would make the corpus read smaller than
--     it is.
--
--   * the people are composite and invented. No living person, no real
--     individual and no personal data appears here. The place names are real
--     regions; the people attached to them are not.
--
-- The labels that judge this corpus are the sibling file
-- retrieval_measurement_cases.jsonl. This file is the data and that file is the
-- labels, they are versioned together by RetrievalMeasurementVersion, and
-- changing one without the other invalidates the measurement.

SET client_min_messages = warning;

-- The owner every row below is attributed to.
--
-- The corpus creates its own rather than relying on db/seeds/001_demo.sql, and
-- the measurement schema does not apply that seed: it is hermetic, so the same
-- corpus is measured on a freshly migrated database as on a seeded one. This row
-- is what makes that work, and finding it missing is what proved the schema was
-- being built without the demo data - `people.created_by` is a foreign key and the
-- insert failed rather than silently attributing the corpus to nobody.
INSERT INTO users (id, email, display_name_ar)
VALUES ('00000000-0000-0000-0000-000000000001', 'measurement@dawha.local', 'باحث القياس')
ON CONFLICT (id) DO NOTHING;

INSERT INTO user_roles (user_id, role)
VALUES ('00000000-0000-0000-0000-000000000001', 'registered')
ON CONFLICT (user_id, role) DO NOTHING;

-- People. Every one of them is a composite for display and not a record.
INSERT INTO people (id, canonical_name_ar, normalized_name_ar, gender, birth_date_from, birth_date_to, death_date_from, death_date_to, identity_status, notes_ar, created_by)
VALUES
  ('10000000-0000-0000-0000-000000000101', 'سالم بن نافع', 'سالم بن نافع', 'male', '1100-01-01', '1130-12-31', '1140-01-01', '1180-12-31', 'reviewed', 'شخصية تركيبية لأغراض القياس، وليست سجلاً موثقاً.', '00000000-0000-0000-0000-000000000001'),
  ('10000000-0000-0000-0000-000000000102', 'مبارك بن سالم', 'مبارك بن سالم', 'male', '1125-01-01', '1160-12-31', '1170-01-01', '1215-12-31', 'reviewed', 'شخصية تركيبية لأغراض القياس، وليست سجلاً موثقاً.', '00000000-0000-0000-0000-000000000001'),
  ('10000000-0000-0000-0000-000000000103', 'هلال بن مبارك', 'هلال بن مبارك', 'male', '1150-01-01', '1185-12-31', '1195-01-01', '1240-12-31', 'reviewed', 'شخصية تركيبية لأغراض القياس، وليست سجلاً موثقاً.', '00000000-0000-0000-0000-000000000001'),
  ('10000000-0000-0000-0000-000000000104', 'راشد بن هلال', 'راشد بن هلال', 'male', '1175-01-01', '1210-12-31', '1220-01-01', '1265-12-31', 'reviewed', 'شخصية تركيبية لأغراض القياس، وليست سجلاً موثقاً.', '00000000-0000-0000-0000-000000000001'),
  ('10000000-0000-0000-0000-000000000105', 'أسد بن راشد', 'اسد بن راشد', 'male', '1200-01-01', '1235-12-31', '1245-01-01', '1290-12-31', 'reviewed', 'شخصية تركيبية لأغراض القياس، وليست سجلاً موثقاً.', '00000000-0000-0000-0000-000000000001'),
  ('10000000-0000-0000-0000-000000000106', 'صفوان بن أسد', 'صفوان بن اسد', 'male', '1225-01-01', '1260-12-31', '1270-01-01', '1315-12-31', 'reviewed', 'شخصية تركيبية لأغراض القياس، وليست سجلاً موثقاً.', '00000000-0000-0000-0000-000000000001'),
  ('10000000-0000-0000-0000-000000000107', 'ناصر بن صفوان', 'ناصر بن صفوان', 'male', '1250-01-01', '1285-12-31', '1295-01-01', '1340-12-31', 'reviewed', 'شخصية تركيبية لأغراض القياس، وليست سجلاً موثقاً.', '00000000-0000-0000-0000-000000000001'),
  ('10000000-0000-0000-0000-000000000108', 'مهند بن ناصر', 'مهند بن ناصر', 'male', '1275-01-01', '1310-12-31', '1320-01-01', '1360-12-31', 'reviewed', 'شخصية تركيبية لأغراض القياس، وليست سجلاً موثقاً.', '00000000-0000-0000-0000-000000000001'),
  ('10000000-0000-0000-0000-000000000109', 'ميمون بن مهند', 'ميمون بن مهند', 'male', '1300-01-01', '1335-12-31', '1345-01-01', '1380-12-31', 'unreviewed', 'شخصية تركيبية لأغراض القياس، وليست سجلاً موثقاً.', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (id) DO NOTHING;

-- One alias, and it is the only row in the corpus carrying this spelling.
-- It is here so the question set has a lexical control that the `ILIKE` leg
-- can win outright.
INSERT INTO person_aliases (person_id, value_ar, normalized_value_ar, alias_type)
VALUES ('10000000-0000-0000-0000-000000000101', 'سالم بن نافع بن عـنبر', 'سالم بن نافع بن عنبر', 'source_spelling')
ON CONFLICT DO NOTHING;

-- Places. Six of them, all declared here.
--
-- The last three share their names with places db/seeds/001_demo.sql declares
-- and they are NOT the same rows. When this corpus lived in db/seeds/ it referred
-- to the demo seed's places by id, which is a dependency it no longer has: the
-- measurement schema applies no seed at all, so a referenced place would simply be
-- absent and the insert would fail on its foreign key. Declaring them here is what
-- makes the corpus self-contained, and self-contained is the point - the
-- measurement runs the same corpus whatever else the database happens to hold.
INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, geometry, visibility)
VALUES
  ('20000000-0000-0000-0000-000000000101', 'نجد', 'نجد', 'region', ST_SetSRID(ST_MakePoint(42.0, 23.0), 4326), 'public'),
  ('20000000-0000-0000-0000-000000000102', 'اليمن', 'اليمن', 'region', ST_SetSRID(ST_MakePoint(44.2, 15.3), 4326), 'public'),
  ('20000000-0000-0000-0000-000000000103', 'اليمامة', 'اليمامه', 'region', ST_SetSRID(ST_MakePoint(46.8, 25.0), 4326), 'public'),
  ('20000000-0000-0000-0000-000000000104', 'الرياض', 'الرياض', 'city', ST_SetSRID(ST_MakePoint(46.6753, 24.7136), 4326), 'public'),
  ('20000000-0000-0000-0000-000000000105', 'الأحساء', 'الاحساء', 'region', ST_SetSRID(ST_MakePoint(49.5658, 25.3647), 4326), 'public'),
  ('20000000-0000-0000-0000-000000000106', 'حجاز', 'حجاز', 'region', ST_SetSRID(ST_MakePoint(39.1925, 21.4858), 4326), 'public')
ON CONFLICT (id) DO NOTHING;

INSERT INTO families (id, canonical_name_ar, normalized_name_ar, description_ar, origin_place_id, created_by, visibility)
VALUES
  ('c0000000-0000-0000-0000-000000000101', 'أسرة الحارثي', 'اسره الحارثي', 'أسرة تركيبية لأغراض القياس.', '20000000-0000-0000-0000-000000000104', '00000000-0000-0000-0000-000000000001', 'public'),
  ('c0000000-0000-0000-0000-000000000102', 'أسرة النصري', 'اسره النصري', 'أسرة تركيبية لأغراض القياس.', '20000000-0000-0000-0000-000000000105', '00000000-0000-0000-0000-000000000001', 'public'),
  ('c0000000-0000-0000-0000-000000000103', 'أسرة الزبيري', 'اسره الزبيري', 'أسرة تركيبية لأغراض القياس.', '20000000-0000-0000-0000-000000000106', '00000000-0000-0000-0000-000000000001', 'public')
ON CONFLICT (id) DO NOTHING;

INSERT INTO branches (id, family_id, canonical_name_ar, normalized_name_ar, created_by, visibility)
VALUES
  ('c1000000-0000-0000-0000-000000000101', 'c0000000-0000-0000-0000-000000000101', 'فرع سالم', 'فرع سالم', '00000000-0000-0000-0000-000000000001', 'public'),
  ('c1000000-0000-0000-0000-000000000102', 'c0000000-0000-0000-0000-000000000101', 'فرع هلال', 'فرع هلال', '00000000-0000-0000-0000-000000000001', 'public'),
  ('c1000000-0000-0000-0000-000000000103', 'c0000000-0000-0000-0000-000000000102', 'فرع صفوان', 'فرع صفوان', '00000000-0000-0000-0000-000000000001', 'public')
ON CONFLICT (id) DO NOTHING;

-- Sources. All twelve are public and all twelve are marked synthetic.
INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, citation_ar, location_ar, visibility, metadata)
VALUES
  ('30000000-0000-0000-0000-000000000101', 'معجم الأنساب المختصر', 'مؤلف تجريبي', 'manuscript', '1150-01-01', 'الجزء الأول، ص ٤٤', 'مخطوط تجريبي', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000102', 'تاريخ المدن والقرى', 'مؤلف تجريبي', 'book', '1190-01-01', 'المجلد الأول، ص ١٢', 'طبعة تجريبية', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000103', 'رسالة في المواضع', 'مؤلف تجريبي', 'book', '1220-01-01', 'ص ٧', 'مخطوط تجريبي', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000104', 'دفتر الوقفيات', 'مؤلف تجريبي', 'manuscript', '1240-01-01', 'ص ٣١', 'مخطوط تجريبي', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000105', 'سجل القادة', 'مؤلف تجريبي', 'manuscript', '1210-01-01', 'ص ٩', 'مخطوط تجريبي', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000106', 'معجم قبائل الحجاز', 'مؤلف تجريبي', 'book', '1260-01-01', 'الجزء الثاني، ص ٦٦', 'طبعة تجريبية', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000107', 'كتاب المواسم', 'مؤلف تجريبي', 'book', '1280-01-01', 'ص ١٨', 'طبعة تجريبية', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000108', 'مذكرة عن الأوقاف', 'مؤلف تجريبي', 'manuscript', '1230-01-01', 'ص ٥', 'مخطوط تجريبي', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000109', 'نسخة من نقش قديم', 'مؤلف تجريبي', 'manuscript', '1170-01-01', 'صفحة واحدة', 'نقش تجريبي', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000110', 'تقرير الوافدين', 'مؤلف تجريبي', 'book', '1300-01-01', 'ص ٢٢', 'طبعة تجريبية', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000111', 'مجموع خطابات', 'مؤلف تجريبي', 'manuscript', '1250-01-01', 'ص ٤٠', 'مخطوط تجريبي', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000112', 'مختصر الجغرافيا', 'مؤلف تجريبي', 'book', '1310-01-01', 'ص ١٥', 'طبعة تجريبية', 'public', '{"synthetic":true}'),
  ('30000000-0000-0000-0000-000000000113', 'مختصر الأنساب المتصل', 'مؤلف تجريبي', 'book', '1320-01-01', 'ص ٢', 'طبعة تجريبية', 'public', '{"synthetic":true}')
ON CONFLICT (id) DO NOTHING;

-- Passages and their accepted statements. Both text columns come from one
-- string, so the pair cannot drift.
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000101', '30000000-0000-0000-0000-000000000101', 1, '44', 'ص 44', 'يفرد معجم الأنساب سالم بن نافع تحت أسرة الحارثي، ويذكر أن نسبه ثابت في المصادر القديمة.', 'يفرد معجم الانساب سالم بن نافع تحت اسره الحارثي ويذكر ان نسبه ثابت في المصادر القديمه')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000102', '30000000-0000-0000-0000-000000000101', 2, '44', 'ص 44', 'ويضيف المعجم أن مبارك بن سالم من بيت سالم، وأن ترتيبه يليه في النسب نفسه.', 'ويضيف المعجم ان مبارك بن سالم من بيت سالم وان ترتيبه يليه في النسب نفسه')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000103', '30000000-0000-0000-0000-000000000106', 1, '66', 'ص 66', 'يسمي معجم قبائل الحجاز هلال بن مبارك ابن المبارك، ويؤثر هذا اللفظ في الروايات اللاحقة.', 'يسمي معجم قبائل الحجاز هلال بن مبارك ابن المبارك ويؤثر هذا اللفظ في الروايات اللاحقه')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000104', '30000000-0000-0000-0000-000000000105', 1, '9', 'ص 9', 'ورد في سجل القادة أن راشد بن هلال تولى الخراب في سنة أربع وخمسين ومئة.', 'ورد في سجل القاده ان راشد بن هلال تولي الخراب في سنه اربع وخمسين ومئه')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000105', '30000000-0000-0000-0000-000000000104', 1, '31', 'ص 31', 'أسجل دفتر الوقفيات أن أسد بن راشد أوقف بئرا في الحارة، وعرف الوقف باسمه.', 'اسجل دفتر الوقفيات ان اسد بن راشد اوقف بئرا في الحاره وعرف الوقف باسمه')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000106', '30000000-0000-0000-0000-000000000106', 2, '66', 'ص 66', 'ينسب المعجم صفوان بن أسد إلى أسرة النصري، ويصف السلالة بأنها مستقلة عن جيرانها.', 'ينسب المعجم صفوان بن اسد الي اسره النصري ويصف السلاله بانها مستقله عن جيرانها')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000107', '30000000-0000-0000-0000-000000000111', 1, '40', 'ص 40', 'تصف الخطابات رسالة ناصر بن صفوان، وتذكر في صدرها أن أباها صفوان بن أسد.', 'تصف الخطابات رساله ناصر بن صفوان وتذكر في صدرها ان اباها صفوان بن اسد')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000108', '30000000-0000-0000-0000-000000000110', 1, '22', 'ص 22', 'يفيد تقرير الوافدين أن مهند بن ناصر قدم إلى الحجاز في سنة سبع وسبعين ومئة.', 'يفيد تقرير الوافدين ان مهند بن ناصر قدم الي الحجاز في سنه سبع وسبعين ومئه')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000109', '30000000-0000-0000-0000-000000000102', 1, '12', 'ص 12', 'يسجل تاريخ المدن ميمون بن مهند بين وافدي سنة تسع ومئة، لا في غيرها.', 'يسجل تاريخ المدن ميمون بن مهند بين وافدي سنه تسع ومئه لا في غيرها')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000110', '30000000-0000-0000-0000-000000000103', 1, '7', 'ص 7', 'تذكر رسالة المواضع أن ناصر بن صفوان ومهند بن ناصر ينتميان إلى أسرة الزبيري معا.', 'تذكر رساله المواضع ان ناصر بن صفوان ومهند بن ناصر ينتميان الي اسره الزبيري معا')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000111', '30000000-0000-0000-0000-000000000103', 2, '7', 'ص 7', 'وتضيف الرسالة أن ميمون بن مهند يدرج في النسب نفسه، على رواية أقدم.', 'وتضيف الرساله ان ميمون بن مهند يدرج في النسب نفسه علي روايه اقدم')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000112', '30000000-0000-0000-0000-000000000102', 2, '12', 'ص 12', 'يصف تاريخ المدن مدينة الرياض بأنها أعظم مدن نجد في هذا المجلد.', 'يصف تاريخ المدن مدينه الرياض بانها اعظم مدن نجد في هذا المجلد')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000113', '30000000-0000-0000-0000-000000000102', 3, '13', 'ص 13', 'يحكم تاريخ المدن بأن الأحساء تقع شرق نجد، وأن حدودها لا تتجاوز الفرات.', 'يحكم تاريخ المدن بان الاحساء تقع شرق نجد وان حدودها لا تتجاوز الفرات')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000114', '30000000-0000-0000-0000-000000000103', 3, '7', 'ص 7', 'تحدد رسالة المواضع موقع العلا شمال حجاز، على بُعد يومين من ساحل البحر الأحمر.', 'تحدد رساله المواضع موقع العلا شمال حجاز علي بعد يومين من ساحل البحر الاحمر')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000115', '30000000-0000-0000-0000-000000000112', 1, '15', 'ص 15', 'يضع مختصر الجغرافيا اليمن في جنوب الجزيرة، ويجعل البحر حدها الغربي.', 'يضع مختصر الجغرافيا اليمن في جنوب الجزيره ويجعل البحر حدها الغربي')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000116', '30000000-0000-0000-0000-000000000107', 1, '18', 'ص 18', 'يعدد كتاب المواسم مواسم جدة ثم الرياض ثم الأحساء، بهذا الترتيب.', 'يعدد كتاب المواسم مواسم جده ثم الرياض ثم الاحساء بهذا الترتيب')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000117', '30000000-0000-0000-0000-000000000108', 1, '5', 'ص 5', 'تورد مذكرة الأوقاف عبارة سطر مولى على حاشيتها بخط اليد، وهي عبارة لا ترد في غيرها.', 'تورد مذكره الاوقاف عباره سطر مولي علي حاشيتها بخط اليد وهي عباره لا ترد في غيرها')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000118', '30000000-0000-0000-0000-000000000109', 1, '1', 'ص 1', 'يحمل النقش عبارة المفرد على حاشيته السفلى بخط مختلف عن خط النص.', 'يحمل النقش عباره المفرد علي حاشيته السفلي بخط مختلف عن خط النص')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000119', '30000000-0000-0000-0000-000000000111', 2, '41', 'ص 41', 'تختم الخطابات بعبارة آخر ما كتب، وهي خاتمة المجمع كله.', 'تختم الخطابات بعباره اخر ما كتب وهي خاتمه المجمع كله')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000120', '30000000-0000-0000-0000-000000000104', 2, '32', 'ص 32', 'يعود دفتر الوقفيات إلى عبارة ختم الدفتر في هامش كل صفحة، فتتكرر في السجل كله.', 'يعود دفتر الوقفيات الي عباره ختم الدفتر في هامش كل صفحه فتتكرر في السجل كله')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000121', '30000000-0000-0000-0000-000000000110', 2, '23', 'ص 23', 'يذكر تقرير الوافدين أن مهند بن ناصر كان من أكثر الوافدين صلة بالتجار.', 'يذكر تقرير الوافدين ان مهند بن ناصر كان من اكثر الوافدين صله بالتجار')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000122', '30000000-0000-0000-0000-000000000106', 3, '67', 'ص 67', 'ويختم معجم قبائل الحجاز فصله بذكر أن هلال بن مبارك لم يعقب ذكري.', 'ويختم معجم قبائل الحجاز فصله بذكر ان هلال بن مبارك لم يعقب ذكري')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000123', '30000000-0000-0000-0000-000000000105', 2, '10', 'ص 10', 'أضاف سجل القادة سطرا عن راشد بن هلال، لكنه لم يذكر سنة توليه.', 'اضاف سجل القاده سطرا عن راشد بن هلال لكنه لم يذكر سنه توليه')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000124', '30000000-0000-0000-0000-000000000102', 4, '14', 'ص 14', 'ويضيف تاريخ المدن صفحة عن ناصر بن صفوان، دون أن يحدد سنة قدومه.', 'ويضيف تاريخ المدن صفحه عن ناصر بن صفوان دون ان يحدد سنه قدومه')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000125', '30000000-0000-0000-0000-000000000101', 3, '45', 'ص 45', 'يختم معجم الأنساب بقسم في الألقاب، لا صلة له بالنسب.', 'يختم معجم الانساب بقسم في الالقاب لا صله له بالنسب')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000126', '30000000-0000-0000-0000-000000000103', 4, '8', 'ص 8', 'تسجل رسالة المواضع انتقال راشد بن هلال من الأحساء إلى اليمامة في العقد الثاني.', 'تسجل رساله المواضع انتقال راشد بن هلال من الاحساء الي اليمامه في العقد الثاني')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000127', '30000000-0000-0000-0000-000000000110', 3, '24', 'ص 24', 'ويضيف تقرير الوافدين أن ناصر بن صفوان استقر في اليمن بعد رحلته من نجد.', 'ويضيف تقرير الوافدين ان ناصر بن صفوان استقر في اليمن بعد رحلته من نجد')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000128', '30000000-0000-0000-0000-000000000112', 2, '16', 'ص 16', 'يورد مختصر الجغرافيا أن ميمون بن مهند غادر اليمامة متجها إلى الحجاز.', 'يورد مختصر الجغرافيا ان ميمون بن مهند غادر اليمامه متجها الي الحجاز')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000129', '30000000-0000-0000-0000-000000000107', 2, '19', 'ص 19', 'يعلق كتاب المواسم على انتقال أسد بن راشد، ويصفه بأنه انتقال داخلي قصير.', 'يعلق كتاب المواسم علي انتقال اسد بن راشد ويصفه بانه انتقال داخلي قصير')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000130', '30000000-0000-0000-0000-000000000102', 5, '15', 'ص 15', 'تشهد صفحة في تاريخ المدن بأن المهجرين إلى الأحساء جاءوا في موجة واحدة.', 'تشهد صفحه في تاريخ المدن بان المهجرين الي الاحساء جاءوا في موجه واحده')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000135', '30000000-0000-0000-0000-000000000113', 1, '2', 'ص 2', 'يبدأ مختصر الأنساب بسالم بن نافع، فيذكر أن أبا مبارك بن سالم هو سالم بن نافع.', 'يبدا مختصر الانساب بسالم بن نافع فيذكر ان ابا مبارك بن سالم هو سالم بن نافع')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000136', '30000000-0000-0000-0000-000000000113', 2, '2', 'ص 2', 'ثم يقرر أن أبا هلال بن مبارك هو مبارك بن سالم، ويسرد السلسلة إلى ذلك.', 'ثم يقرر ان ابا هلال بن مبارك هو مبارك بن سالم ويسرد السلسله الي ذلك')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000137', '30000000-0000-0000-0000-000000000113', 3, '2', 'ص 2', 'ويلحق بأن أبا راشد بن هلال هو هلال بن مبارك، ويؤيد ذلك بسند مذكور.', 'ويلحق بان ابا راشد بن هلال هو هلال بن مبارك ويؤيد ذلك بسند مذكور')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000138', '30000000-0000-0000-0000-000000000113', 4, '2', 'ص 2', 'ثم يثبت أن أبا أسد بن راشد هو راشد بن هلال، ويجعل ذلك مذهب المختصر.', 'ثم يثبت ان ابا اسد بن راشد هو راشد بن هلال ويجعل ذلك مذهب المختصر')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000139', '30000000-0000-0000-0000-000000000113', 5, '2', 'ص 2', 'بعد ذلك يقرر أن أبا صفوان بن أسد هو أسد بن راشد.', 'بعد ذلك يقرر ان ابا صفوان بن اسد هو اسد بن راشد')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000140', '30000000-0000-0000-0000-000000000113', 6, '3', 'ص 3', 'ثم يربط المختصر بين ناصر بن صفوان وأبيه صفوان بن أسد، ويسمّي الوسيط.', 'ثم يربط المختصر بين ناصر بن صفوان وابيه صفوان بن اسد ويسمي الوسيط')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000141', '30000000-0000-0000-0000-000000000113', 7, '3', 'ص 3', 'ويلحق بذلك أن أبا مهند بن ناصر هو ناصر بن صفوان.', 'ويلحق بذلك ان ابا مهند بن ناصر هو ناصر بن صفوان')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000142', '30000000-0000-0000-0000-000000000113', 8, '3', 'ص 3', 'ويختم بأن أبا ميمون بن مهند هو مهند بن ناصر، على رواية أقدم من الرواية الأولى.', 'ويختم بان ابا ميمون بن مهند هو مهند بن ناصر علي روايه اقدم من الروايه الاولي')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000131', '30000000-0000-0000-0000-000000000104', 3, '33', 'ص 33', 'يصف دفتر الوقفيات بئر الوقف بأنها في سفح الجبل، ويذكر أن أهل القرية يعرفونها باسم آخر.', 'يصف دفتر الوقفيات بئر الوقف بانها في سفح الجبل ويذكر ان اهل القريه يعرفونها باسم اخر')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000132', '30000000-0000-0000-0000-000000000112', 3, '17', 'ص 17', 'ينسب مختصر الجغرافيا اليمامة إلى شرق نجد، ويجعل بينهما برية طويلة.', 'ينسب مختصر الجغرافيا اليمامه الي شرق نجد ويجعل بينهما بريه طويله')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000133', '30000000-0000-0000-0000-000000000106', 4, '68', 'ص 68', 'يعرّف معجم قبائل الحجاز لفظ النصري بأنه نسبة إلى مرفأ بعينه.', 'يعرف معجم قبائل الحجاز لفظ النصري بانه نسبه الي مرفا بعينه')
ON CONFLICT (id) DO NOTHING;
INSERT INTO source_passages (id, source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES ('40000000-0000-0000-0000-000000000134', '30000000-0000-0000-0000-000000000105', 3, '11', 'ص 11', 'يسرد سجل القادة أسماء من تولوا الخراب، وفيه اسم أسد بن راشد.', 'يسرد سجل القاده اسماء من تولوا الخراب وفيه اسم اسد بن راشد')
ON CONFLICT (id) DO NOTHING;

INSERT INTO source_statements (id, source_id, source_passage_id, statement_text_ar, locator_ar, extraction_method, review_status, created_by)
VALUES
  ('50000000-0000-0000-0000-000000000101', '30000000-0000-0000-0000-000000000101', '40000000-0000-0000-0000-000000000101', 'سالم بن نافع من أسرة الحارثي.', 'ص 44', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000102', '30000000-0000-0000-0000-000000000101', '40000000-0000-0000-0000-000000000102', 'مبارك بن سالم من بيت سالم في أسرة الحارثي.', 'ص 44', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000103', '30000000-0000-0000-0000-000000000106', '40000000-0000-0000-0000-000000000103', 'هلال بن مبارك ابن مبارك بن سالم.', 'ص 66', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000104', '30000000-0000-0000-0000-000000000105', '40000000-0000-0000-0000-000000000104', 'راشد بن هلال تولى الخراب.', 'ص 9', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000105', '30000000-0000-0000-0000-000000000104', '40000000-0000-0000-0000-000000000105', 'أسد بن راشد أوقف بئرا.', 'ص 31', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000106', '30000000-0000-0000-0000-000000000106', '40000000-0000-0000-0000-000000000106', 'صفوان بن أسد من أسرة النصري.', 'ص 66', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000107', '30000000-0000-0000-0000-000000000111', '40000000-0000-0000-0000-000000000107', 'ناصر بن صفوان ابن صفوان بن أسد.', 'ص 40', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000108', '30000000-0000-0000-0000-000000000110', '40000000-0000-0000-0000-000000000108', 'مهند بن ناصر قدم إلى الحجاز.', 'ص 22', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000109', '30000000-0000-0000-0000-000000000102', '40000000-0000-0000-0000-000000000109', 'ميمون بن مهند من وافدي سنة تسع ومئة.', 'ص 12', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000110', '30000000-0000-0000-0000-000000000103', '40000000-0000-0000-0000-000000000110', 'ناصر بن صفوان ومهند بن ناصر من أسرة الزبيري.', 'ص 7', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000111', '30000000-0000-0000-0000-000000000103', '40000000-0000-0000-0000-000000000111', 'ميمون بن مهند من أسرة الزبيري.', 'ص 7', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000112', '30000000-0000-0000-0000-000000000102', '40000000-0000-0000-0000-000000000112', 'الرياض أعظم مدن نجد في تاريخ المدن.', 'ص 12', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000113', '30000000-0000-0000-0000-000000000102', '40000000-0000-0000-0000-000000000113', 'الأحساء تقع شرق نجد.', 'ص 13', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000114', '30000000-0000-0000-0000-000000000103', '40000000-0000-0000-0000-000000000114', 'العلا في شمال حجاز.', 'ص 7', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000115', '30000000-0000-0000-0000-000000000112', '40000000-0000-0000-0000-000000000115', 'اليمن في جنوب الجزيرة.', 'ص 15', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000116', '30000000-0000-0000-0000-000000000107', '40000000-0000-0000-0000-000000000116', 'مواسم كتاب المواسم جدة ثم الرياض ثم الأحساء.', 'ص 18', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000117', '30000000-0000-0000-0000-000000000108', '40000000-0000-0000-0000-000000000117', 'عبارة سطر مولى وردت في هامش المذكرة.', 'ص 5', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000118', '30000000-0000-0000-0000-000000000109', '40000000-0000-0000-0000-000000000118', 'النقش يحمل عبارة المفرد على حاشيته.', 'ص 1', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000119', '30000000-0000-0000-0000-000000000111', '40000000-0000-0000-0000-000000000119', 'الخطابات تختم بعبارة آخر ما كتب.', 'ص 41', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000120', '30000000-0000-0000-0000-000000000104', '40000000-0000-0000-0000-000000000120', 'عبارة ختم الدفتر تتكرر في هامش كل صفحة.', 'ص 32', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000121', '30000000-0000-0000-0000-000000000110', '40000000-0000-0000-0000-000000000121', 'مهند بن ناصر من أكثر الوافدين صلة بالتجار.', 'ص 23', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000122', '30000000-0000-0000-0000-000000000106', '40000000-0000-0000-0000-000000000122', 'هلال بن مبارك لم يعقب في المعجم.', 'ص 67', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000123', '30000000-0000-0000-0000-000000000105', '40000000-0000-0000-0000-000000000123', 'السجل يضيف سطرا عن راشد بن هلال بلا سنة.', 'ص 10', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000124', '30000000-0000-0000-0000-000000000102', '40000000-0000-0000-0000-000000000124', 'تاريخ المدن يذكر ناصر بن صفوان بلا سنة.', 'ص 14', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000125', '30000000-0000-0000-0000-000000000101', '40000000-0000-0000-0000-000000000125', 'قسم الألقاب في معجم الأنساب.', 'ص 45', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000126', '30000000-0000-0000-0000-000000000103', '40000000-0000-0000-0000-000000000126', 'راشد بن هلال انتقل من الأحساء إلى اليمامة.', 'ص 8', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000127', '30000000-0000-0000-0000-000000000110', '40000000-0000-0000-0000-000000000127', 'ناصر بن صفوان استقر في اليمن من نجد.', 'ص 24', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000128', '30000000-0000-0000-0000-000000000112', '40000000-0000-0000-0000-000000000128', 'ميمون بن مهند غادر اليمامة إلى الحجاز.', 'ص 16', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000129', '30000000-0000-0000-0000-000000000107', '40000000-0000-0000-0000-000000000129', 'انتقال أسد بن راشد كان داخليا قصيرا.', 'ص 19', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000130', '30000000-0000-0000-0000-000000000102', '40000000-0000-0000-0000-000000000130', 'الموجة الأولى من المهاجرين إلى الأحساء.', 'ص 15', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000135', '30000000-0000-0000-0000-000000000113', '40000000-0000-0000-0000-000000000135', 'أبا مبارك بن سالم هو سالم بن نافع.', 'ص 2', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000136', '30000000-0000-0000-0000-000000000113', '40000000-0000-0000-0000-000000000136', 'أبا هلال بن مبارك هو مبارك بن سالم.', 'ص 2', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000137', '30000000-0000-0000-0000-000000000113', '40000000-0000-0000-0000-000000000137', 'أبا راشد بن هلال هو هلال بن مبارك.', 'ص 2', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000138', '30000000-0000-0000-0000-000000000113', '40000000-0000-0000-0000-000000000138', 'أبا أسد بن راشد هو راشد بن هلال.', 'ص 2', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000139', '30000000-0000-0000-0000-000000000113', '40000000-0000-0000-0000-000000000139', 'أبا صفوان بن أسد هو أسد بن راشد.', 'ص 2', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000140', '30000000-0000-0000-0000-000000000113', '40000000-0000-0000-0000-000000000140', 'أبا ناصر بن صفوان هو صفوان بن أسد.', 'ص 3', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000141', '30000000-0000-0000-0000-000000000113', '40000000-0000-0000-0000-000000000141', 'أبا مهند بن ناصر هو ناصر بن صفوان.', 'ص 3', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000142', '30000000-0000-0000-0000-000000000113', '40000000-0000-0000-0000-000000000142', 'أبا ميمون بن مهند هو مهند بن ناصر.', 'ص 3', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000131', '30000000-0000-0000-0000-000000000104', '40000000-0000-0000-0000-000000000131', 'بئر الوقف في سفح الجبل.', 'ص 33', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000132', '30000000-0000-0000-0000-000000000112', '40000000-0000-0000-0000-000000000132', 'اليمامة شرق نجد.', 'ص 17', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000133', '30000000-0000-0000-0000-000000000106', '40000000-0000-0000-0000-000000000133', 'النصري نسبة إلى مرفأ بعينه.', 'ص 68', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001'),
  ('50000000-0000-0000-0000-000000000134', '30000000-0000-0000-0000-000000000105', '40000000-0000-0000-0000-000000000134', 'سجل القادة يفرد أسد بن راشد.', 'ص 11', 'manual', 'accepted', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (id) DO NOTHING;

-- Claims: the edges the graph arm walks. `member_of` is the hop a family
-- question has to make, `father_of` is the person chain, and `migrated_to`
-- is the place hop. Every claim is backed by an accepted statement, because
-- `branch_claims` and `evidence_connection` read their evidence through the
-- same accepted-statement rule the passage legs use.
INSERT INTO claims (id, subject_type, subject_id, predicate, object_type, object_id, place_id, status, notes_ar, created_by)
VALUES
  ('60000000-0000-0000-0000-000000000101', 'person', '10000000-0000-0000-0000-000000000101', 'member_of', 'family', 'c0000000-0000-0000-0000-000000000101', NULL, 'supported', 'سالم بن نافع من أسرة الحارثي.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000102', 'person', '10000000-0000-0000-0000-000000000102', 'member_of', 'family', 'c0000000-0000-0000-0000-000000000101', NULL, 'supported', 'مبارك بن سالم من أسرة الحارثي.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000103', 'person', '10000000-0000-0000-0000-000000000103', 'member_of', 'family', 'c0000000-0000-0000-0000-000000000101', NULL, 'supported', 'هلال بن مبارك من أسرة الحارثي.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000104', 'person', '10000000-0000-0000-0000-000000000104', 'member_of', 'family', 'c0000000-0000-0000-0000-000000000101', NULL, 'supported', 'راشد بن هلال من أسرة الحارثي.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000105', 'person', '10000000-0000-0000-0000-000000000105', 'member_of', 'family', 'c0000000-0000-0000-0000-000000000101', NULL, 'supported', 'أسد بن راشد من أسرة الحارثي.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000106', 'person', '10000000-0000-0000-0000-000000000106', 'member_of', 'family', 'c0000000-0000-0000-0000-000000000102', NULL, 'supported', 'صفوان بن أسد من أسرة النصري.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000107', 'person', '10000000-0000-0000-0000-000000000107', 'member_of', 'family', 'c0000000-0000-0000-0000-000000000103', NULL, 'supported', 'ناصر بن صفوان من أسرة الزبيري.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000108', 'person', '10000000-0000-0000-0000-000000000108', 'member_of', 'family', 'c0000000-0000-0000-0000-000000000103', NULL, 'supported', 'مهند بن ناصر من أسرة الزبيري.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000109', 'person', '10000000-0000-0000-0000-000000000109', 'member_of', 'family', 'c0000000-0000-0000-0000-000000000103', NULL, 'contested', 'ميمون بن مهند من أسرة الزبيري، رواية أقدم.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000110', 'person', '10000000-0000-0000-0000-000000000102', 'father_of', 'person', '10000000-0000-0000-0000-000000000101', NULL, 'supported', 'مبارك بن سالم ابن سالم بن نافع.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000111', 'person', '10000000-0000-0000-0000-000000000103', 'father_of', 'person', '10000000-0000-0000-0000-000000000102', NULL, 'supported', 'هلال بن مبارك ابن مبارك بن سالم.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000112', 'person', '10000000-0000-0000-0000-000000000104', 'father_of', 'person', '10000000-0000-0000-0000-000000000103', NULL, 'supported', 'راشد بن هلال ابن هلال بن مبارك.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000113', 'person', '10000000-0000-0000-0000-000000000105', 'father_of', 'person', '10000000-0000-0000-0000-000000000104', NULL, 'supported', 'أسد بن راشد ابن راشد بن هلال.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000114', 'person', '10000000-0000-0000-0000-000000000106', 'father_of', 'person', '10000000-0000-0000-0000-000000000105', NULL, 'supported', 'صفوان بن أسد ابن أسد بن راشد.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000115', 'person', '10000000-0000-0000-0000-000000000107', 'father_of', 'person', '10000000-0000-0000-0000-000000000106', NULL, 'supported', 'ناصر بن صفوان ابن صفوان بن أسد.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000116', 'person', '10000000-0000-0000-0000-000000000108', 'father_of', 'person', '10000000-0000-0000-0000-000000000107', NULL, 'supported', 'مهند بن ناصر ابن ناصر بن صفوان.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000117', 'person', '10000000-0000-0000-0000-000000000109', 'father_of', 'person', '10000000-0000-0000-0000-000000000108', NULL, 'unresolved', 'ميمون بن مهند ابن مهند بن ناصر، غير محسوم.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000118', 'person', '10000000-0000-0000-0000-000000000104', 'migrated_to', 'place', '20000000-0000-0000-0000-000000000103', '20000000-0000-0000-0000-000000000105', 'inferred', 'راشد بن هلال انتقل من الأحساء إلى اليمامة.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000119', 'person', '10000000-0000-0000-0000-000000000107', 'migrated_to', 'place', '20000000-0000-0000-0000-000000000102', '20000000-0000-0000-0000-000000000101', 'inferred', 'ناصر بن صفوان انتقل من نجد إلى اليمن.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000120', 'person', '10000000-0000-0000-0000-000000000109', 'migrated_to', 'place', '20000000-0000-0000-0000-000000000106', '20000000-0000-0000-0000-000000000103', 'inferred', 'ميمون بن مهند غادر اليمامة إلى الحجاز.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000121', 'person', '10000000-0000-0000-0000-000000000105', 'migrated_to', 'place', '20000000-0000-0000-0000-000000000103', '20000000-0000-0000-0000-000000000101', 'inferred', 'أسد بن راشد انتقل داخل نجد.', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (id) DO NOTHING;

INSERT INTO claim_evidence (claim_id, source_statement_id, source_passage_id, relation, evidence_note_ar, created_by)
VALUES
  ('60000000-0000-0000-0000-000000000101', '50000000-0000-0000-0000-000000000101', '40000000-0000-0000-0000-000000000101', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000102', '50000000-0000-0000-0000-000000000102', '40000000-0000-0000-0000-000000000102', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000103', '50000000-0000-0000-0000-000000000103', '40000000-0000-0000-0000-000000000103', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000104', '50000000-0000-0000-0000-000000000104', '40000000-0000-0000-0000-000000000104', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000105', '50000000-0000-0000-0000-000000000105', '40000000-0000-0000-0000-000000000105', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000106', '50000000-0000-0000-0000-000000000106', '40000000-0000-0000-0000-000000000106', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000107', '50000000-0000-0000-0000-000000000110', '40000000-0000-0000-0000-000000000110', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000108', '50000000-0000-0000-0000-000000000110', '40000000-0000-0000-0000-000000000110', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000109', '50000000-0000-0000-0000-000000000111', '40000000-0000-0000-0000-000000000111', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000110', '50000000-0000-0000-0000-000000000135', '40000000-0000-0000-0000-000000000135', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000111', '50000000-0000-0000-0000-000000000136', '40000000-0000-0000-0000-000000000136', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000112', '50000000-0000-0000-0000-000000000137', '40000000-0000-0000-0000-000000000137', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000113', '50000000-0000-0000-0000-000000000138', '40000000-0000-0000-0000-000000000138', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000114', '50000000-0000-0000-0000-000000000139', '40000000-0000-0000-0000-000000000139', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000115', '50000000-0000-0000-0000-000000000140', '40000000-0000-0000-0000-000000000140', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000116', '50000000-0000-0000-0000-000000000141', '40000000-0000-0000-0000-000000000141', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000117', '50000000-0000-0000-0000-000000000142', '40000000-0000-0000-0000-000000000142', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000118', '50000000-0000-0000-0000-000000000126', '40000000-0000-0000-0000-000000000126', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000119', '50000000-0000-0000-0000-000000000127', '40000000-0000-0000-0000-000000000127', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000120', '50000000-0000-0000-0000-000000000128', '40000000-0000-0000-0000-000000000128', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001'),
  ('60000000-0000-0000-0000-000000000121', '50000000-0000-0000-0000-000000000129', '40000000-0000-0000-0000-000000000129', 'supports', 'العبارة واردة في المصدر.', '00000000-0000-0000-0000-000000000001')
ON CONFLICT DO NOTHING;

INSERT INTO migration_events (id, subject_type, subject_id, from_place_id, to_place_id, time_from, time_to, status, certainty, source_id, notes_ar, created_by)
VALUES
  ('a1000000-0000-0000-0000-000000000101', 'person', '10000000-0000-0000-0000-000000000104', '20000000-0000-0000-0000-000000000105', '20000000-0000-0000-0000-000000000103', '1215-01-01', '1220-12-31', 'documented', 'approximate', '30000000-0000-0000-0000-000000000103', 'انتقال راشد بن هلال إلى اليمامة.', '00000000-0000-0000-0000-000000000001'),
  ('a1000000-0000-0000-0000-000000000102', 'person', '10000000-0000-0000-0000-000000000107', '20000000-0000-0000-0000-000000000101', '20000000-0000-0000-0000-000000000102', '1290-01-01', '1300-12-31', 'platform_inferred', 'uncertain', '30000000-0000-0000-0000-000000000110', 'استقرار ناصر بن صفوان في اليمن.', '00000000-0000-0000-0000-000000000001'),
  ('a1000000-0000-0000-0000-000000000103', 'person', '10000000-0000-0000-0000-000000000109', '20000000-0000-0000-0000-000000000103', '20000000-0000-0000-0000-000000000106', '1350-01-01', '1360-12-31', 'platform_inferred', 'uncertain', '30000000-0000-0000-0000-000000000112', 'خروج ميمون بن مهند إلى الحجاز.', '00000000-0000-0000-0000-000000000001'),
  ('a1000000-0000-0000-0000-000000000104', 'person', '10000000-0000-0000-0000-000000000105', '20000000-0000-0000-0000-000000000101', '20000000-0000-0000-0000-000000000103', '1250-01-01', '1255-12-31', 'platform_inferred', 'uncertain', '30000000-0000-0000-0000-000000000107', 'انتقال أسد بن راشد الداخلي.', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (id) DO NOTHING;

INSERT INTO geographic_associations (id, entity_type, entity_id, place_id, relation_type, time_from, time_to, status, certainty, source_id, created_by)
VALUES
  ('a0000000-0000-0000-0000-000000000101', 'person', '10000000-0000-0000-0000-000000000101', '20000000-0000-0000-0000-000000000101', 'documented_in', '1120-01-01', '1180-12-31', 'documented', 'approximate', '30000000-0000-0000-0000-000000000101', '00000000-0000-0000-0000-000000000001'),
  ('a0000000-0000-0000-0000-000000000102', 'person', '10000000-0000-0000-0000-000000000106', '20000000-0000-0000-0000-000000000102', 'documented_in', '1260-01-01', '1315-12-31', 'documented', 'approximate', '30000000-0000-0000-0000-000000000106', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (id) DO NOTHING;

-- One published tree, so `shortest_relationship_path` and
-- `common_ancestor_path` have an interpretation to walk. It is the الحارثي
-- line and nothing else.
INSERT INTO trees (id, name_ar, description_ar, visibility, owner_id)
VALUES ('b0000000-0000-0000-0000-000000000101', 'شجرة الحارثي', 'تفسير تجريبي لنسب الحارثي.', 'public', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (id) DO NOTHING;

INSERT INTO tree_versions (id, tree_id, version_number, state, publication_note_ar, published_by, published_at)
VALUES ('b1000000-0000-0000-0000-000000000101', 'b0000000-0000-0000-0000-000000000101', 1, 'published', 'النسخة المنشورة لأغراض القياس.', '00000000-0000-0000-0000-000000000001', '2026-03-20T00:00:00Z')
ON CONFLICT (id) DO NOTHING;

INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order)
VALUES
  ('b2000000-0000-0000-0000-000000000101', 'b1000000-0000-0000-0000-000000000101', '10000000-0000-0000-0000-000000000101', 'سالم بن نافع', 1),
  ('b2000000-0000-0000-0000-000000000102', 'b1000000-0000-0000-0000-000000000101', '10000000-0000-0000-0000-000000000102', 'مبارك بن سالم', 2),
  ('b2000000-0000-0000-0000-000000000103', 'b1000000-0000-0000-0000-000000000101', '10000000-0000-0000-0000-000000000103', 'هلال بن مبارك', 3),
  ('b2000000-0000-0000-0000-000000000104', 'b1000000-0000-0000-0000-000000000101', '10000000-0000-0000-0000-000000000104', 'راشد بن هلال', 4),
  ('b2000000-0000-0000-0000-000000000105', 'b1000000-0000-0000-0000-000000000101', '10000000-0000-0000-0000-000000000105', 'أسد بن راشد', 5)
ON CONFLICT (id) DO NOTHING;

INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, source_id, created_by)
VALUES
  ('b3000000-0000-0000-0000-000000000101', 'b1000000-0000-0000-0000-000000000101', 'b2000000-0000-0000-0000-000000000101', 'b2000000-0000-0000-0000-000000000102', 'parent_of', 'interpreted', '30000000-0000-0000-0000-000000000101', '00000000-0000-0000-0000-000000000001'),
  ('b3000000-0000-0000-0000-000000000102', 'b1000000-0000-0000-0000-000000000101', 'b2000000-0000-0000-0000-000000000102', 'b2000000-0000-0000-0000-000000000103', 'parent_of', 'interpreted', '30000000-0000-0000-0000-000000000103', '00000000-0000-0000-0000-000000000001'),
  ('b3000000-0000-0000-0000-000000000103', 'b1000000-0000-0000-0000-000000000101', 'b2000000-0000-0000-0000-000000000103', 'b2000000-0000-0000-0000-000000000104', 'parent_of', 'interpreted', '30000000-0000-0000-0000-000000000105', '00000000-0000-0000-0000-000000000001'),
  ('b3000000-0000-0000-0000-000000000104', 'b1000000-0000-0000-0000-000000000101', 'b2000000-0000-0000-0000-000000000104', 'b2000000-0000-0000-0000-000000000105', 'parent_of', 'interpreted', '30000000-0000-0000-0000-000000000104', '00000000-0000-0000-0000-000000000001')
ON CONFLICT (id) DO NOTHING;

INSERT INTO source_dependencies (source_id, depends_on_source_id, dependency_type, evidence_ar, status)
VALUES
  ('30000000-0000-0000-0000-000000000102', '30000000-0000-0000-0000-000000000101', 'likely_paraphrase', 'تشابه الصياغة يحتاج إلى مراجعة بشرية.', 'needs_review'),
  ('30000000-0000-0000-0000-000000000112', '30000000-0000-0000-0000-000000000102', 'derived_from', 'مختصر الجغرافيا يلخص تاريخ المدن.', 'confirmed')
ON CONFLICT DO NOTHING;
