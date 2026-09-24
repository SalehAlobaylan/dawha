package contradiction

import (
	"context"

	"github.com/google/uuid"
)

func requireRole(ctx context.Context, q queryer, actorID string, roles ...string) error {
	actor, err := uuid.Parse(actorID)
	if err != nil {
		return ErrForbidden
	}
	var allowed bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role = ANY($2::text[]))`, actor, roles).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}
