BEGIN;

INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, visibility)
VALUES
    ('b0000000-0000-0000-0000-00000000e080', 'الأوضاع الاقتصادية والاجتماعية في إقليم الأحساء', 'حمد محمد القحطاني', 'research_paper', '2010-01-01', 'public'),
    ('b0000000-0000-0000-0000-00000000e081', 'رسم تخطيطي للمناطق القبلية على حدود الكويت وساحل الأحساء', 'هارولد ديكسون', 'archive_record', NULL, 'public'),
    ('b0000000-0000-0000-0000-00000000e082', 'قلب جزيرة العرب', 'فؤاد حمزة', 'book', '1933-01-01', 'public')
ON CONFLICT (id) DO NOTHING;

INSERT INTO atlas_source_items (source_id, item_key)
VALUES
    ('b0000000-0000-0000-0000-00000000e080', 'figure-6'),
    ('b0000000-0000-0000-0000-00000000e081', 'folio-228r'),
    ('b0000000-0000-0000-0000-00000000e082', 'page-192'),
    ('b0000000-0000-0000-0000-00000000e082', 'page-193')
ON CONFLICT (source_id, item_key) DO NOTHING;

INSERT INTO atlas_source_item_revisions
    (item_id, revision_key, locator_ar, external_url, rights_ar, credit_ar, period_ar, depicted_from_year, depicted_to_year)
SELECT i.id, 'initial-citation', v.locator, v.url, v.rights, v.credit, v.period, v.from_year, v.to_year
FROM (VALUES
    ('b0000000-0000-0000-0000-00000000e080', 'figure-6',
     'الشكل ٦ · ص ١٠٨ (الصفحة المطبوعة وصفحة الملف)',
     'https://www.alhasa-turath.org/wp-content/uploads/2020/10/الأوضاع-الاقتصادية-والاجتماعية-في-إقليم-الأحساء-١٢٨٨ـ١٣٣١-هـ-١٨٧١ـ١٩١٣-م.pdf#page=108',
     'حقوق النشر محفوظة · رابط إلى النسخة المتاحة', 'مركز دراسات الخليج والجزيرة العربية، جامعة الكويت',
     '١٨٧١–١٩١٣ (فترة الدراسة)', 1871, 1913),
    ('b0000000-0000-0000-0000-00000000e081', 'folio-228r',
     'IOR/R/15/1/586 · 228r', 'https://www.qdl.qa/en/archive/81055/vdc_100023488765.0x000045',
     'ملك عام بحسب فهرس مكتبة قطر الرقمية', 'المكتبة البريطانية عبر مكتبة قطر الرقمية',
     '٣ أغسطس ١٩٢٩ (تاريخ الرسم)', 1929, 1929),
    ('b0000000-0000-0000-0000-00000000e082', 'page-192',
     'ص ١٩٢ · صورة ٢١٨', 'https://sites.dlib.nyu.edu/viewer/books/aub_aco001825/218?lang=ar',
     'رابط إلى النسخة المرقمنة', 'الجامعة الأمريكية في بيروت · المجموعات العربية على الإنترنت', '', NULL, NULL),
    ('b0000000-0000-0000-0000-00000000e082', 'page-193',
     'ص ١٩٣ · صورة ٢١٩', 'https://sites.dlib.nyu.edu/viewer/books/aub_aco001825/219?lang=ar',
     'رابط إلى النسخة المرقمنة', 'الجامعة الأمريكية في بيروت · المجموعات العربية على الإنترنت', '', NULL, NULL)
) AS v(source_id, item_key, locator, url, rights, credit, period, from_year, to_year)
JOIN atlas_source_items i ON i.source_id = v.source_id::uuid AND i.item_key = v.item_key
ON CONFLICT (item_id, revision_key) DO NOTHING;

INSERT INTO atlas_source_occurrences
    (item_revision_id, occurrence_key, original_text, display_ar, language, reading_method, note_ar, tribe_id, published)
SELECT r.id, v.occurrence_key, v.original, v.display, v.language, v.method, v.note, t.id, true
FROM (VALUES
    ('figure-6', 'mutayr', 'مطير', 'مطير', 'ar', 'original', '', 'b0000000-0000-0000-0000-c00000000006'),
    ('figure-6', 'awazim', 'العوازم', 'العوازم', 'ar', 'original', '', NULL),
    ('figure-6', 'ajman', 'العجمان', 'العجمان', 'ar', 'original', '', NULL),
    ('figure-6', 'hajar', 'بنو هاجر', 'بنو هاجر', 'ar', 'original', '', NULL),
    ('figure-6', 'khalid', 'بنو خالد', 'بنو خالد', 'ar', 'original', '', 'b0000000-0000-0000-0000-c00000000007'),
    ('figure-6', 'murra', 'آل مرة', 'آل مرة', 'ar', 'original', '', NULL),
    ('folio-228r', 'mutayr', 'Mutair', 'مطير', 'en', 'catalogue', 'الاسم كما يفهرسه سجل مكتبة قطر الرقمية.', 'b0000000-0000-0000-0000-c00000000006'),
    ('folio-228r', 'awazim', 'Awazim', 'العوازم', 'en', 'catalogue', 'الاسم كما يفهرسه سجل مكتبة قطر الرقمية.', NULL),
    ('folio-228r', 'ajman', 'Ajman', 'العجمان', 'en', 'catalogue', 'الاسم كما يفهرسه سجل مكتبة قطر الرقمية.', NULL),
    ('page-192', 'barriyah', 'بريه', 'برية', 'ar', 'original', 'يذكر فؤاد حمزة الاسم في مدخل قبيلة مطير.', NULL),
    ('page-193', 'barriyah', 'بريه', 'برية', 'ar', 'original', 'يفصّل المؤلف الأفخاذ والعشائر تحت هذا الاسم.', NULL)
) AS v(item_key, occurrence_key, original, display, language, method, note, tribe_id)
JOIN atlas_source_items i ON i.item_key = v.item_key
    AND i.source_id IN ('b0000000-0000-0000-0000-00000000e080','b0000000-0000-0000-0000-00000000e081','b0000000-0000-0000-0000-00000000e082')
JOIN atlas_source_item_revisions r ON r.item_id = i.id AND r.revision_key = 'initial-citation'
LEFT JOIN tribes t ON t.id = v.tribe_id::uuid AND t.visibility = 'public'
ON CONFLICT (item_revision_id, occurrence_key) DO NOTHING;

COMMIT;
