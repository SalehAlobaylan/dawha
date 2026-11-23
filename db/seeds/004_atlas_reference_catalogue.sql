BEGIN;

INSERT INTO sources (id, title_ar, author_ar, source_type, visibility)
VALUES
    ('b0000000-0000-0000-0000-00000000e083', 'جغرافية المملكة العربية السعودية', 'بسمة الرحيلي', 'other', 'public'),
    ('b0000000-0000-0000-0000-00000000e084', 'القول المكتوب في تاريخ الجنوب', 'محمد بن أحمد بن غيثان', 'book', 'public'),
    ('b0000000-0000-0000-0000-00000000e085', 'خريطة بني سامة في العصور الإسلامية الأولى', NULL, 'article', 'public'),
    ('b0000000-0000-0000-0000-00000000e086', 'قصاصات خريطة منشورة عبر الأرشيف العثماني', NULL, 'website', 'public'),
    ('b0000000-0000-0000-0000-00000000e087', 'جغرافية عسير وقبائلها · في الصورة', 'تركي بن طلال', 'other', 'public'),
    ('b0000000-0000-0000-0000-00000000e088', 'رسم تقريبي لديرة آل مرة', 'هارولد ديكسون', 'archive_record', 'public')
ON CONFLICT (id) DO NOTHING;

-- The two social crops have a shared source. Their exact archival origin
-- remains unresolved, and neither is a separate witness.
WITH entries(source_id, item_key, locator, url, rights, credit, period, year_from, year_to) AS (VALUES
    ('b0000000-0000-0000-0000-00000000e083', 'saudi-p35', 'ص ٣٥',
     'https://drive.uqu.edu.sa/_/gmqurbeh/files/Gegr%20Of%20KSA%20Intisab.pdf#page=35',
     'رابط إلى الصفحة · مصدر رسم الشكل وتاريخ المسح غير محددين', 'جامعة أم القرى', '', NULL, NULL),
    ('b0000000-0000-0000-0000-00000000e084', 'asir-p502', 'ج ٢٠ · ص ٥٠٢ · صفحة الملف ٨',
     'https://prof-ghithan.com/wp-content/uploads/2022/03/312.pdf#page=8',
     'رابط إلى صفحة المؤلف', 'محمد بن أحمد بن غيثان', 'قراءة تاريخية لنص الهمداني', NULL, NULL),
    ('b0000000-0000-0000-0000-00000000e085', 'oman-map-and-key', 'ملحق الخرائط · ص ٦٠٩ · صفحة الملف ٣٢',
     'https://hamadaljasser.com/wp-content/uploads/2024/01/ربيع-الآخر-جماى-الآخرة-1445هـ.pdf#page=32',
     'رابط إلى صفحة المجلة', 'مجلة العرب', 'العصور الإسلامية الأولى بحسب عنوان الشكل', NULL, NULL),
    ('b0000000-0000-0000-0000-00000000e086', 'tabuk-crop', 'قصاصة تبوك · أول صورة في المنشور',
     'https://x.com/Ottoman2Archive/status/1923658725685973069/photo/1',
     'التاريخ والمرجع الأرشيفي للأصل غير محققين · رابط إلى المنشور', 'حساب Ottoman2Archive · ١٧ مايو ٢٠٢٥', '', NULL, NULL),
    ('b0000000-0000-0000-0000-00000000e086', 'medina-crop', 'قصاصة المدينة · ثاني صورة في المنشور',
     'https://x.com/Ottoman2Archive/status/1923658725685973069/photo/2',
     'التاريخ والمرجع الأرشيفي للأصل غير محققين · رابط إلى المنشور', 'حساب Ottoman2Archive · ١٧ مايو ٢٠٢٥', '', NULL, NULL),
    ('b0000000-0000-0000-0000-00000000e087', 'asir-33m10', 'برنامج في الصورة · ٣٣:١٠',
     'https://www.youtube.com/watch?v=rT1EUgxsf-Y&t=1990s',
     'رابط إلى البث', 'روتانا خليجية', 'ديسمبر ٢٠٢١ (تاريخ البث)', NULL, NULL),
    ('b0000000-0000-0000-0000-00000000e088', 'murra-195b', 'IOR/R/15/1/605 · f. 195B',
     'https://www.qdl.qa/en/archive/81055/vdc_100048679713.0x000002',
     'يسرد الفهرس الملك العام ورخصة الحكومة المفتوحة · رابط إلى السجل', 'المكتبة البريطانية عبر مكتبة قطر الرقمية',
     '١٩٣٥ (تاريخ الرسم)', 1935, 1935)
), added AS (
    INSERT INTO atlas_source_items (source_id, item_key)
    SELECT source_id::uuid, item_key FROM entries
    ON CONFLICT (source_id, item_key) DO NOTHING
    RETURNING id, source_id, item_key
), all_items AS (
    SELECT a.id, a.source_id, a.item_key FROM added a
    UNION ALL
    SELECT i.id, i.source_id, i.item_key FROM atlas_source_items i
    JOIN entries e ON i.source_id = e.source_id::uuid AND i.item_key = e.item_key
)
INSERT INTO atlas_source_item_revisions
    (item_id, revision_key, locator_ar, external_url, rights_ar, credit_ar, period_ar, depicted_from_year, depicted_to_year)
