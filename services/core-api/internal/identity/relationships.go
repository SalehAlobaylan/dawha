package identity

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// CreateEntityRelationship records a generic typed relationship.
//
// A relationship is a research record: internal/entityresolution and
// internal/contradiction read this table, and no public read path does, so a
// relationship created here is invisible to an anonymous reader by the same
// argument that governs a person. It is always created unresolved - the status is
// not a request field - because a create cannot assert a settled reading.
func (s *Service) CreateEntityRelationship(ctx context.Context, actorID string, input CreateEntityRelationshipInput) (EntityRelationshipView, error) {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	plan, err := validateRelationshipInput(input.SubjectType, input.SubjectID, input.Predicate, input.ObjectType, input.ObjectID, input.ValidFrom, input.ValidTo)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireIdentityWrite(ctx, tx, actorUUID); err != nil {
		return EntityRelationshipView{}, err
	}
	if err := requireExistingEntity(ctx, tx, plan.subjectType, plan.subjectID); err != nil {
		return EntityRelationshipView{}, err
	}
	if err := requireExistingEntity(ctx, tx, plan.objectType, plan.objectID); err != nil {
		return EntityRelationshipView{}, err
	}
	relationshipID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO entity_relationships (id, subject_type, subject_id, predicate, object_type, object_id, valid_from, valid_to, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'unresolved', $9)
	`, relationshipID, plan.subjectType, plan.subjectID, plan.predicate, plan.objectType, plan.objectID, plan.validity.From, plan.validity.To, actorUUID); err != nil {
		return EntityRelationshipView{}, mapWriteError(err)
	}
	view, err := readRelationship(ctx, tx, relationshipID)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, actionRelationshipCreated, entityRelationship, relationshipID, nil, relationshipSnapshot(view), ""); err != nil {
		return EntityRelationshipView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EntityRelationshipView{}, err
	}
	return view, nil
}

// UpdateEntityRelationship refines an unresolved relationship.
//
// Two refusals apply, and both are about a decision someone already made rather
// than about this row's own storage. A relationship whose status is no longer
// unresolved records a settled reading: reversing it is a reviewed action, not an
// edit. And a relationship whose endpoint is a person a published tree version
// already published sits next to an interpretation the platform has already
// stood behind, so it is not changed from here.
func (s *Service) UpdateEntityRelationship(ctx context.Context, relationshipID, actorID string, input UpdateEntityRelationshipInput) (EntityRelationshipView, error) {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	relationshipUUID, err := parseID(relationshipID)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	predicate, err := requiredEnum(input.Predicate, relationshipPredicates)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	status, err := optionalEnum(input.Status, "unresolved", relationshipStatuses)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	validity, err := parseDateRange(input.ValidFrom, input.ValidTo)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	reason, err := optionalText(input.ReasonAR, maxReasonRunes)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireIdentityWrite(ctx, tx, actorUUID); err != nil {
		return EntityRelationshipView{}, err
	}
	before, err := lockRelationship(ctx, tx, relationshipUUID)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	if err := refusePublishedEndpoint(ctx, tx, before.SubjectType, before.SubjectID); err != nil {
		return EntityRelationshipView{}, err
	}
	if err := refusePublishedEndpoint(ctx, tx, before.ObjectType, before.ObjectID); err != nil {
		return EntityRelationshipView{}, err
	}
	if before.Status != "unresolved" {
		return EntityRelationshipView{}, ErrSettledInterpretation
	}
	if _, err := tx.Exec(ctx, `
		UPDATE entity_relationships
		SET predicate = $1, valid_from = $2, valid_to = $3, status = $4, updated_at = now()
		WHERE id = $5
	`, predicate, validity.From, validity.To, status, relationshipUUID); err != nil {
		return EntityRelationshipView{}, mapWriteError(err)
	}
	view, err := readRelationship(ctx, tx, relationshipUUID)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, actionRelationshipUpdated, entityRelationship, relationshipUUID, relationshipSnapshot(before), relationshipSnapshot(view), reason); err != nil {
		return EntityRelationshipView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return EntityRelationshipView{}, err
	}
	return view, nil
}

// DeleteEntityRelationship removes an unresolved relationship, under the same two
// refusals as the update.
func (s *Service) DeleteEntityRelationship(ctx context.Context, relationshipID, actorID, reasonAR string) error {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return err
	}
	relationshipUUID, err := parseID(relationshipID)
	if err != nil {
		return err
	}
	reason, err := optionalText(reasonAR, maxReasonRunes)
	if err != nil {
		return err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := requireIdentityWrite(ctx, tx, actorUUID); err != nil {
		return err
	}
	before, err := lockRelationship(ctx, tx, relationshipUUID)
	if err != nil {
		return err
	}
	if err := refusePublishedEndpoint(ctx, tx, before.SubjectType, before.SubjectID); err != nil {
		return err
	}
	if err := refusePublishedEndpoint(ctx, tx, before.ObjectType, before.ObjectID); err != nil {
		return err
	}
	if before.Status != "unresolved" {
		return ErrSettledInterpretation
	}
	if _, err := tx.Exec(ctx, `DELETE FROM entity_relationships WHERE id = $1`, relationshipUUID); err != nil {
		return mapWriteError(err)
	}
	return commitWithAudit(ctx, tx, actorUUID, actionRelationshipDeleted, entityRelationship, relationshipUUID, relationshipSnapshot(before), nil, reason)
}

// GetEntityRelationship reads one relationship. The gate is the write role for the
// same reason as everywhere else in this package: the relationship table has no
// visibility column, and this API must not become a public reader of it.
func (s *Service) GetEntityRelationship(ctx context.Context, relationshipID, actorID string) (EntityRelationshipView, error) {
	if err := s.gateRead(ctx, actorID); err != nil {
		return EntityRelationshipView{}, err
	}
	relationshipUUID, err := parseID(relationshipID)
	if err != nil {
		return EntityRelationshipView{}, err
	}
	return readRelationship(ctx, s.Pool, relationshipUUID)
}

func (s *Service) ListEntityRelationships(ctx context.Context, actorID, search string) (ListResponse[EntityRelationshipView], error) {
	if err := s.gateRead(ctx, actorID); err != nil {
		return ListResponse[EntityRelationshipView]{}, err
	}
	term, err := validateListQuery(search)
	if err != nil {
		return ListResponse[EntityRelationshipView]{}, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT `+relationshipColumns+`
		FROM entity_relationships r
		WHERE $1 = '' OR r.predicate ILIKE '%' || $1 || '%' OR r.status ILIKE '%' || $1 || '%'
		ORDER BY r.created_at DESC, r.id
		LIMIT $2
	`, term, defaultListLimit)
	if err != nil {
		return ListResponse[EntityRelationshipView]{}, err
	}
	defer rows.Close()
	items := make([]EntityRelationshipView, 0)
	for rows.Next() {
		item, err := scanRelationship(rows)
		if err != nil {
			return ListResponse[EntityRelationshipView]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResponse[EntityRelationshipView]{}, err
	}
	return ListResponse[EntityRelationshipView]{Kind: "entity-relationships", Query: term, Items: items}, nil
}

