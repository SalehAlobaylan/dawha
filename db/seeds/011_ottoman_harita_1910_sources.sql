-- Seed 011: Normalized Ottoman archival tribal survey Harita 93677 (c. 1910)
-- and Ibrahim Fasih al-Haydari's Najd toponym witnesses via OpenGulf (NYUAD).
-- Idempotent: safe to re-run.

BEGIN;

INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, citation_ar, location_ar, visibility)
VALUES
  ('b0000000-0000-0000-0000-00000000e096',
   'خريطة وجدول عشائر العراق ونجد العثمانية (خريطة ٩٣٦٧٧ · حوالي ١٩١٠م)',
   'مكتبة جامعة إسطنبول للمصادر النادرة · تفريغ مشروع OpenGulf',
   'archive_record', '1910-01-01',
   'Harita 93677 (Istanbul University Rare Sources Library), digitized in Cole, Barakat, Ammagui & Wrisley, Mapping Tribes: Ottoman Spatial Thinking in Iraq and Arabia, c. 1910 (JOTSA 9.2, 2022; github.com/opengulf/ottoman-map).',
   'إسطنبول / نجد والأحساء والعراق', 'public'),
  ('b0000000-0000-0000-0000-00000000e097',
   'عنوان المجد في بيان أحوال بغداد والبصرة ونجد (قسم نجد)',
   'إبراهيم فصيح الحيدري البغدادي',
   'book', '1869-01-01',
   'إبراهيم فصيح بن صبغة الله الحيدري البغدادي، عنوان المجد في بيان أحوال بغداد والبصرة ونجد (تأليف ١٨٦٩م) · فهرسة مواضع نجد عبر OpenGulf.',
   'بغداد / نجد', 'public')
ON CONFLICT (id) DO UPDATE SET
  title_ar = EXCLUDED.title_ar, author_ar = EXCLUDED.author_ar,
  citation_ar = EXCLUDED.citation_ar, location_ar = EXCLUDED.location_ar;

INSERT INTO atlas_source_items (source_id, item_key)
VALUES
  ('b0000000-0000-0000-0000-00000000e096', 'harita-93677-najd-table'),
  ('b0000000-0000-0000-0000-00000000e097', 'haydari-najd-ch2')
ON CONFLICT (source_id, item_key) DO NOTHING;

INSERT INTO atlas_source_item_revisions
  (item_id, revision_key, locator_ar, external_url, rights_ar, credit_ar, period_ar, depicted_from_year, depicted_to_year)
SELECT i.id, 'initial-citation', v.locator, v.url, v.rights, v.credit, v.period, v.yfrom, v.yto
FROM (VALUES
  ('b0000000-0000-0000-0000-00000000e096', 'harita-93677-najd-table',
   'خريطة ٩٣٦٧٧ والجدول المرافق · قسم نجد والأحساء وقطر والكويت',
   'https://github.com/opengulf/ottoman-map',
   'بيانات بحثية مفتوحة التوثيق · مرجع أكاديمي (JOTSA 2022)',
   'جامعة إسطنبول · مشروع OpenGulf (جامعة نيويورك أبوظبي)',
   'حوالي ١٩١٠م (مسح عثماني)', 1910, 1910),
  ('b0000000-0000-0000-0000-00000000e097', 'haydari-najd-ch2',
   'الفصل الثاني من قسم نجد · فهرس المواضع والأقاليم',
   'https://github.com/opengulf/ottoman-map/blob/main/HaydariNajd/HaydariNajd.csv',
   'نص تاريخي من القرن التاسع عشر · إحالة بحثية',
   'إبراهيم فصيح الحيدري (١٨٦٩م) · فهرسة OpenGulf',
   'القرن التاسع عشر (١٨٦٩م)', 1869, 1869)
) AS v(source_id, item_key, locator, url, rights, credit, period, yfrom, yto)
JOIN atlas_source_items i ON i.source_id = v.source_id::uuid AND i.item_key = v.item_key
ON CONFLICT (item_id, revision_key) DO NOTHING;

INSERT INTO atlas_source_occurrences
  (item_revision_id, occurrence_key, original_text, display_ar, language, reading_method, note_ar, tribe_id, published)
