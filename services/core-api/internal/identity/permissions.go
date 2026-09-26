package identity

import (
	"context"
	"errors"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// requireIdentityWrite is the single authorization gate of the identity write
// surface. It resolves the actor's platform roles inside the caller's
// transaction and hands them to the shared permission policy, so this service
// and every other service in the API decide with the same function.
//
// Two properties matter and are both deliberate:
//
//   - A global identity row is shared by every tree, so a right over one tree -
//     owning it, being a collaborator on it, even publishing it - is not a right
//     to change the shared rows. auth.Can encodes that: only the platform
//     collaborator, researcher, moderator and admin roles pass.
//   - There is no ownership escape hatch. Creating a person does not make its
//     creator its editor, because the row the next tree reads is the same row.
func requireIdentityWrite(ctx context.Context, q identityReader, actorID uuid.UUID) error {
	roles, err := loadRoles(ctx, q, actorID)
	if err != nil {
		return err
	}
	actor := auth.Actor{UserID: actorID.String(), Roles: roles}
	if !auth.Can(actor, auth.IdentityWrite, auth.Resource{}) {
		return ErrForbidden
	}
	return nil
}

func loadRoles(ctx context.Context, q identityReader, actorID uuid.UUID) ([]auth.Role, error) {
	rows, err := q.Query(ctx, `SELECT role FROM user_roles WHERE user_id = $1 ORDER BY role`, actorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := make([]auth.Role, 0, 4)
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, auth.Role(role))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return roles, nil
}

// entityTable maps a relationship endpoint type to the identity table that backs
// it. The map is closed on purpose: an unknown endpoint type is refused during
// validation, so no statement in this package is built from a caller string.
func entityTable(entityType string) (string, bool) {
	switch entityType {
	case "person":
		return "people", true
	case "family":
		return "families", true
	case "branch":
		return "branches", true
	case "tribe":
		return "tribes", true
	case "place":
		return "places", true
	}
	return "", false
}

// isConstraintViolation reports whether an error is a database constraint
// failure. A uniqueness conflict is a 409 through the handler, and the same
// mapping covers the foreign keys that keep a person out of a tree node, so
// neither surfaces as an unexplained 500.
func isConstraintViolation(err error) bool {
	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) {
		return false
	}
	return pgError.Code == "23505" || pgError.Code == "23503" || pgError.Code == "23514"
}