// ------------------------------------------------------------- shared pieces --

// refusePublishedEndpoint applies the person published check only where the
// endpoint can be a person. A family, tribe, branch or place endpoint has no
// published-version membership at all - no tree version references those tables -
// so the check has nothing to ask about them and says so instead of guessing.
func refusePublishedEndpoint(ctx context.Context, q identityReader, entityType, entityID string) error {
	if entityType != "person" {
		return nil
	}
	parsed, err := uuid.Parse(strings.TrimSpace(entityID))
	if err != nil {
		return err
	}
	return refusePublishedPerson(ctx, q, parsed)
}

// requireExistingEntity refuses a relationship that names an endpoint which does
// not exist. The table comes from the closed entityTable map, so the statement is
// never assembled from a caller string.
func requireExistingEntity(ctx context.Context, q identityReader, entityType string, entityID uuid.UUID) error {
	table, ok := entityTable(entityType)
	if !ok {
		return ErrValidation
	}
	return requireExistingRow(ctx, q, table, entityID)
}

type relationshipPlan struct {
	subjectType string
	subjectID   uuid.UUID
	predicate   string
	objectType  string
	objectID    uuid.UUID
	validity    dateRange
}

func validateRelationshipInput(subjectType, subjectID, predicate, objectType, objectID, validFrom, validTo string) (relationshipPlan, error) {
	resolvedSubjectType, err := optionalEnum(subjectType, "person", relationshipEntityTypes)
	if err != nil {
		return relationshipPlan{}, err
	}
	resolvedObjectType, err := optionalEnum(objectType, "person", relationshipEntityTypes)
	if err != nil {
		return relationshipPlan{}, err
	}
	parsedSubjectID, err := requiredID(subjectID)
	if err != nil {
		return relationshipPlan{}, err
	}
	parsedObjectID, err := requiredID(objectID)
	if err != nil {
		return relationshipPlan{}, err
	}
	// An entity cannot be related to itself under any predicate the vocabulary
	// holds; every one of them describes two different participants.
	if resolvedSubjectType == resolvedObjectType && parsedSubjectID == parsedObjectID {
		return relationshipPlan{}, ErrValidation
	}
	resolvedPredicate, err := requiredEnum(predicate, relationshipPredicates)
	if err != nil {
		return relationshipPlan{}, err
	}
	validity, err := parseDateRange(validFrom, validTo)
	if err != nil {
		return relationshipPlan{}, err
	}
	return relationshipPlan{
		subjectType: resolvedSubjectType,
		subjectID:   parsedSubjectID,
		predicate:   resolvedPredicate,
		objectType:  resolvedObjectType,
		objectID:    parsedObjectID,
		validity:    validity,
	}, nil
}

