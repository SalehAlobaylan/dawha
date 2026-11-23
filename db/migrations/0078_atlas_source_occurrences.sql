CREATE TABLE atlas_source_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    item_key text NOT NULL CHECK (length(item_key) BETWEEN 1 AND 200),
    UNIQUE (source_id, item_key)
);

CREATE TABLE atlas_source_item_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id uuid NOT NULL REFERENCES atlas_source_items(id) ON DELETE CASCADE,
    revision_key text NOT NULL CHECK (length(revision_key) BETWEEN 1 AND 200),
    locator_ar text NOT NULL CHECK (length(locator_ar) BETWEEN 1 AND 500),
    external_url text NOT NULL CHECK (external_url LIKE 'https://%' AND length(external_url) <= 3000),
    rights_ar text NOT NULL,
    credit_ar text NOT NULL,
    period_ar text NOT NULL DEFAULT '',
    depicted_from_year integer,
    depicted_to_year integer,
    CHECK (depicted_from_year <= depicted_to_year),
    UNIQUE (item_id, revision_key)
);

CREATE TABLE atlas_source_occurrences (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_revision_id uuid NOT NULL REFERENCES atlas_source_item_revisions(id) ON DELETE CASCADE,
    occurrence_key text NOT NULL,
    original_text text NOT NULL CHECK (length(original_text) BETWEEN 1 AND 500),
    display_ar text NOT NULL CHECK (length(display_ar) BETWEEN 1 AND 200),
    language text NOT NULL CHECK (language IN ('ar','en','und')),
    reading_method text NOT NULL CHECK (reading_method IN ('original','transliteration','catalogue','transcript')),
    note_ar text NOT NULL DEFAULT '',
    tribe_id uuid REFERENCES tribes(id) ON DELETE SET NULL,
    published boolean NOT NULL DEFAULT false,
    UNIQUE (item_revision_id, occurrence_key)
);
CREATE INDEX atlas_source_occurrences_tribe_idx ON atlas_source_occurrences (tribe_id) WHERE published;

-- Search keys are versioned separately from the original forms.
CREATE FUNCTION atlas_occurrence_search_key_v1(value text) RETURNS text
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS $$
    SELECT btrim(regexp_replace(
        translate(lower(regexp_replace(normalize(value, NFKC), '[ؐ-ًؚ-ٰٟـ̀-ͯ]', '', 'g')),
                  'أإآٱىة٠١٢٣٤٥٦٧٨٩۰۱۲۳۴۵۶۷۸۹', 'اااايه01234567890123456789'),
        '[[:space:][:punct:]]+', ' ', 'g'))
$$;
