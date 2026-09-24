CREATE TABLE IF NOT EXISTS schema_migrations (
  version text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL UNIQUE,
  display_name_ar text NOT NULL,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_profiles (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  bio_ar text,
  avatar_url text,
  preferred_locale text NOT NULL DEFAULT 'ar',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE people (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  canonical_name_ar text NOT NULL,
  normalized_name_ar text NOT NULL,
  gender text CHECK (gender IN ('male', 'female', 'unknown')),
  birth_date_from date,
  birth_date_to date,
  death_date_from date,
  death_date_to date,
  identity_status text NOT NULL DEFAULT 'unreviewed' CHECK (identity_status IN ('unreviewed', 'reviewed', 'disputed', 'merged')),
  notes_ar text,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX people_normalized_name_idx ON people USING gin (normalized_name_ar gin_trgm_ops);
CREATE INDEX people_identity_status_idx ON people (identity_status);

CREATE TABLE person_aliases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  person_id uuid NOT NULL REFERENCES people(id) ON DELETE CASCADE,
  value_ar text NOT NULL,
  normalized_value_ar text NOT NULL,
  alias_type text NOT NULL DEFAULT 'alternative_name' CHECK (alias_type IN ('alternative_name', 'kunyah', 'laqab', 'nisbah', 'source_spelling')),
  source_id uuid,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX person_aliases_normalized_idx ON person_aliases USING gin (normalized_value_ar gin_trgm_ops);
CREATE UNIQUE INDEX person_aliases_identity_idx ON person_aliases (person_id, normalized_value_ar, alias_type);

CREATE TABLE families (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  canonical_name_ar text NOT NULL,
  normalized_name_ar text NOT NULL,
  description_ar text,
  origin_place_id uuid,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX families_normalized_name_idx ON families USING gin (normalized_name_ar gin_trgm_ops);

CREATE TABLE family_aliases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  value_ar text NOT NULL,
  normalized_value_ar text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX family_aliases_identity_idx ON family_aliases (family_id, normalized_value_ar);

CREATE TABLE tribes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  canonical_name_ar text NOT NULL,
  normalized_name_ar text NOT NULL,
  description_ar text,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tribes_normalized_name_idx ON tribes USING gin (normalized_name_ar gin_trgm_ops);

CREATE TABLE tribe_aliases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tribe_id uuid NOT NULL REFERENCES tribes(id) ON DELETE CASCADE,
  value_ar text NOT NULL,
  normalized_value_ar text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX tribe_aliases_identity_idx ON tribe_aliases (tribe_id, normalized_value_ar);

CREATE TABLE branches (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  family_id uuid NOT NULL REFERENCES families(id) ON DELETE CASCADE,
  parent_branch_id uuid REFERENCES branches(id),
  canonical_name_ar text NOT NULL,
  normalized_name_ar text NOT NULL,
  valid_from date,
  valid_to date,
  notes_ar text,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX branches_family_idx ON branches (family_id);
CREATE INDEX branches_normalized_name_idx ON branches USING gin (normalized_name_ar gin_trgm_ops);

CREATE TABLE places (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  canonical_name_ar text NOT NULL,
  normalized_name_ar text NOT NULL,
  place_type text NOT NULL CHECK (place_type IN ('region', 'city', 'village', 'historical_settlement', 'other')),
  parent_place_id uuid REFERENCES places(id),
  geometry geometry(Point, 4326),
  approximate_area geometry(Geometry, 4326),
  notes_ar text,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX places_normalized_name_idx ON places USING gin (normalized_name_ar gin_trgm_ops);
CREATE INDEX places_geometry_idx ON places USING gist (geometry);

CREATE TABLE historical_place_names (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  place_id uuid NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  name_ar text NOT NULL,
  normalized_name_ar text NOT NULL,
  name_type text NOT NULL DEFAULT 'historical' CHECK (name_type IN ('historical', 'alternative', 'modern')),
  valid_from date,
  valid_to date,
  source_id uuid,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE entity_relationships (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subject_type text NOT NULL,
  subject_id uuid NOT NULL,
  predicate text NOT NULL,
  object_type text NOT NULL,
  object_id uuid NOT NULL,
  valid_from date,
  valid_to date,
  status text NOT NULL DEFAULT 'documented' CHECK (status IN ('documented', 'supported', 'contested', 'disputed', 'inferred', 'unresolved')),
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX entity_relationships_subject_idx ON entity_relationships (subject_type, subject_id);
CREATE INDEX entity_relationships_object_idx ON entity_relationships (object_type, object_id);
CREATE INDEX entity_relationships_predicate_idx ON entity_relationships (predicate);

CREATE TABLE trees (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name_ar text NOT NULL,
  description_ar text,
  visibility text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private', 'unlisted', 'public')),
  owner_id uuid NOT NULL REFERENCES users(id),
  parent_tree_id uuid REFERENCES trees(id),
  parent_version_id uuid,
  forked_by uuid REFERENCES users(id),
  forked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tree_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tree_id uuid NOT NULL REFERENCES trees(id) ON DELETE CASCADE,
  version_number integer NOT NULL,
  state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'published', 'archived')),
  publication_note_ar text,
  published_by uuid REFERENCES users(id),
  published_at timestamptz,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tree_id, version_number)
);

CREATE TABLE tree_nodes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tree_version_id uuid NOT NULL REFERENCES tree_versions(id) ON DELETE CASCADE,
  person_id uuid NOT NULL REFERENCES people(id),
  display_name_ar text NOT NULL,
  sort_order integer NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tree_version_id, person_id)
);

CREATE TABLE tree_relationships (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tree_version_id uuid NOT NULL REFERENCES tree_versions(id) ON DELETE CASCADE,
  subject_node_id uuid NOT NULL REFERENCES tree_nodes(id) ON DELETE CASCADE,
  object_node_id uuid NOT NULL REFERENCES tree_nodes(id) ON DELETE CASCADE,
  predicate text NOT NULL CHECK (predicate IN ('parent_of', 'spouse_of', 'sibling_of')),
  status text NOT NULL DEFAULT 'interpreted' CHECK (status IN ('interpreted', 'disputed', 'unresolved')),
  source_id uuid,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (subject_node_id <> object_node_id)
);

CREATE INDEX tree_nodes_version_idx ON tree_nodes (tree_version_id);
CREATE INDEX tree_relationships_version_idx ON tree_relationships (tree_version_id);

CREATE TABLE tree_forks (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tree_id uuid NOT NULL REFERENCES trees(id) ON DELETE CASCADE,
  parent_tree_id uuid NOT NULL REFERENCES trees(id),
  parent_version_id uuid NOT NULL REFERENCES tree_versions(id),
  forked_by uuid NOT NULL REFERENCES users(id),
  forked_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tree_id, parent_version_id)
);

CREATE TABLE audit_log (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_id uuid REFERENCES users(id),
  action text NOT NULL,
  entity_type text NOT NULL,
  entity_id uuid NOT NULL,
  before_value jsonb,
  after_value jsonb,
  reason_ar text,
  request_id text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id, created_at DESC);
