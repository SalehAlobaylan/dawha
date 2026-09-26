package identity

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// audit action names. Every mutation in this package writes exactly one of
// them, inside the transaction that made the change, so a row that exists always
// has the event that explains it and an event never outlives its row.
const (
	actionPersonCreated       = "person_created"
	actionPersonUpdated       = "person_updated"
	actionPersonDeleted       = "person_deleted"
	actionPersonAliasCreated  = "person_alias_created"
	actionPersonAliasUpdated  = "person_alias_updated"
	actionPersonAliasDeleted  = "person_alias_deleted"
	actionFamilyCreated       = "family_created"
	actionTribeCreated        = "tribe_created"
	actionBranchCreated       = "branch_created"
	actionPlaceCreated        = "place_created"
	actionRelationshipCreated = "entity_relationship_created"
	actionRelationshipUpdated = "entity_relationship_updated"
	actionRelationshipDeleted = "entity_relationship_deleted"
)

// audit entity types. They name the table the id belongs to, so the audit log
// stays readable without a join.
const (
	entityPerson       = "person"
	entityPersonAlias  = "person_alias"
	entityFamily       = "family"
	entityTribe        = "tribe"
	entityBranch       = "branch"
	entityPlace        = "place"
	entityRelationship = "entity_relationship"
)

// writeAudit is the last statement of every mutation in this package. It runs
// inside the same transaction as the change, so a failure here takes the change
// with it: there is no window in which a row exists without the event that
// explains it, and no window in which an event describes a change that was
// rolled back.
func writeAudit(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any, reasonAR string) error {
	var reason any
	if trimmed := strings.TrimSpace(reasonAR); trimmed != "" {
		reason = trimmed
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, before_value, after_value, reason_ar)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, actorID, action, entityType, entityID, auditValue(before), auditValue(after), reason)
	return err
}

func auditValue(value any) any {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}

// begin opens the transaction every mutation runs in. The authorization decision
// is taken inside it, before the first write, so a revoked role and a granted one
// are decided against the same snapshot the write lands in.
func (s *Service) begin(ctx context.Context) (pgx.Tx, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	return s.Pool.Begin(ctx)
}

func (s *Service) ready() error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	return nil
}

// textValue reads a nullable text column as a string.
func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

// uuidValue reads a nullable uuid column as a string.
func uuidValue(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

// uuidUUID reads a nullable uuid column as a value, and the nil uuid when the
// column is NULL. It is only used where the statement has already established
// that the column is present.
func uuidUUID(value pgtype.UUID) uuid.UUID {
	if !value.Valid {
		return uuid.Nil
	}
	return uuid.UUID(value.Bytes)
}

// timestampValue renders a nullable timestamptz as an RFC 3339 string. The
// write surface returns timestamps as text rather than as a parsed time so a
// caller in the Arabic web client never has to guess a zone.
func timestampValue(value pgtype.Timestamptz) string {
	if !value.Valid {
		return ""
	}
	return value.Time.UTC().Format("2006-01-02T15:04:05Z")
}
