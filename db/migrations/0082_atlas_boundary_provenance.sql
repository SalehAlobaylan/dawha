ALTER TABLE region_boundary_reviews
    ADD COLUMN valid_from date,
    ADD COLUMN valid_to date,
    ADD COLUMN occurrence_id uuid REFERENCES atlas_source_occurrences(id),
    ADD CHECK (valid_from <= valid_to);

ALTER TABLE historical_regions
    ADD COLUMN boundary_locator_ar text,
    ADD COLUMN boundary_occurrence_id uuid REFERENCES atlas_source_occurrences(id);
