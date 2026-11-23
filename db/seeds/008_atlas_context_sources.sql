BEGIN;

INSERT INTO sources (id, title_ar, author_ar, source_type, publication_date_from, visibility)
VALUES
    ('b0000000-0000-0000-0000-00000000e091', 'ملوك العرب', 'أمين الريحاني', 'book', NULL, 'public'),
    ('b0000000-0000-0000-0000-00000000e092', 'خرائط شمال الجزيرة العربية، مجموعة معهد عمّان', 'ألويس موزيل', 'archive_record', '1926-01-01', 'public'),
    ('b0000000-0000-0000-0000-00000000e093', 'شمال الجزيرة العربية، مجموعة عيران لاور', 'ألويس موزيل', 'archive_record', NULL, 'public'),
    ('b0000000-0000-0000-0000-00000000e094', 'المنطقة الشرقية · مجلة العرب', 'حمد الجاسر', 'article', NULL, 'public'),
    ('b0000000-0000-0000-0000-00000000e095', 'خريطة الجزيرة العربية نحو ٦٠٠م · الرسم الإنجليزي', NULL, 'other', NULL, 'public')
ON CONFLICT (id) DO NOTHING;

INSERT INTO atlas_source_items (source_id,item_key)
VALUES ('b0000000-0000-0000-0000-00000000e091','muluk-map-p20'),
       ('b0000000-0000-0000-0000-00000000e092','musil-amman-catalogue'),
       ('b0000000-0000-0000-0000-00000000e093','musil-laor-catalogue')
ON CONFLICT (source_id,item_key) DO NOTHING;

INSERT INTO atlas_source_item_revisions (item_id,revision_key,locator_ar,external_url,rights_ar,credit_ar,period_ar)
SELECT i.id,'initial-citation',v.locator,v.url,v.rights,v.credit,v.period
FROM (VALUES
  ('b0000000-0000-0000-0000-00000000e091','muluk-map-p20','المقدمة · ص ٢٠، الخريطة والنص',
    'https://downloads.hindawi.org/books/83158373.pdf#page=20',
    'رابط إلى صفحة النسخة الرقمية','مؤسسة هنداوي','رحلات أوائل القرن العشرين'),
  ('b0000000-0000-0000-0000-00000000e092','musil-amman-catalogue','AI-RBC-915.53-MUS · أربع خرائط مطوية',
    'https://digitalrepository.cbrl.ac.uk/musil-northern-arabia-1926-american-geographic-society-maps',
    'CC BY-NC-SA 4.0 بحسب فهرس معهد عمّان','Council for British Research in the Levant · AI-RBC-915.53-MUS','١٩٢٦ (تاريخ إعداد الخرائط)'),
  ('b0000000-0000-0000-0000-00000000e093','musil-laor-catalogue','NNL_ALEPH990027073430205171',
    'https://www.nli.org.il/en/maps/NNL_ALEPH990027073430205171/NLI',
    'ملك عام بحسب سجل المجموعة · رابط إلى الفهرس','مجموعة خرائط عيران لاور','')
) AS v(source_id,item_key,locator,url,rights,credit,period)
JOIN atlas_source_items i ON i.source_id=v.source_id::uuid AND i.item_key=v.item_key
ON CONFLICT (item_id,revision_key) DO NOTHING;

INSERT INTO atlas_source_occurrences
    (item_revision_id,occurrence_key,original_text,display_ar,language,reading_method,note_ar,source_selector,published)
SELECT r.id, v.key, v.original, v.display, 'ar', 'original', v.note, '{"pdfPage":20}'::jsonb, true
FROM (VALUES
  ('oman','عمان','عُمان','يظهر الاسم في فقرة الرحلة المصاحبة للخريطة.'),
  ('qatar','قطر','قطر','يظهر الاسم في فقرة الرحلة المصاحبة للخريطة.'),
  ('muscat','مسقط','مسقط','يذكر المؤلف مسقط في وصف الرحلة المصاحب للخريطة.')
) AS v(key,original,display,note)
JOIN atlas_source_items i ON i.source_id='b0000000-0000-0000-0000-00000000e091' AND i.item_key='muluk-map-p20'
JOIN atlas_source_item_revisions r ON r.item_id=i.id AND r.revision_key='initial-citation'
ON CONFLICT (item_revision_id,occurrence_key) DO NOTHING;

INSERT INTO source_dependencies (source_id,depends_on_source_id,dependency_type,evidence_ar,status)
VALUES
  ('b0000000-0000-0000-0000-00000000e080','b0000000-0000-0000-0000-00000000e094','cites',
   'هامش ٩ أسفل شكل ٦ في ص ١٠٨ يحيل إلى حمد الجاسر، المنطقة الشرقية، مجلة العرب، ص ٧٣.','confirmed'),
  ('b0000000-0000-0000-0000-00000000e089','b0000000-0000-0000-0000-00000000e095','derived_from',
   'توضح صفحة ملف ويكيميديا كومنز أن هذه النسخة تعريب للرسم الإنجليزي.','confirmed'),
  ('b0000000-0000-0000-0000-00000000e092','b0000000-0000-0000-0000-00000000e093','shared_origin',
   'تتصل السجلّات بخرائط موزيل لشمال الجزيرة. مطابقة الأوراق والطبعة بانتظار المراجعة.','needs_review')
ON CONFLICT (source_id,depends_on_source_id,dependency_type) DO NOTHING;

COMMIT;