SELECT r.id, v.occ_key, v.orig, v.disp, 'ar', 'original', v.note, t.id, true
FROM (VALUES
  ('najd-264', 'بني ثقيف', 'ثقيف', 'الموضع/التبعية في الجدول العثماني: بو دخي', NULL::uuid),
  ('najd-263', 'البقارا', 'البقارة (الحجاز)', 'الموضع/التبعية في الجدول العثماني: حجاز ولايتنه راجعدر', NULL::uuid),
  ('najd-262', 'بني زيد', 'بنو زيد', 'الموضع/التبعية في الجدول العثماني: مكه مكرمه', NULL::uuid),
  ('najd-261', 'بني عون', 'بنو عون', 'الموضع/التبعية في الجدول العثماني: مكه مكرمه', NULL::uuid),
  ('najd-260', 'حرب', 'حرب', 'تقدير السكان: 80,000 · الخيام: 10,000 · الموضع/التبعية في الجدول العثماني: ابن رشيده تابعدر', 'b0000000-0000-0000-0000-c00000000001'::uuid),
  ('najd-259', 'عتيبة', 'عتيبة', 'تقدير السكان: 100,000 · الخيام: 15,000 · الموضع/التبعية في الجدول العثماني: حجاز ولايتنه راجعدر', 'b0000000-0000-0000-0000-c00000000005'::uuid),
  ('najd-256', 'ال سعود', 'آل سعود', 'تقدير السكان: 8,000 · الخيام: 800 · الخيالة: 1,000 · الموضع/التبعية في الجدول العثماني: عارض', NULL::uuid),
  ('najd-246', 'بني عطية', 'بنو عطية', 'الفرق: 6 · الموضع/التبعية في الجدول العثماني: بو دخي', NULL::uuid),
  ('najd-244', 'الجوف', 'الجوف', 'الموضع/التبعية في الجدول العثماني: بو دخي', NULL::uuid),
  ('najd-242', 'شمر', 'شمّر', 'تقدير السكان: 67,000 · الخيام: 8,000 · الخيالة: 3,000 · الموضع/التبعية في الجدول العثماني: نجد', 'b0000000-0000-0000-0000-c00000000002'::uuid),
  ('najd-255', 'سبيع', 'سبيع', 'تقدير السكان: 11,700 · الخيام: 1,000 · الخيالة: 800 · الموضع/التبعية في الجدول العثماني: عارض', NULL::uuid),
  ('najd-241', 'العجمان', 'العجمان', 'تقدير السكان: 17,050 · الخيام: 3,410 · الخيالة: 1,876 · الفرق: 15 · الموضع/التبعية في الجدول العثماني: احسا', NULL::uuid),
  ('najd-265', 'دهنة الحجانية', 'الدهناء (بادية نجد)', 'الموضع/التبعية في الجدول العثماني: نجد', NULL::uuid),
  ('najd-267', 'بني مطير', 'مطير', 'الموضع/التبعية في الجدول العثماني: نجد', 'b0000000-0000-0000-0000-c00000000006'::uuid),
  ('najd-237', 'زعب رشايدة', 'زعب والرشايدة', 'تقدير السكان: 390 · الخيام: 35 · الخيالة: 15 · الفرق: 6 · الموضع/التبعية في الجدول العثماني: قطيف', NULL::uuid),
  ('najd-236', 'بني خالد', 'بنو خالد', 'تقدير السكان: 6,575 · الخيام: 1,315 · الخيالة: 670 · الفرق: 4 · الموضع/التبعية في الجدول العثماني: قطيف', 'b0000000-0000-0000-0000-c00000000007'::uuid),
  ('najd-234', 'بني فهد', 'بنو فهد', 'تقدير السكان: 200 · الخيام: 50 · الخيالة: 10 · الفرق: 1 · الموضع/التبعية في الجدول العثماني: احسا', NULL::uuid),
  ('najd-247', 'بني مرة', 'آل مرة', 'تقدير السكان: 14,440 · الخيام: 2,900 · الخيالة: 1,880 · الفرق: 6 · الموضع/التبعية في الجدول العثماني: احسا', NULL::uuid),
  ('najd-251', 'مريحات', 'المريحات', 'تقدير السكان: 830 · الخيام: 120 · الخيالة: 7 · الفرق: 1 · الموضع/التبعية في الجدول العثماني: قطر', NULL::uuid),
  ('najd-250', 'الكعبان', 'الكعبان', 'تقدير السكان: 820 · الخيام: 115 · الخيالة: 5 · الفرق: 1 · الموضع/التبعية في الجدول العثماني: قطر', NULL::uuid),
  ('najd-239', 'عريبدار', 'عربدار (بادية الكويت)', 'تقدير السكان: 6,000 · الخيام: 600 · الخيالة: 160 · الفرق: 1 · الموضع/التبعية في الجدول العثماني: كويت', NULL::uuid),
  ('najd-238', 'الطواحة', 'الطواحة (بادية الكويت)', 'تقدير السكان: 6,000 · الخيام: 600 · الخيالة: 150 · الفرق: 1 · الموضع/التبعية في الجدول العثماني: كويت', NULL::uuid),
  ('najd-243', 'عنزة', 'عنزة', 'الموضع/التبعية في الجدول العثماني: شام ولايتنه راجعه', 'b0000000-0000-0000-0000-c00000000003'::uuid),
  ('najd-245', 'شرارات', 'الشرارات', 'الموضع/التبعية في الجدول العثماني: بو دخي', NULL::uuid),
  ('najd-257', 'بني قحطان', 'قحطان', 'تقدير السكان: 46,500 · الخيام: 6,000 · الموضع/التبعية في الجدول العثماني: مكه مكرمه', 'b0000000-0000-0000-0000-c0000000000a'::uuid),
  ('najd-253', 'الكيسة', 'القبيسات / الكيسة', 'تقدير السكان: 1,000 · الخيام: 130 · الخيالة: 15 · الفرق: 9 · الموضع/التبعية في الجدول العثماني: قطر', NULL::uuid),
  ('najd-252', 'نعيم', 'النعيم', 'تقدير السكان: 1,550 · الخيام: 175 · الخيالة: 50 · الفرق: 1 · الموضع/التبعية في الجدول العثماني: قطر', NULL::uuid),
  ('najd-248', 'بني هاجر', 'بنو هاجر', 'تقدير السكان: 4,500 · الخيام: 500 · الخيالة: 160 · الفرق: 2 · الموضع/التبعية في الجدول العثماني: قطر', NULL::uuid),
  ('najd-249', 'مناصير', 'المناصير', 'تقدير السكان: 1,280 · الخيام: 225 · الخيالة: 30 · الفرق: 1 · الموضع/التبعية في الجدول العثماني: قطر', NULL::uuid),
  ('najd-258', 'دواسر', 'الدواسر', 'تقدير السكان: 18,500 · الخيام: 2,000 · الخيالة: 300 · الموضع/التبعية في الجدول العثماني: عارض', NULL::uuid),
  ('najd-235', 'عتبان', 'عتبان (القطيف)', 'تقدير السكان: 600 · الخيالة: 30 · الفرق: 1 · الموضع/التبعية في الجدول العثماني: قطيف', NULL::uuid),
  ('najd-240', 'العوازم', 'العوازم', 'تقدير السكان: 9,240 · الخيام: 1,850 · الخيالة: 90 · الفرق: 1 · الموضع/التبعية في الجدول العثماني: احسا', NULL::uuid)
) AS v(occ_key, orig, disp, note, tribe_id)
JOIN atlas_source_items i ON i.source_id = 'b0000000-0000-0000-0000-00000000e096' AND i.item_key = 'harita-93677-najd-table'
JOIN atlas_source_item_revisions r ON r.item_id = i.id AND r.revision_key = 'initial-citation'
LEFT JOIN tribes t ON t.id = v.tribe_id AND t.visibility = 'public'
ON CONFLICT (item_revision_id, occurrence_key) DO UPDATE SET
  display_ar = EXCLUDED.display_ar, note_ar = EXCLUDED.note_ar, tribe_id = COALESCE(atlas_source_occurrences.tribe_id, EXCLUDED.tribe_id);

