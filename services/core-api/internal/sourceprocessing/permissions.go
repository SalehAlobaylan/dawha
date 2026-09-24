package sourceprocessing

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type queryExecutor interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func canReviewSource(ctx context.Context, q queryExecutor, sourceID, actorID uuid.UUID) (bool, error) {
	var allowed bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sources s
			WHERE s.id = $1 AND EXISTS (
				SELECT 1 FROM user_roles ur
				WHERE ur.user_id = $2 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')
			)
		)
	`, sourceID, actorID).Scan(&allowed)
	return allowed, err
}

func canManageSource(ctx context.Context, q queryExecutor, sourceID, actorID uuid.UUID) (bool, error) {
	var allowed bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM sources s
			WHERE s.id = $1 AND (
				s.created_by = $2
				OR EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin'))
			)
		)
	`, sourceID, actorID).Scan(&allowed)
	return allowed, err
}
