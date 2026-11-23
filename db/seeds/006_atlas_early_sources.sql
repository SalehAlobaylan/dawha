BEGIN;

INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, citation_ar, visibility)
VALUES
  ('b0000000-0000-0000-0000-00000000e089', 'مواطن القبائل في الجزيرة العربية نحو سنة ٦٠٠م', 'ميشيل بكّني ومساهمو ويكيميديا كومنز',
   'other', '2020-06-15', 'تعريب خريطة Map of Arabia 600 AD · نسخة ١٥ أكتوبر ٢٠٢٣ · CC BY-SA 4.0', 'public'),
  ('b0000000-0000-0000-0000-00000000e090', 'البدو · ملحق خرائط انتشار القبائل (المجلد الخامس)', 'ماكس فون أوبنهايم وآخرون',
   'book', NULL, 'إعداد الخرائط ماجد شبر · دار الوراق · إحالة فهرسية، النسخة المصورة قيد البحث', 'public')
ON CONFLICT (id) DO NOTHING;

INSERT INTO atlas_source_items (source_id, item_key)
VALUES ('b0000000-0000-0000-0000-00000000e089', 'arabia-600-svg'),
       ('b0000000-0000-0000-0000-00000000e090', 'bedouins-v5-catalogue')
ON CONFLICT (source_id, item_key) DO NOTHING;

INSERT INTO atlas_source_item_revisions
    (item_id, revision_key, locator_ar, external_url, rights_ar, credit_ar, period_ar, depicted_from_year, depicted_to_year, scan_sha256)
SELECT i.id, v.revision, v.locator, v.url, v.rights, v.credit, v.period, v.from_year, v.to_year, v.digest
FROM (VALUES
    ('b0000000-0000-0000-0000-00000000e089', 'arabia-600-svg', 'commons-2023-10-15', 'الرسم العربي · نسخة ١٥ أكتوبر ٢٠٢٣',
     'https://commons.wikimedia.org/w/index.php?title=File:Map_of_Arabia_600_AD-ar.svg&oldid=1248516008',
     'CC BY-SA 4.0', 'ميشيل بكّني ومساهمو ويكيميديا كومنز', 'نحو ٦٠٠م', 600, 600,
     'b2e39abb3d74e666597a2dfce76e5753685ff0182bc9ca49fd3a245d1b595373'),
    ('b0000000-0000-0000-0000-00000000e090', 'bedouins-v5-catalogue', 'catalogue-only', 'المجلد الخامس · ملحق الخرائط',
     'https://www.drabusulaimanlibrary.com/bib/3001',
     'الترخيص والنسخة المصورة قيد التحقق', 'مكتبة الدكتور عبد الوهاب أبو سليمان', '', NULL, NULL, NULL)
) AS v(source_id, item_key, revision, locator, url, rights, credit, period, from_year, to_year, digest)
JOIN atlas_source_items i ON i.source_id = v.source_id::uuid AND i.item_key = v.item_key
ON CONFLICT (item_id, revision_key) DO NOTHING;

INSERT INTO atlas_source_occurrences
    (item_revision_id, occurrence_key, original_text, display_ar, language, reading_method, note_ar, published)
SELECT r.id, 'abd-al-qays', 'عبد القيس', 'عبد القيس', 'ar', 'original',
       'يظهر الاسم في الجهة الغربية من الخليج على الرسم. يُقرأ في مساحة المصدر بسبب خطأ معايرته الجغرافية.', true
FROM atlas_source_item_revisions r JOIN atlas_source_items i ON i.id = r.item_id
WHERE i.source_id = 'b0000000-0000-0000-0000-00000000e089' AND r.revision_key = 'commons-2023-10-15'
ON CONFLICT (item_revision_id, occurrence_key) DO NOTHING;

COMMIT;
