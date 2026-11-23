BEGIN;

INSERT INTO source_passages (source_id, sequence_number, page_number, locator_ar, text_ar, normalized_text_ar)
VALUES
    ('b0000000-0000-0000-0000-00000000e082', 192, 192, 'قبيلة مطير · ص ١٩٢',
     'علوه وبريه', atlas_occurrence_search_key_v1('علوه وبريه')),
    ('b0000000-0000-0000-0000-00000000e082', 193, 193, 'قبيلة مطير · ص ١٩٣',
     'وتتألف بريه من الأفخاذ والعشائر الآتية', atlas_occurrence_search_key_v1('وتتألف بريه من الأفخاذ والعشائر الآتية'))
ON CONFLICT (source_id, sequence_number) DO NOTHING;

UPDATE atlas_source_occurrences o
SET source_passage_id = p.id
FROM atlas_source_item_revisions r
JOIN atlas_source_items i ON i.id = r.item_id
JOIN source_passages p ON p.source_id = i.source_id AND i.item_key = concat('page-', p.page_number)
WHERE o.item_revision_id = r.id
  AND r.revision_key = 'initial-citation'
  AND i.source_id = 'b0000000-0000-0000-0000-00000000e082'
  AND o.occurrence_key = 'barriyah'
  AND o.source_passage_id IS NULL;

COMMIT;
