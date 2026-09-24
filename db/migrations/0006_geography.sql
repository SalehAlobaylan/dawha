CREATE TABLE historical_regions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name_ar text NOT NULL,
  normalized_name_ar text NOT NULL,
  geometry geometry(Geometry, 4326),
  valid_from date,
  valid_to date,
  certainty text NOT NULL DEFAULT 'approximate' CHECK (certainty IN ('precise', 'approximate', 'disputed')),
  source_id uuid REFERENCES sources(id),
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX historical_regions_name_idx ON historical_regions USING gin (normalized_name_ar gin_trgm_ops);
CREATE INDEX historical_regions_geometry_idx ON historical_regions USING gist (geometry);

CREATE TABLE geographic_associations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  entity_type text NOT NULL,
  entity_id uuid NOT NULL,
  place_id uuid NOT NULL REFERENCES places(id),
  relation_type text NOT NULL CHECK (relation_type IN ('documented_in', 'lived_in', 'originated_in', 'associated_with', 'mentioned_at')),
  time_from date,
  time_to date,
  status text NOT NULL DEFAULT 'documented' CHECK (status IN ('documented', 'interpreted', 'platform_inferred', 'disputed', 'unresolved')),
  certainty text NOT NULL DEFAULT 'approximate' CHECK (certainty IN ('precise', 'approximate', 'uncertain')),
  claim_id uuid REFERENCES claims(id),
  source_id uuid REFERENCES sources(id),
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX geographic_associations_entity_idx ON geographic_associations (entity_type, entity_id);
CREATE INDEX geographic_associations_place_idx ON geographic_associations (place_id);
CREATE INDEX geographic_associations_time_idx ON geographic_associations (time_from, time_to);

CREATE TABLE migration_events (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subject_type text NOT NULL,
  subject_id uuid NOT NULL,
  from_place_id uuid REFERENCES places(id),
  to_place_id uuid REFERENCES places(id),
  time_from date,
  time_to date,
  status text NOT NULL DEFAULT 'unresolved' CHECK (status IN ('documented', 'interpreted', 'platform_inferred', 'disputed', 'unresolved')),
  certainty text NOT NULL DEFAULT 'uncertain' CHECK (certainty IN ('precise', 'approximate', 'uncertain')),
  claim_id uuid REFERENCES claims(id),
  source_id uuid REFERENCES sources(id),
  notes_ar text,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (from_place_id IS NOT NULL OR to_place_id IS NOT NULL)
);

CREATE INDEX migration_events_subject_idx ON migration_events (subject_type, subject_id);
CREATE INDEX migration_events_time_idx ON migration_events (time_from, time_to);

CREATE TABLE spatial_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  geographic_association_id uuid REFERENCES geographic_associations(id) ON DELETE CASCADE,
  migration_event_id uuid REFERENCES migration_events(id) ON DELETE CASCADE,
  source_passage_id uuid REFERENCES source_passages(id),
  geometry geometry(Geometry, 4326),
  evidence_text_ar text,
  review_status text NOT NULL DEFAULT 'needs_review' CHECK (review_status IN ('unreviewed', 'accepted', 'rejected', 'needs_review')),
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (geographic_association_id IS NOT NULL OR migration_event_id IS NOT NULL)
);

CREATE INDEX spatial_evidence_geometry_idx ON spatial_evidence USING gist (geometry);
