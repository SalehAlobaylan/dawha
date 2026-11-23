ALTER TABLE external_region_candidates ADD COLUMN scan_sha256 text
    CHECK (scan_sha256 ~ '^[0-9a-f]{64}$');

UPDATE external_region_candidates c
SET scan_sha256 = s.metadata->>'file_sha256'
FROM sources s
WHERE c.source_id = s.id
  AND s.metadata->>'file_sha256' ~ '^[0-9a-f]{64}$';

CREATE FUNCTION pin_atlas_candidate_scan() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.scan_sha256 IS NULL THEN
        SELECT metadata->>'file_sha256' INTO NEW.scan_sha256
        FROM sources
        WHERE id = NEW.source_id AND metadata->>'file_sha256' ~ '^[0-9a-f]{64}$';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER external_region_candidate_scan
    BEFORE INSERT ON external_region_candidates
    FOR EACH ROW EXECUTE FUNCTION pin_atlas_candidate_scan();
