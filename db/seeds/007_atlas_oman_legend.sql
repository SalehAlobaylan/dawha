BEGIN;

-- Each original is the reading of one printed key row on PDF page 32.
-- A single map symbol can refer to more than one group.
INSERT INTO atlas_source_occurrences
    (item_revision_id, occurrence_key, original_text, display_ar, language, reading_method, source_selector, published)
SELECT r.id, 'legend-' || row.number, row.original, row.display, 'ar', 'original',
       jsonb_build_object(
          'figure', jsonb_build_object('pdfPage', 32, 'printedPage', '٦٠٩', 'symbol', row.number),
          'legend', jsonb_build_object('pdfPage', 32, 'printedPage', '٦٠٩', 'row', row.number)
       ), true
FROM (VALUES
    (1, 'بنو سامة بن لؤي وبنو الحارث', 'بني سامة بن لؤي وبنو الحارث'),
    (2, 'أزد عمان', 'أزد عمان'),
    (3, 'قبائل الحدان', 'قبائل الحدان'),
    (4, 'عبدالقيس', 'عبد القيس'),
    (5, 'بنو ريام', 'بنو ريام'),
    (6, 'بنو الحارث', 'بنو الحارث'),
    (7, 'قبائل المهرة', 'قبائل المهرة'),
    (8, 'بنو الجلندى', 'بنو الجلندى')
) AS row(number, original, display)
JOIN atlas_source_items i ON i.source_id = 'b0000000-0000-0000-0000-00000000e085' AND i.item_key = 'oman-map-and-key'
JOIN atlas_source_item_revisions r ON r.item_id = i.id AND r.revision_key = 'initial-citation'
ON CONFLICT (item_revision_id, occurrence_key) DO NOTHING;

COMMIT;
