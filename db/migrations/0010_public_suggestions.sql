CREATE TABLE suggestions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tree_id uuid NOT NULL REFERENCES trees(id) ON DELETE CASCADE,
  version_id uuid NOT NULL REFERENCES tree_versions(id) ON DELETE CASCADE,
  node_id uuid NOT NULL REFERENCES tree_nodes(id) ON DELETE CASCADE,
  submitted_by uuid REFERENCES users(id) ON DELETE SET NULL,
  text_ar text NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected', 'converted')),
  question_id uuid REFERENCES open_questions(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX suggestions_tree_status_idx ON suggestions (tree_id, status, created_at DESC);
CREATE INDEX suggestions_node_idx ON suggestions (node_id, created_at DESC);

CREATE TABLE suggestion_reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  suggestion_id uuid NOT NULL REFERENCES suggestions(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id),
  decision text NOT NULL CHECK (decision IN ('accepted', 'rejected', 'converted')),
  note_ar text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX suggestion_reviews_suggestion_idx ON suggestion_reviews (suggestion_id, created_at DESC);
