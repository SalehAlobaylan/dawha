-- Retire stale first-decision tokens on legacy items already reopened by 0058.
-- Only the operational queue changes; decisions and historical events do not.
UPDATE source_tree_suggestions s SET review_revision=1
WHERE s.status='suggested' AND s.review_revision=0
  AND EXISTS (SELECT 1 FROM source_tree_suggestion_decisions d
              WHERE d.suggestion_id=s.id AND d.review_revision=0)
  AND EXISTS (SELECT 1 FROM source_name_review_events e
              WHERE e.suggestion_id=s.id AND e.event_type='reopened' AND e.revision=0);
