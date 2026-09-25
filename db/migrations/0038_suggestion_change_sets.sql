-- An accepted suggestion carries a typed change set: the target the reviewer approved
-- and the exact record the review created. Storing the change beside the review keeps
-- an applied change reviewable without replaying the audit log, and the result columns
-- tie the change to the row that was written. The target is constrained to the typed
-- domain records a change set may touch, so a review can never record an unstructured
-- edit to the tree.
CREATE TABLE suggestion_change_sets (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  suggestion_id uuid NOT NULL REFERENCES suggestions(id) ON DELETE CASCADE,
  review_id uuid REFERENCES suggestion_reviews(id) ON DELETE SET NULL,
  target text NOT NULL CHECK (target IN ('person', 'relationship', 'claim', 'source_link')),
  change jsonb NOT NULL,
  result_type text NOT NULL,
  result_id uuid NOT NULL,
  applied_by uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX suggestion_change_sets_suggestion_idx ON suggestion_change_sets (suggestion_id, created_at DESC);
