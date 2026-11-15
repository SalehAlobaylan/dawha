-- Phase 028-E: guard against two pipelines writing the same variant twice.
--
-- `historical_place_names` (0002) carries no uniqueness at all. Phase E's
-- importer and Phase F's alias-only accept both insert (place_id, name_type,
-- normalized_name_ar) triples; without a guard they race and a variant can
-- exist twice, which the search index then double-counts. The index is the
-- contract; existing duplicates are removed first so a migrated database that
-- already holds them can still take the index, keeping the earliest row.
DELETE FROM historical_place_names
 WHERE id IN (
   SELECT id FROM (
     SELECT id,
            ROW_NUMBER() OVER (PARTITION BY place_id, name_type, normalized_name_ar
                               ORDER BY created_at, id) AS keep_rank
       FROM historical_place_names
   ) ranked
   WHERE keep_rank > 1
 );

CREATE UNIQUE INDEX IF NOT EXISTS historical_place_names_unique
  ON historical_place_names (place_id, name_type, normalized_name_ar);