-- Key Al-Haydari Najd region & settlement witnesses (1869)
INSERT INTO atlas_source_occurrences
  (item_revision_id, occurrence_key, original_text, display_ar, language, reading_method, note_ar, published)
SELECT r.id, v.occ_key, v.orig, v.disp, 'ar', 'original', v.note, true
FROM (VALUES
  ('najd', 'نجد', 'نجد', 'الإقليم الرئيس في فصل الحيدري (١٨٦٩م).'),
  ('jabal-shammar', 'جبل شمر', 'جبل شمّر', 'يورد الحيدري حائل وجبة وبقعاء وسميراء وموقق ضمن جبل شمر.'),
  ('qassim', 'القصيم', 'القصيم', 'يذكر الحيدري عنيزة وبريدة والأسياح وعين ابن فهيد وقصيباء.'),
  ('hail', 'حائل', 'حائل', 'حاضرة جبل شمر في وصف الحيدري (١٨٦٩م).'),
  ('riyadh', 'الرياض', 'الرياض', 'من حواضر العارض في نجد.'),
  ('unaizah', 'عنيزة', 'عنيزة', 'من حواضر القصيم في وصف الحيدري.'),
  ('buraidah', 'بريدة', 'بريدة', 'من حواضر القصيم في وصف الحيدري.'),
  ('sudayr', 'سدير', 'سدير', 'من أقاليم نجد المعدودة عند الحيدري.'),
  ('arid', 'العارض', 'العارض', 'من أقاليم نجد الرئيسة عند الحيدري.'),
  ('kharj', 'الخرج', 'الخرج', 'من أقاليم نجد الجنوبية عند الحيدري.'),
  ('aflaj', 'الأفلاج', 'الأفلاج', 'من أقاليم نجد الجنوبية عند الحيدري.')
) AS v(occ_key, orig, disp, note)
JOIN atlas_source_items i ON i.source_id = 'b0000000-0000-0000-0000-00000000e097' AND i.item_key = 'haydari-najd-ch2'
JOIN atlas_source_item_revisions r ON r.item_id = i.id AND r.revision_key = 'initial-citation'
ON CONFLICT (item_revision_id, occurrence_key) DO NOTHING;

COMMIT;
