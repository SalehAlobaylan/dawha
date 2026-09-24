CREATE INDEX tree_collaborators_user_idx
  ON tree_collaborators (user_id);

CREATE INDEX tree_invitations_invitee_user_idx
  ON tree_invitations (invitee_user_id)
  WHERE invitee_user_id IS NOT NULL;

CREATE INDEX tree_invitations_email_idx
  ON tree_invitations (lower(invitee_email))
  WHERE invitee_email IS NOT NULL;

CREATE UNIQUE INDEX tree_invitations_pending_user_idx
  ON tree_invitations (tree_id, invitee_user_id)
  WHERE status = 'pending' AND invitee_user_id IS NOT NULL;

CREATE UNIQUE INDEX tree_invitations_pending_email_idx
  ON tree_invitations (tree_id, lower(invitee_email))
  WHERE status = 'pending' AND invitee_email IS NOT NULL;

CREATE TABLE tree_change_log (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tree_id uuid NOT NULL REFERENCES trees(id) ON DELETE CASCADE,
  tree_version_id uuid REFERENCES tree_versions(id) ON DELETE SET NULL,
  actor_id uuid REFERENCES users(id) ON DELETE SET NULL,
  action text NOT NULL,
  entity_type text NOT NULL,
  entity_id uuid NOT NULL,
  before_value jsonb,
  after_value jsonb,
  reason_ar text,
  request_id text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX tree_change_log_tree_created_idx
  ON tree_change_log (tree_id, created_at DESC);

CREATE INDEX tree_change_log_tree_version_created_idx
  ON tree_change_log (tree_id, tree_version_id, created_at DESC);
