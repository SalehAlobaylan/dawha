-- Phase 028-F (completion): append-only pairing-decision history.
--
-- The candidate row keeps the latest reviewer answer beside the machine
-- proposal. This table keeps every answer, so a reopened candidate still has
-- a provenance trail and a later reviewer can see what happened before.
CREATE TABLE pairing_decision_history (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  candidate_id uuid NOT NULL REFERENCES external_place_candidates(id) ON DELETE CASCADE,
  reviewer_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  action text NOT NULL CHECK (action IN ('alias', 'link', 'reject', 'defer', 'reopen')),
  from_status text NOT NULL,
  to_status text NOT NULL,
  note_ar text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX pairing_decision_history_candidate_idx
  ON pairing_decision_history (candidate_id, created_at, id);
CREATE INDEX pairing_decision_history_reviewer_idx
  ON pairing_decision_history (reviewer_id, created_at DESC);