const relationshipColumns = `r.id, r.subject_type, r.subject_id, r.predicate, r.object_type, r.object_id, r.valid_from, r.valid_to, r.status, r.created_at, r.updated_at`

func scanRelationship(row pgx.Row) (EntityRelationshipView, error) {
	var view EntityRelationshipView
	var id, subjectID, objectID pgtype.UUID
	var validFrom, validTo pgtype.Date
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &view.SubjectType, &subjectID, &view.Predicate, &view.ObjectType, &objectID, &validFrom, &validTo, &view.Status, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return EntityRelationshipView{}, ErrNotFound
		}
		return EntityRelationshipView{}, err
	}
	view.ID = uuidValue(id)
	view.SubjectID = uuidValue(subjectID)
	view.ObjectID = uuidValue(objectID)
	view.ValidFrom = dateValue(validFrom)
	view.ValidTo = dateValue(validTo)
	view.CreatedAt = timestampValue(createdAt)
	view.UpdatedAt = timestampValue(updatedAt)
	return view, nil
}

func readRelationship(ctx context.Context, q identityReader, relationshipID uuid.UUID) (EntityRelationshipView, error) {
	return scanRelationship(q.QueryRow(ctx, `SELECT `+relationshipColumns+` FROM entity_relationships r WHERE r.id = $1`, relationshipID))
}

func lockRelationship(ctx context.Context, tx pgx.Tx, relationshipID uuid.UUID) (EntityRelationshipView, error) {
	return scanRelationship(tx.QueryRow(ctx, `SELECT `+relationshipColumns+` FROM entity_relationships r WHERE r.id = $1 FOR UPDATE`, relationshipID))
}

func relationshipSnapshot(view EntityRelationshipView) map[string]any {
	return map[string]any{
		"subject_type": view.SubjectType,
		"subject_id":   view.SubjectID,
		"predicate":    view.Predicate,
		"object_type":  view.ObjectType,
		"object_id":    view.ObjectID,
		"valid_from":   view.ValidFrom,
		"valid_to":     view.ValidTo,
		"status":       view.Status,
	}
}
