CREATE TABLE user_credentials (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  password_hash text NOT NULL,
  password_updated_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE auth_sessions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX auth_sessions_user_idx ON auth_sessions (user_id, expires_at DESC);
CREATE INDEX auth_sessions_expiry_idx ON auth_sessions (expires_at) WHERE revoked_at IS NULL;

CREATE TABLE user_roles (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role text NOT NULL CHECK (role IN ('registered', 'tree_owner', 'collaborator', 'researcher', 'moderator', 'admin')),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, role)
);

CREATE TABLE tree_collaborators (
  tree_id uuid NOT NULL REFERENCES trees(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  permission_level text NOT NULL DEFAULT 'edit' CHECK (permission_level IN ('view', 'edit', 'review')),
  invited_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tree_id, user_id)
);

CREATE TABLE tree_invitations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tree_id uuid NOT NULL REFERENCES trees(id) ON DELETE CASCADE,
  inviter_id uuid NOT NULL REFERENCES users(id),
  invitee_user_id uuid REFERENCES users(id),
  invitee_email text,
  token_hash text NOT NULL UNIQUE,
  permission_level text NOT NULL DEFAULT 'edit' CHECK (permission_level IN ('view', 'edit', 'review')),
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'revoked', 'expired')),
  expires_at timestamptz NOT NULL,
  accepted_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (invitee_user_id IS NOT NULL OR invitee_email IS NOT NULL)
);

CREATE INDEX tree_invitations_pending_idx ON tree_invitations (tree_id, status, expires_at);