SELECT i.id, 'initial-citation', e.locator, e.url, e.rights, e.credit, e.period, e.year_from, e.year_to
FROM entries e JOIN all_items i ON i.source_id = e.source_id::uuid AND i.item_key = e.item_key
ON CONFLICT (item_id, revision_key) DO NOTHING;

INSERT INTO atlas_source_occurrences
    (item_revision_id, occurrence_key, original_text, display_ar, language, reading_method, note_ar, published)
SELECT r.id, v.occurrence_key, v.original, v.display, v.language, v.method, v.note, true
FROM (VALUES
    ('saudi-p35', 'shammar', 'شمر', 'شمر', 'ar', 'original', ''),
    ('saudi-p35', 'mutayr', 'مطير', 'مطير', 'ar', 'original', ''),
    ('saudi-p35', 'harb', 'حرب', 'حرب', 'ar', 'original', ''),
    ('saudi-p35', 'utayba', 'عتيبه', 'عتيبة', 'ar', 'original', ''),
    ('asir-p502', 'sarat-anz', 'سراة عنز بن وائل', 'سراة عنز بن وائل', 'ar', 'original', 'يشرح المؤلف اعتماده على الهمداني في النص أعلى الرسم.'),
    ('oman-map-and-key', 'bani-sama', 'بني سامة', 'بني سامة', 'ar', 'original', 'خريطة وبيان مرقّم للقبائل في ملحق الدراسة.'),
    ('tabuk-crop', 'bani-atiya', 'بني عطية', 'بني عطية', 'ar', 'original', 'يظهر الاسم في قصاصة تبوك.'),
    ('medina-crop', 'harb', 'عشائر حرب', 'حرب', 'ar', 'original', 'يظهر الاسم قرب المدينة وينبع.'),
    ('asir-33m10', 'asir-map', 'عسير', 'عسير', 'ar', 'catalogue', 'يبدأ شرح خريطة مستويات عسير عند هذا الموضع.'),
    ('murra-195b', 'murra', 'Murra', 'آل مرة', 'en', 'catalogue', 'يصف الفهرس ديرة تقريبية. المسار الأحمر مسار الرحّالة برترام توماس.')
) AS v(item_key, occurrence_key, original, display, language, method, note)
JOIN atlas_source_items i ON i.item_key = v.item_key AND i.source_id BETWEEN
    'b0000000-0000-0000-0000-00000000e083' AND 'b0000000-0000-0000-0000-00000000e088'
JOIN atlas_source_item_revisions r ON r.item_id = i.id AND r.revision_key = 'initial-citation'
ON CONFLICT (item_revision_id, occurrence_key) DO NOTHING;

COMMIT;
