-- A chart interpretation is a versioned staging artifact. Its proposed people,
-- reviewed connections and page geometry remain tied to immutable page evidence
-- until a reviewer deliberately creates a private draft tree.
CREATE TABLE source_tree_drafts (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  source_file_id uuid NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
  created_by uuid NOT NULL REFERENCES users(id),
  title_ar text NOT NULL,
  state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'imported', 'retracted')),
  current_revision integer NOT NULL DEFAULT 1 CHECK (current_revision > 0),
  tree_id uuid REFERENCES trees(id),
  tree_version_id uuid REFERENCES tree_versions(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((state = 'draft' AND tree_id IS NULL AND tree_version_id IS NULL) OR
         (state IN ('imported', 'retracted') AND tree_id IS NOT NULL AND tree_version_id IS NOT NULL))
);

CREATE UNIQUE INDEX source_tree_drafts_one_open_per_file_idx
  ON source_tree_drafts (source_file_id) WHERE state = 'draft';
CREATE INDEX source_tree_drafts_source_idx
  ON source_tree_drafts (source_id, created_at DESC);

CREATE TABLE source_tree_draft_revisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  draft_id uuid NOT NULL REFERENCES source_tree_drafts(id) ON DELETE CASCADE,
  revision_number integer NOT NULL CHECK (revision_number > 0),
  reviewer_id uuid NOT NULL REFERENCES users(id),
  payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (draft_id, revision_number)
);

-- A second reviewer is a distinct account, not a second click by the first
-- reviewer. Fingerprints bind each decision to the exact connection geometry,
-- endpoints, labels, and relation so later edits require fresh review.
CREATE TABLE source_tree_draft_edge_reviews (
  draft_id uuid NOT NULL REFERENCES source_tree_drafts(id) ON DELETE CASCADE,
  edge_id uuid NOT NULL,
  edge_fingerprint text NOT NULL CHECK (edge_fingerprint ~ '^[0-9a-f]{64}$'),
  reviewer_id uuid NOT NULL REFERENCES users(id),
  revision_number integer NOT NULL CHECK (revision_number > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (draft_id, edge_id, edge_fingerprint, reviewer_id)
);

CREATE INDEX source_tree_draft_edge_reviews_edge_idx
  ON source_tree_draft_edge_reviews (draft_id, edge_id, edge_fingerprint, created_at);

CREATE TABLE source_tree_draft_imports (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  draft_id uuid NOT NULL UNIQUE REFERENCES source_tree_drafts(id),
  actor_id uuid NOT NULL REFERENCES users(id),
  tree_id uuid NOT NULL REFERENCES trees(id),
  tree_version_id uuid NOT NULL REFERENCES tree_versions(id),
  status text NOT NULL DEFAULT 'applied' CHECK (status IN ('applied', 'retracted')),
  manifest jsonb NOT NULL CHECK (jsonb_typeof(manifest) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  retracted_at timestamptz
);

CREATE INDEX source_tree_draft_imports_tree_idx
  ON source_tree_draft_imports (tree_id, tree_version_id);

CREATE TABLE source_tree_import_node_evidence (
  import_id uuid NOT NULL REFERENCES source_tree_draft_imports(id) ON DELETE CASCADE,
  tree_node_id uuid NOT NULL REFERENCES tree_nodes(id),
  draft_node_id uuid NOT NULL,
  page_evidence_id uuid NOT NULL REFERENCES source_page_evidence(id),
  label_ar text NOT NULL,
  region jsonb NOT NULL CHECK (jsonb_typeof(region) = 'object'),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retracted')),
  PRIMARY KEY (import_id, tree_node_id)
);

CREATE TABLE source_tree_import_edge_evidence (
  import_id uuid NOT NULL REFERENCES source_tree_draft_imports(id) ON DELETE CASCADE,
  tree_relationship_id uuid NOT NULL REFERENCES tree_relationships(id),
  draft_edge_id uuid NOT NULL,
  page_evidence_id uuid NOT NULL REFERENCES source_page_evidence(id),
  connector_path jsonb NOT NULL CHECK (jsonb_typeof(connector_path) = 'array'),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retracted')),
  PRIMARY KEY (import_id, tree_relationship_id)
);

CREATE INDEX source_tree_import_node_evidence_page_idx
  ON source_tree_import_node_evidence (page_evidence_id);
CREATE INDEX source_tree_import_edge_evidence_page_idx
  ON source_tree_import_edge_evidence (page_evidence_id);

-- Book assertions remain staged evidence. They do not create or merge canonical
-- people or claims; a reviewed assertion becomes a source statement for later
-- identity resolution and historical research.
ALTER TABLE source_statements
  ADD COLUMN source_page_evidence_id uuid REFERENCES source_page_evidence(id) ON DELETE SET NULL,
  ADD COLUMN source_page_review_revision_id uuid REFERENCES source_page_review_revisions(id) ON DELETE SET NULL;

CREATE TABLE source_book_assertions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  source_id uuid NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  source_file_id uuid NOT NULL REFERENCES source_files(id) ON DELETE CASCADE,
  page_evidence_id uuid NOT NULL REFERENCES source_page_evidence(id),
  page_review_revision_id uuid NOT NULL REFERENCES source_page_review_revisions(id),
  created_by uuid NOT NULL REFERENCES users(id),
  source_statement_id uuid REFERENCES source_statements(id),
  state text NOT NULL DEFAULT 'draft' CHECK (state IN ('draft', 'reviewed', 'rejected')),
  current_revision integer NOT NULL DEFAULT 1 CHECK (current_revision > 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((state = 'reviewed' AND source_statement_id IS NOT NULL) OR state <> 'reviewed')
);

CREATE INDEX source_book_assertions_file_idx
  ON source_book_assertions (source_file_id, created_at DESC);
CREATE INDEX source_book_assertions_page_idx
  ON source_book_assertions (page_evidence_id, created_at);

CREATE TABLE source_book_assertion_revisions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  assertion_id uuid NOT NULL REFERENCES source_book_assertions(id) ON DELETE CASCADE,
  revision_number integer NOT NULL CHECK (revision_number > 0),
  reviewer_id uuid NOT NULL REFERENCES users(id),
  payload jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (assertion_id, revision_number)
);

CREATE TABLE source_book_assertion_reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  assertion_id uuid NOT NULL REFERENCES source_book_assertions(id) ON DELETE CASCADE,
  revision_number integer NOT NULL CHECK (revision_number > 0),
  reviewer_id uuid NOT NULL REFERENCES users(id),
  decision text NOT NULL CHECK (decision IN ('reviewed', 'rejected')),
  note_ar text,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (assertion_id, reviewer_id, revision_number)
);
