CREATE TABLE sources (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  title_ar text NOT NULL,
  author_ar text,
  source_type text NOT NULL CHECK (source_type IN ('book', 'manuscript', 'family_document', 'archive_record', 'newspaper', 'article', 'research_paper', 'oral_testimony', 'website', 'published_tree', 'historical_registry', 'other')),
  publication_date_from date,
  publication_date_to date,
  edition_ar text,
  citation_ar text,
  location_ar text,
  metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
  dependency_status text NOT NULL DEFAULT 'unknown' CHECK (dependency_status IN ('unknown', 'independent', 'derived', 'likely_dependent')),
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sources_title_idx ON sources USING gin (title_ar gin_trgm_ops);
CREATE INDEX sources_type_idx ON sources (source_type);

ALTER TABLE person_aliases
  ADD CONSTRAINT person_aliases_source_fk FOREIGN KEY (source_id) REFERENCES sources(id);

ALTER TABLE historical_place_names
  ADD CONSTRAINT historical_place_names_source_fk FOREIGN KEY (source_id) REFERENCES sources(id);

CREATE TABLE source_files (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  storage_key text NOT NULL,
  original_filename_ar text,
  mime_type text,
  byte_size bigint,
  checksum_sha256 text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE source_passages (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  sequence_number integer NOT NULL,
  page_number integer,
  locator_ar text,
  text_ar text NOT NULL,
  normalized_text_ar text NOT NULL,
  embedding vector(1536),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_id, sequence_number)
);

CREATE INDEX source_passages_source_idx ON source_passages (source_id);
CREATE INDEX source_passages_embedding_idx ON source_passages USING hnsw (embedding vector_cosine_ops);

CREATE TABLE source_statements (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  source_passage_id uuid REFERENCES source_passages(id),
  statement_text_ar text NOT NULL,
  locator_ar text,
  extraction_method text NOT NULL DEFAULT 'manual' CHECK (extraction_method IN ('manual', 'ocr', 'ai', 'imported')),
  review_status text NOT NULL DEFAULT 'unreviewed' CHECK (review_status IN ('unreviewed', 'accepted', 'rejected', 'needs_review')),
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX source_statements_source_idx ON source_statements (source_id);
CREATE INDEX source_statements_text_idx ON source_statements USING gin (statement_text_ar gin_trgm_ops);

CREATE TABLE claims (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  subject_type text NOT NULL,
  subject_id uuid NOT NULL,
  predicate text NOT NULL,
  object_type text NOT NULL,
  object_id uuid NOT NULL,
  time_from date,
  time_to date,
  place_id uuid REFERENCES places(id),
  status text NOT NULL DEFAULT 'unresolved' CHECK (status IN ('documented', 'supported', 'contested', 'disputed', 'inferred', 'platform_generated', 'unresolved', 'contradicted', 'rejected', 'superseded', 'unknown')),
  notes_ar text,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX claims_subject_idx ON claims (subject_type, subject_id);
CREATE INDEX claims_object_idx ON claims (object_type, object_id);
CREATE INDEX claims_status_idx ON claims (status);

CREATE TABLE claim_versions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  claim_id uuid NOT NULL REFERENCES claims(id) ON DELETE CASCADE,
  version_number integer NOT NULL,
  snapshot jsonb NOT NULL,
  change_reason_ar text,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (claim_id, version_number)
);

CREATE TABLE claim_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  claim_id uuid NOT NULL REFERENCES claims(id) ON DELETE CASCADE,
  source_statement_id uuid REFERENCES source_statements(id),
  source_passage_id uuid REFERENCES source_passages(id),
  evidence_note_ar text,
  relation text NOT NULL DEFAULT 'supports' CHECK (relation IN ('supports', 'contextualizes', 'contradicts', 'refutes')),
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX claim_evidence_claim_idx ON claim_evidence (claim_id);

CREATE TABLE claim_counter_evidence (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  claim_id uuid NOT NULL REFERENCES claims(id) ON DELETE CASCADE,
  source_statement_id uuid REFERENCES source_statements(id),
  source_passage_id uuid REFERENCES source_passages(id),
  note_ar text,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE interpretations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name_ar text NOT NULL,
  description_ar text,
  maintainer_id uuid REFERENCES users(id),
  status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE interpretation_claims (
  interpretation_id uuid NOT NULL REFERENCES interpretations(id) ON DELETE CASCADE,
  claim_id uuid NOT NULL REFERENCES claims(id) ON DELETE CASCADE,
  position text NOT NULL DEFAULT 'supports' CHECK (position IN ('supports', 'opposes', 'mentions')),
  PRIMARY KEY (interpretation_id, claim_id)
);

CREATE TABLE interpretation_tree_versions (
  interpretation_id uuid NOT NULL REFERENCES interpretations(id) ON DELETE CASCADE,
  tree_version_id uuid NOT NULL REFERENCES tree_versions(id) ON DELETE CASCADE,
  role text NOT NULL DEFAULT 'represents' CHECK (role IN ('represents', 'publishes')),
  PRIMARY KEY (interpretation_id, tree_version_id)
);

CREATE TABLE platform_findings (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  finding_type text NOT NULL,
  title_ar text NOT NULL,
  explanation_ar text NOT NULL,
  status text NOT NULL DEFAULT 'needs_review' CHECK (status IN ('needs_review', 'confirmed', 'dismissed', 'investigating')),
  signals jsonb NOT NULL DEFAULT '{}'::jsonb,
  algorithm_version text,
  model_version text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE finding_entities (
  finding_id uuid NOT NULL REFERENCES platform_findings(id) ON DELETE CASCADE,
  entity_type text NOT NULL,
  entity_id uuid NOT NULL,
  PRIMARY KEY (finding_id, entity_type, entity_id)
);

CREATE TABLE finding_claims (
  finding_id uuid NOT NULL REFERENCES platform_findings(id) ON DELETE CASCADE,
  claim_id uuid NOT NULL REFERENCES claims(id) ON DELETE CASCADE,
  relation text NOT NULL DEFAULT 'concerns' CHECK (relation IN ('concerns', 'supports', 'opposes')),
  PRIMARY KEY (finding_id, claim_id, relation)
);

CREATE TABLE open_questions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  title_ar text NOT NULL,
  description_ar text,
  status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'under_investigation', 'resolved', 'reopened', 'archived')),
  priority text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high')),
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE question_entities (
  question_id uuid NOT NULL REFERENCES open_questions(id) ON DELETE CASCADE,
  entity_type text NOT NULL,
  entity_id uuid NOT NULL,
  PRIMARY KEY (question_id, entity_type, entity_id)
);

CREATE TABLE question_claims (
  question_id uuid NOT NULL REFERENCES open_questions(id) ON DELETE CASCADE,
  claim_id uuid NOT NULL REFERENCES claims(id) ON DELETE CASCADE,
  role text NOT NULL DEFAULT 'concerns' CHECK (role IN ('concerns', 'supports', 'opposes')),
  PRIMARY KEY (question_id, claim_id, role)
);

CREATE TABLE question_sources (
  question_id uuid NOT NULL REFERENCES open_questions(id) ON DELETE CASCADE,
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  role text NOT NULL DEFAULT 'context' CHECK (role IN ('context', 'supporting', 'counter_evidence', 'missing')),
  PRIMARY KEY (question_id, source_id, role)
);

CREATE TABLE question_findings (
  question_id uuid NOT NULL REFERENCES open_questions(id) ON DELETE CASCADE,
  finding_id uuid NOT NULL REFERENCES platform_findings(id) ON DELETE CASCADE,
  PRIMARY KEY (question_id, finding_id)
);

CREATE TABLE question_notes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  question_id uuid NOT NULL REFERENCES open_questions(id) ON DELETE CASCADE,
  note_ar text NOT NULL,
  created_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE source_dependencies (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  depends_on_source_id uuid REFERENCES sources(id) ON DELETE CASCADE,
  dependency_type text NOT NULL CHECK (dependency_type IN ('cites', 'derived_from', 'likely_paraphrase', 'shared_origin', 'unknown')),
  evidence_ar text,
  status text NOT NULL DEFAULT 'needs_review' CHECK (status IN ('needs_review', 'confirmed', 'rejected')),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (source_id, depends_on_source_id, dependency_type)
);

CREATE TABLE jobs (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  type text NOT NULL,
  payload jsonb NOT NULL DEFAULT '{}'::jsonb,
  status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'dead')),
  priority integer NOT NULL DEFAULT 0,
  attempts integer NOT NULL DEFAULT 0,
  run_at timestamptz NOT NULL DEFAULT now(),
  locked_at timestamptz,
  locked_by text,
  last_error text,
  idempotency_key text UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX jobs_claim_idx ON jobs (status, run_at, priority DESC);
