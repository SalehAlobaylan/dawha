#!/usr/bin/env bash
set -euo pipefail

case "${1:-}" in
  "") finish=COMMIT ;;
  --dry-run) finish=ROLLBACK ;;
  *) printf 'Usage: %s [--dry-run]\n' "$0" >&2; exit 2 ;;
esac

cd "$(dirname "$0")/../.."
{
  printf 'BEGIN;\n'
  cat <<'SQL'
CREATE TEMP TABLE atlas_import_before ON COMMIT DROP AS
SELECT i.item_key, r.revision_key, o.occurrence_key, o.id
FROM atlas_source_occurrences o
JOIN atlas_source_item_revisions r ON r.id = o.item_revision_id
JOIN atlas_source_items i ON i.id = r.item_id;
SQL
  for manifest in db/seeds/003_atlas_source_occurrences.sql db/seeds/004_atlas_reference_catalogue.sql db/seeds/005_atlas_occurrence_passages.sql db/seeds/006_atlas_early_sources.sql db/seeds/007_atlas_oman_legend.sql db/seeds/008_atlas_context_sources.sql; do
    sed '/^BEGIN;$/d; /^COMMIT;$/d' "$manifest"
  done
  cat <<'SQL'
SELECT count(*) FILTER (WHERE b.id IS NULL) AS added_occurrences,
       count(*) FILTER (WHERE b.id IS NOT NULL) AS retained_occurrences,
       count(*) FILTER (WHERE o.tribe_id IS NULL) AS unresolved_occurrences,
       count(*) AS external_citations
FROM atlas_source_occurrences o
JOIN atlas_source_item_revisions r ON r.id = o.item_revision_id
JOIN atlas_source_items i ON i.id = r.item_id
LEFT JOIN atlas_import_before b ON b.id = o.id
WHERE i.source_id BETWEEN 'b0000000-0000-0000-0000-00000000e080' AND 'b0000000-0000-0000-0000-00000000e095';
SQL
  printf '%s;\n' "$finish"
} | docker compose exec -T db psql -X -v ON_ERROR_STOP=1 -U "${POSTGRES_USER:-dawha}" -d "${POSTGRES_DB:-dawha}"
