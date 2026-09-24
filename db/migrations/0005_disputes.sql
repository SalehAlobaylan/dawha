CREATE TABLE disputes (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  title_ar text NOT NULL,
  description_ar text,
  status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'under_review', 'resolved', 'reopened', 'archived')),
  resolution_ar text,
  created_by uuid REFERENCES users(id),
  resolved_by uuid REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE dispute_claims (
  dispute_id uuid NOT NULL REFERENCES disputes(id) ON DELETE CASCADE,
  claim_id uuid NOT NULL REFERENCES claims(id) ON DELETE CASCADE,
  position text NOT NULL DEFAULT 'concerns' CHECK (position IN ('concerns', 'supports', 'opposes', 'mentions')),
  PRIMARY KEY (dispute_id, claim_id, position)
);

CREATE TABLE question_disputes (
  question_id uuid NOT NULL REFERENCES open_questions(id) ON DELETE CASCADE,
  dispute_id uuid NOT NULL REFERENCES disputes(id) ON DELETE CASCADE,
  PRIMARY KEY (question_id, dispute_id)
);

CREATE INDEX disputes_status_idx ON disputes (status, updated_at DESC);
