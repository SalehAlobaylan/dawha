package trees

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable   = errors.New("tree database is unavailable")
	ErrNotFound              = errors.New("tree not found")
	ErrForbidden             = errors.New("tree access is forbidden")
	ErrNoDraft               = errors.New("tree has no draft version")
	ErrDuplicateRelationship = errors.New("relationship already exists in this draft")
	ErrStaleVersion          = errors.New("tree draft version is stale")
	ErrValidation            = errors.New("tree input is invalid")
)

type CreateTreeInput struct {
	Name        string        `json:"name_ar"`
	Description string        `json:"description_ar"`
	Visibility  string        `json:"visibility"`
	People      []PersonInput `json:"people"`
}

type PersonInput struct {
	CanonicalName string `json:"canonical_name_ar"`
	Gender        string `json:"gender"`
	BirthDateFrom string `json:"birth_date_from"`
	BirthDateTo   string `json:"birth_date_to"`
	DeathDateFrom string `json:"death_date_from"`
	DeathDateTo   string `json:"death_date_to"`
}

type AddRelationshipInput struct {
	SubjectNodeID string `json:"subject_node_id"`
	ObjectNodeID  string `json:"object_node_id"`
	Predicate     string `json:"predicate"`
	Status        string `json:"status"`
}

type UpdateRelationshipInput struct {
	Status            string `json:"status"`
	ExpectedVersionID string `json:"expected_version_id"`
	ReasonAR          string `json:"reason_ar"`
}

type TreeSummary struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Description         string    `json:"description"`
	Visibility          string    `json:"visibility"`
	OwnerID             string    `json:"ownerId"`
	UpdatedAt           time.Time `json:"updatedAt"`
	LatestVersionID     string    `json:"latestVersionId"`
	LatestVersionNumber int       `json:"latestVersionNumber"`
	LatestState         string    `json:"latestState"`
	People              int       `json:"people"`
	Relationships       int       `json:"relationships"`
	Unresolved          int       `json:"unresolved"`
}

type TreeVersionView struct {
	ID              string     `json:"id"`
	Number          int        `json:"number"`
	State           string     `json:"state"`
	PublicationNote string     `json:"publicationNote"`
	PublishedAt     *time.Time `json:"publishedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type TreeNodeView struct {
	ID          string `json:"id"`
	PersonID    string `json:"personId"`
	DisplayName string `json:"displayName"`
	SortOrder   int    `json:"sortOrder"`
	Years       string `json:"years"`
	Role        string `json:"role"`
	Tone        string `json:"tone"`
	SourceCount int    `json:"sourceCount"`
	Note        string `json:"note"`
}

type TreeRelationshipView struct {
	ID            string `json:"id"`
	SubjectNodeID string `json:"subjectNodeId"`
	ObjectNodeID  string `json:"objectNodeId"`
	Predicate     string `json:"predicate"`
	Status        string `json:"status"`
}

type TreePermissions struct {
	CanEdit                bool   `json:"canEdit"`
	CanPublish             bool   `json:"canPublish"`
	CanManageCollaborators bool   `json:"canManageCollaborators"`
	PermissionLevel        string `json:"permissionLevel"`
}

type TreeDetail struct {
	Tree            TreeSummary            `json:"tree"`
	SelectedVersion TreeVersionView        `json:"selectedVersion"`
	Permissions     TreePermissions        `json:"permissions"`
	Versions        []TreeVersionView      `json:"versions"`
	Nodes           []TreeNodeView         `json:"nodes"`
	Relationships   []TreeRelationshipView `json:"relationships"`
}

type Service struct {
	Pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

func (s *Service) CreateTree(ctx context.Context, ownerID string, input CreateTreeInput) (TreeDetail, error) {
	if s == nil || s.Pool == nil {
		return TreeDetail{}, ErrDatabaseUnavailable
	}
	ownerUUID, err := uuid.Parse(ownerID)
	if err != nil {
		return TreeDetail{}, ErrValidation
	}
	input, err = validateCreateInput(input)
	if err != nil {
		return TreeDetail{}, err
	}

	treeID := uuid.New()
	versionID := uuid.New()
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TreeDetail{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO trees (id, name_ar, description_ar, visibility, owner_id)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5)
	`, treeID, input.Name, input.Description, input.Visibility, ownerUUID); err != nil {
		return TreeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tree_versions (id, tree_id, version_number, state, created_by)
		VALUES ($1, $2, 1, 'draft', $3)
	`, versionID, treeID, ownerUUID); err != nil {
		return TreeDetail{}, err
	}

	for index, person := range input.People {
		birthFrom, birthTo, deathFrom, deathTo, err := parsePersonDates(person)
		if err != nil {
			return TreeDetail{}, err
		}
		personID := uuid.New()
		nodeID := uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO people (id, canonical_name_ar, normalized_name_ar, gender, birth_date_from, birth_date_to, death_date_from, death_date_to, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, personID, person.CanonicalName, identity.NormalizeArabicName(person.CanonicalName), person.Gender, birthFrom, birthTo, deathFrom, deathTo, ownerUUID); err != nil {
			return TreeDetail{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order)
			VALUES ($1, $2, $3, $4, $5)
		`, nodeID, versionID, personID, person.CanonicalName, index); err != nil {
			return TreeDetail{}, err
		}
	}

	auditValue, _ := json.Marshal(map[string]any{"name": input.Name, "visibility": input.Visibility})
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, after_value)
		VALUES ($1, 'tree_created', 'tree', $2, $3)
	`, ownerUUID, treeID, auditValue); err != nil {
		return TreeDetail{}, err
	}
	if err := writeTreeChange(ctx, tx, treeID, &versionID, ownerUUID, "tree_created", "tree", treeID, nil, map[string]any{"name": input.Name, "visibility": input.Visibility}, ""); err != nil {
		return TreeDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TreeDetail{}, err
	}
	return s.GetTree(ctx, treeID.String(), ownerID)
}

func (s *Service) AddPerson(ctx context.Context, treeID, ownerID string, input PersonInput) (TreeDetail, error) {
	if s == nil || s.Pool == nil {
		return TreeDetail{}, ErrDatabaseUnavailable
	}
	treeUUID, err := uuid.Parse(treeID)
	if err != nil {
		return TreeDetail{}, ErrNotFound
	}
	ownerUUID, err := uuid.Parse(ownerID)
	if err != nil {
		return TreeDetail{}, ErrForbidden
	}
	input, err = validatePersonInput(input)
	if err != nil {
		return TreeDetail{}, err
	}
	birthFrom, birthTo, deathFrom, deathTo, err := parsePersonDates(input)
	if err != nil {
		return TreeDetail{}, err
	}
	if _, err := s.treeSummary(ctx, treeUUID); err != nil {
		return TreeDetail{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TreeDetail{}, err
	}
	defer tx.Rollback(ctx)
	versionID, _, err := s.lockLatestDraft(ctx, tx, treeUUID)
	if err != nil {
		return TreeDetail{}, err
	}
	allowed, err := s.canEditDraftTx(ctx, tx, treeUUID, ownerUUID)
	if err != nil {
		return TreeDetail{}, err
	}
	if !allowed {
		return TreeDetail{}, ErrForbidden
	}

	var sortOrder int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(sort_order), -1) + 1
		FROM tree_nodes
		WHERE tree_version_id = $1
	`, versionID).Scan(&sortOrder); err != nil {
		return TreeDetail{}, err
	}
	personID := uuid.New()
	nodeID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO people (id, canonical_name_ar, normalized_name_ar, gender, birth_date_from, birth_date_to, death_date_from, death_date_to, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, personID, input.CanonicalName, identity.NormalizeArabicName(input.CanonicalName), input.Gender, birthFrom, birthTo, deathFrom, deathTo, ownerUUID); err != nil {
		return TreeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order)
		VALUES ($1, $2, $3, $4, $5)
	`, nodeID, versionID, personID, input.CanonicalName, sortOrder); err != nil {
		return TreeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE trees SET updated_at = now() WHERE id = $1`, treeUUID); err != nil {
		return TreeDetail{}, err
	}
	auditValue, _ := json.Marshal(map[string]any{"tree_id": treeID, "version_id": uuidString(versionID), "node_id": nodeID.String(), "display_name_ar": input.CanonicalName})
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, after_value)
		VALUES ($1, 'person_added', 'tree_node', $2, $3)
	`, ownerUUID, nodeID, auditValue); err != nil {
		return TreeDetail{}, err
	}
	if err := writeTreeChange(ctx, tx, treeUUID, &versionID, ownerUUID, "person_added", "tree_node", nodeID, nil, map[string]any{"display_name_ar": input.CanonicalName}, ""); err != nil {
		return TreeDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TreeDetail{}, err
	}
	return s.GetTree(ctx, treeID, ownerID)
}

func (s *Service) AddRelationship(ctx context.Context, treeID, ownerID string, input AddRelationshipInput) (TreeDetail, error) {
	if s == nil || s.Pool == nil {
		return TreeDetail{}, ErrDatabaseUnavailable
	}
	treeUUID, err := uuid.Parse(treeID)
	if err != nil {
		return TreeDetail{}, ErrNotFound
	}
	ownerUUID, err := uuid.Parse(ownerID)
	if err != nil {
		return TreeDetail{}, ErrForbidden
	}
	input, err = validateRelationshipInput(input)
	if err != nil {
		return TreeDetail{}, err
	}
	subjectUUID, err := uuid.Parse(input.SubjectNodeID)
	if err != nil {
		return TreeDetail{}, ErrValidation
	}
	objectUUID, err := uuid.Parse(input.ObjectNodeID)
	if err != nil {
		return TreeDetail{}, ErrValidation
	}
	if subjectUUID == objectUUID {
		return TreeDetail{}, ErrValidation
	}
	if _, err := s.treeSummary(ctx, treeUUID); err != nil {
		return TreeDetail{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TreeDetail{}, err
	}
	defer tx.Rollback(ctx)
	versionID, _, err := s.lockLatestDraft(ctx, tx, treeUUID)
	if err != nil {
		return TreeDetail{}, err
	}
	allowed, err := s.canEditDraftTx(ctx, tx, treeUUID, ownerUUID)
	if err != nil {
		return TreeDetail{}, err
	}
	if !allowed {
		return TreeDetail{}, ErrForbidden
	}
	var nodeCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM tree_nodes
		WHERE tree_version_id = $1 AND id IN ($2, $3)
	`, versionID, subjectUUID, objectUUID).Scan(&nodeCount); err != nil {
		return TreeDetail{}, err
	}
	if nodeCount != 2 {
		return TreeDetail{}, ErrValidation
	}
	var exists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM tree_relationships
			WHERE tree_version_id = $1
			  AND subject_node_id = $2
			  AND object_node_id = $3
			  AND predicate = $4
		)
	`, versionID, subjectUUID, objectUUID, input.Predicate).Scan(&exists); err != nil {
		return TreeDetail{}, err
	}
	if exists {
		return TreeDetail{}, ErrDuplicateRelationship
	}
	relationshipID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, relationshipID, versionID, subjectUUID, objectUUID, input.Predicate, input.Status, ownerUUID); err != nil {
		return TreeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE trees SET updated_at = now() WHERE id = $1`, treeUUID); err != nil {
		return TreeDetail{}, err
	}
	auditValue, _ := json.Marshal(map[string]any{"tree_id": treeID, "version_id": uuidString(versionID), "relationship_id": relationshipID.String(), "predicate": input.Predicate, "status": input.Status})
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, after_value)
		VALUES ($1, 'relationship_added', 'tree_relationship', $2, $3)
	`, ownerUUID, relationshipID, auditValue); err != nil {
		return TreeDetail{}, err
	}
	if err := writeTreeChange(ctx, tx, treeUUID, &versionID, ownerUUID, "relationship_added", "tree_relationship", relationshipID, nil, map[string]any{"predicate": input.Predicate, "status": input.Status}, ""); err != nil {
		return TreeDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TreeDetail{}, err
	}
	return s.GetTree(ctx, treeID, ownerID)
}

func (s *Service) UpdateRelationship(ctx context.Context, treeID, relationshipID, ownerID string, input UpdateRelationshipInput) (TreeDetail, error) {
	if s == nil || s.Pool == nil {
		return TreeDetail{}, ErrDatabaseUnavailable
	}
	treeUUID, err := uuid.Parse(treeID)
	if err != nil {
		return TreeDetail{}, ErrNotFound
	}
	relationshipUUID, err := uuid.Parse(relationshipID)
	if err != nil {
		return TreeDetail{}, ErrNotFound
	}
	ownerUUID, err := uuid.Parse(ownerID)
	if err != nil {
		return TreeDetail{}, ErrForbidden
	}
	input, err = validateUpdateRelationshipInput(input)
	if err != nil {
		return TreeDetail{}, err
	}
	expectedVersionUUID, err := uuid.Parse(input.ExpectedVersionID)
	if err != nil {
		return TreeDetail{}, ErrValidation
	}
	if _, err := s.treeSummary(ctx, treeUUID); err != nil {
		return TreeDetail{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TreeDetail{}, err
	}
	defer tx.Rollback(ctx)
	versionID, _, err := s.lockLatestDraft(ctx, tx, treeUUID)
	if err != nil {
		return TreeDetail{}, err
	}
	allowed, err := s.canEditDraftTx(ctx, tx, treeUUID, ownerUUID)
	if err != nil {
		return TreeDetail{}, err
	}
	if !allowed {
		return TreeDetail{}, ErrForbidden
	}
	if expectedVersionUUID.String() != uuidString(versionID) {
		return TreeDetail{}, ErrStaleVersion
	}
	var previousStatus string
	if err := tx.QueryRow(ctx, `
		SELECT status
		FROM tree_relationships
		WHERE tree_version_id = $1 AND id = $2
		FOR UPDATE
	`, versionID, relationshipUUID).Scan(&previousStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreeDetail{}, ErrNotFound
		}
		return TreeDetail{}, err
	}
	if previousStatus == input.Status {
		if err := tx.Commit(ctx); err != nil {
			return TreeDetail{}, err
		}
		return s.GetTree(ctx, treeID, ownerID)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tree_relationships
		SET status = $1
		WHERE tree_version_id = $2 AND id = $3
	`, input.Status, versionID, relationshipUUID); err != nil {
		return TreeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE trees SET updated_at = now() WHERE id = $1`, treeUUID); err != nil {
		return TreeDetail{}, err
	}
	auditValue, _ := json.Marshal(map[string]any{
		"tree_id":         treeID,
		"version_id":      uuidString(versionID),
		"relationship_id": relationshipUUID.String(),
		"before_status":   previousStatus,
		"after_status":    input.Status,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, after_value, reason_ar)
		VALUES ($1, 'relationship_changed', 'tree_relationship', $2, $3, $4)
	`, ownerUUID, relationshipUUID, auditValue, input.ReasonAR); err != nil {
		return TreeDetail{}, err
	}
	if err := writeTreeChange(ctx, tx, treeUUID, &versionID, ownerUUID, "relationship_changed", "tree_relationship", relationshipUUID, map[string]any{"status": previousStatus}, map[string]any{"status": input.Status}, input.ReasonAR); err != nil {
		return TreeDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TreeDetail{}, err
	}
	return s.GetTree(ctx, treeID, ownerID)
}

func (s *Service) ListPublicTrees(ctx context.Context) ([]TreeSummary, error) {
	if s == nil || s.Pool == nil {
		return nil, ErrDatabaseUnavailable
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT t.id, t.name_ar, t.description_ar, t.visibility, t.owner_id, t.updated_at,
		       tv.id, tv.version_number, tv.state,
		       (SELECT count(*) FROM tree_nodes tn WHERE tn.tree_version_id = tv.id),
		       (SELECT count(*) FROM tree_relationships tr WHERE tr.tree_version_id = tv.id),
		       (SELECT count(*) FROM tree_relationships tr WHERE tr.tree_version_id = tv.id AND tr.status = 'unresolved')
		FROM trees t
		JOIN LATERAL (
			SELECT id, version_number, state
			FROM tree_versions
			WHERE tree_id = t.id AND state = 'published'
			ORDER BY version_number DESC
			LIMIT 1
		) tv ON true
		WHERE t.visibility = 'public'
		ORDER BY t.updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTreeSummaries(rows)
}

func (s *Service) ListAccessibleTrees(ctx context.Context, viewerID string) ([]TreeSummary, error) {
	if s == nil || s.Pool == nil {
		return nil, ErrDatabaseUnavailable
	}
	if viewerID == "" {
		return s.ListPublicTrees(ctx)
	}
	if _, err := uuid.Parse(viewerID); err != nil {
		return nil, ErrForbidden
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT t.id, t.name_ar, t.description_ar, t.visibility, t.owner_id, t.updated_at,
		       tv.id, tv.version_number, tv.state,
		       (SELECT count(*) FROM tree_nodes tn WHERE tn.tree_version_id = tv.id),
		       (SELECT count(*) FROM tree_relationships tr WHERE tr.tree_version_id = tv.id),
		       (SELECT count(*) FROM tree_relationships tr WHERE tr.tree_version_id = tv.id AND tr.status = 'unresolved')
		FROM trees t
		JOIN LATERAL (
			SELECT id, version_number, state
			FROM tree_versions
			WHERE tree_id = t.id
			  AND (
				  state = 'published'
				  OR t.owner_id = $1
				  OR EXISTS (
					  SELECT 1
					  FROM tree_collaborators tc
					  WHERE tc.tree_id = t.id AND tc.user_id = $1
				  )
			  )
			ORDER BY version_number DESC
			LIMIT 1
		) tv ON true
		WHERE t.visibility = 'public'
		   OR t.owner_id = $1
		   OR EXISTS (
			   SELECT 1
			   FROM tree_collaborators tc
			   WHERE tc.tree_id = t.id AND tc.user_id = $1
		   )
		ORDER BY t.updated_at DESC
	`, viewerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTreeSummaries(rows)
}

func scanTreeSummaries(rows pgx.Rows) ([]TreeSummary, error) {
	items := make([]TreeSummary, 0)
	for rows.Next() {
		var item TreeSummary
		var description pgtype.Text
		var ownerID pgtype.UUID
		var updatedAt pgtype.Timestamptz
		var versionID pgtype.UUID
		if err := rows.Scan(&item.ID, &item.Name, &description, &item.Visibility, &ownerID, &updatedAt, &versionID, &item.LatestVersionNumber, &item.LatestState, &item.People, &item.Relationships, &item.Unresolved); err != nil {
			return nil, err
		}
		item.Description = textValue(description)
		item.OwnerID = uuidString(ownerID)
		item.UpdatedAt = timeValue(updatedAt)
		item.LatestVersionID = uuidString(versionID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) GetTree(ctx context.Context, treeID, viewerID string) (TreeDetail, error) {
	return s.getTree(ctx, treeID, viewerID, "")
}

func (s *Service) GetTreeVersion(ctx context.Context, treeID, versionID, viewerID string) (TreeDetail, error) {
	return s.getTree(ctx, treeID, viewerID, versionID)
}

func (s *Service) getTree(ctx context.Context, treeID, viewerID, requestedVersionID string) (TreeDetail, error) {
	if s == nil || s.Pool == nil {
		return TreeDetail{}, ErrDatabaseUnavailable
	}
	id, err := uuid.Parse(treeID)
	if err != nil {
		return TreeDetail{}, ErrNotFound
	}
	tree, err := s.treeSummary(ctx, id)
	if err != nil {
		return TreeDetail{}, err
	}
	allowed, err := s.canView(ctx, tree, viewerID)
	if err != nil {
		return TreeDetail{}, err
	}
	if !allowed {
		return TreeDetail{}, ErrForbidden
	}
	draftVisible, err := s.canViewDraft(ctx, tree, viewerID)
	if err != nil {
		return TreeDetail{}, err
	}

	var selected TreeVersionView
	if requestedVersionID == "" {
		selected, err = s.latestVersion(ctx, id, !draftVisible)
	} else {
		versionUUID, parseErr := uuid.Parse(requestedVersionID)
		if parseErr != nil {
			return TreeDetail{}, ErrNotFound
		}
		selected, err = s.version(ctx, id, versionUUID)
		if err == nil && !canViewVersionState(selected.State, draftVisible) {
			return TreeDetail{}, ErrNotFound
		}
	}
	if err != nil {
		return TreeDetail{}, err
	}
	latest, err := s.latestVersion(ctx, id, !draftVisible)
	if err != nil {
		return TreeDetail{}, err
	}
	versions, err := s.versions(ctx, id, !draftVisible)
	if err != nil {
		return TreeDetail{}, err
	}
	nodes, err := s.nodes(ctx, selected.ID)
	if err != nil {
		return TreeDetail{}, err
	}
	relationships, err := s.relationships(ctx, selected.ID)
	if err != nil {
		return TreeDetail{}, err
	}
	tree.LatestVersionID = latest.ID
	tree.LatestVersionNumber = latest.Number
	tree.LatestState = latest.State
	tree.People = len(nodes)
	tree.Relationships = len(relationships)
	for _, relationship := range relationships {
		if relationship.Status == "unresolved" {
			tree.Unresolved++
		}
	}
	permissionLevel, err := s.collaboratorLevel(ctx, id, viewerID)
	if err != nil {
		return TreeDetail{}, err
	}
	permissions := permissionsForAccess(tree, viewerID, selected, permissionLevel)
	return TreeDetail{
		Tree:            tree,
		SelectedVersion: selected,
		Permissions:     permissions,
		Versions:        versions,
		Nodes:           nodes,
		Relationships:   relationships,
	}, nil
}

func (s *Service) ListVersions(ctx context.Context, treeID, viewerID string) ([]TreeVersionView, error) {
	if s == nil || s.Pool == nil {
		return nil, ErrDatabaseUnavailable
	}
	id, err := uuid.Parse(treeID)
	if err != nil {
		return nil, ErrNotFound
	}
	tree, err := s.treeSummary(ctx, id)
	if err != nil {
		return nil, err
	}
	allowed, err := s.canView(ctx, tree, viewerID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	draftVisible, err := s.canViewDraft(ctx, tree, viewerID)
	if err != nil {
		return nil, err
	}
	return s.versions(ctx, id, !draftVisible)
}

func (s *Service) PublishLatestDraft(ctx context.Context, treeID, ownerID, note string) (TreeDetail, error) {
	if s == nil || s.Pool == nil {
		return TreeDetail{}, ErrDatabaseUnavailable
	}
	treeUUID, err := uuid.Parse(treeID)
	if err != nil {
		return TreeDetail{}, ErrNotFound
	}
	ownerUUID, err := uuid.Parse(ownerID)
	if err != nil {
		return TreeDetail{}, ErrForbidden
	}
	tree, err := s.treeSummary(ctx, treeUUID)
	if err != nil {
		return TreeDetail{}, err
	}
	if tree.OwnerID != ownerID {
		return TreeDetail{}, ErrForbidden
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TreeDetail{}, err
	}
	defer tx.Rollback(ctx)
	versionID, versionNumber, err := s.lockLatestDraft(ctx, tx, treeUUID)
	if err != nil {
		return TreeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tree_versions
		SET state = 'published', publication_note_ar = NULLIF($2, ''), published_by = $3, published_at = now()
		WHERE id = $1
	`, versionID, strings.TrimSpace(note), ownerUUID); err != nil {
		return TreeDetail{}, err
	}
	nextVersionID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO tree_versions (id, tree_id, version_number, state, created_by)
		VALUES ($1, $2, $3, 'draft', $4)
	`, nextVersionID, treeUUID, versionNumber+1, ownerUUID); err != nil {
		return TreeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order)
		SELECT gen_random_uuid(), $1, tn.person_id, tn.display_name_ar, tn.sort_order
		FROM tree_nodes tn
		WHERE tn.tree_version_id = $2
	`, nextVersionID, versionID); err != nil {
		return TreeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, source_id, created_by)
		SELECT gen_random_uuid(), $1, subject_node.id, object_node.id, tr.predicate, tr.status, tr.source_id, $3
		FROM tree_relationships tr
		JOIN tree_nodes old_subject ON old_subject.id = tr.subject_node_id
		JOIN tree_nodes old_object ON old_object.id = tr.object_node_id
		JOIN tree_nodes subject_node ON subject_node.tree_version_id = $1 AND subject_node.person_id = old_subject.person_id
		JOIN tree_nodes object_node ON object_node.tree_version_id = $1 AND object_node.person_id = old_object.person_id
		WHERE tr.tree_version_id = $2
	`, nextVersionID, versionID, ownerUUID); err != nil {
		return TreeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE trees SET updated_at = now() WHERE id = $1`, treeUUID); err != nil {
		return TreeDetail{}, err
	}
	auditValue, _ := json.Marshal(map[string]any{"published_version": versionNumber, "next_version": versionNumber + 1})
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, after_value, reason_ar)
		VALUES ($1, 'tree_version_published', 'tree', $2, $3, NULLIF($4, ''))
	`, ownerUUID, treeUUID, auditValue, strings.TrimSpace(note)); err != nil {
		return TreeDetail{}, err
	}
	if err := writeTreeChange(ctx, tx, treeUUID, &versionID, ownerUUID, "tree_version_published", "tree", treeUUID, nil, map[string]any{"published_version": versionNumber, "next_version": versionNumber + 1}, note); err != nil {
		return TreeDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TreeDetail{}, err
	}
	return s.GetTree(ctx, treeID, ownerID)
}

func (s *Service) canEditDraftTx(ctx context.Context, tx pgx.Tx, treeID, actorID uuid.UUID) (bool, error) {
	var ownerID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT owner_id FROM trees WHERE id = $1`, treeID).Scan(&ownerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, ErrNotFound
		}
		return false, err
	}
	if ownerID == actorID {
		return true, nil
	}
	var permissionLevel string
	if err := tx.QueryRow(ctx, `
		SELECT permission_level
		FROM tree_collaborators
		WHERE tree_id = $1 AND user_id = $2
	`, treeID, actorID).Scan(&permissionLevel); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return permissionLevel == "edit", nil
}

func (s *Service) lockLatestDraft(ctx context.Context, tx pgx.Tx, treeID uuid.UUID) (pgtype.UUID, int, error) {
	var versionID pgtype.UUID
	var versionNumber int
	if err := tx.QueryRow(ctx, `
		SELECT id, version_number
		FROM tree_versions
		WHERE tree_id = $1 AND state = 'draft'
		ORDER BY version_number DESC
		LIMIT 1
		FOR UPDATE
	`, treeID).Scan(&versionID, &versionNumber); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, 0, ErrNoDraft
		}
		return pgtype.UUID{}, 0, err
	}
	return versionID, versionNumber, nil
}

func (s *Service) treeSummary(ctx context.Context, id uuid.UUID) (TreeSummary, error) {
	return s.treeSummaryWithVisibility(ctx, id, false)
}

func (s *Service) treeSummaryWithVisibility(ctx context.Context, id uuid.UUID, publicOnly bool) (TreeSummary, error) {
	var item TreeSummary
	var description pgtype.Text
	var ownerID pgtype.UUID
	var updatedAt pgtype.Timestamptz
	query := `
		SELECT id, name_ar, description_ar, visibility, owner_id, updated_at
		FROM trees
		WHERE id = $1
	`
	if publicOnly {
		query += ` AND visibility = 'public'`
	}
	if err := s.Pool.QueryRow(ctx, query, id).Scan(&item.ID, &item.Name, &description, &item.Visibility, &ownerID, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreeSummary{}, ErrNotFound
		}
		return TreeSummary{}, err
	}
	item.Description = textValue(description)
	item.OwnerID = uuidString(ownerID)
	item.UpdatedAt = timeValue(updatedAt)
	return item, nil
}

func canViewVersionState(state string, draftVisible bool) bool {
	return state == "published" || draftVisible
}

func permissionsFor(tree TreeSummary, viewerID string, version TreeVersionView) TreePermissions {
	return permissionsForAccess(tree, viewerID, version, "")
}

func permissionsForAccess(tree TreeSummary, viewerID string, version TreeVersionView, permissionLevel string) TreePermissions {
	isOwner := viewerID != "" && tree.OwnerID == viewerID
	canEdit := version.State == "draft" && (isOwner || permissionLevel == "edit")
	if isOwner {
		permissionLevel = "owner"
	}
	return TreePermissions{
		CanEdit:                canEdit,
		CanPublish:             isOwner && version.State == "draft",
		CanManageCollaborators: isOwner,
		PermissionLevel:        permissionLevel,
	}
}

func (s *Service) CanViewDraft(ctx context.Context, treeID, viewerID string) (bool, error) {
	if s == nil || s.Pool == nil {
		return false, ErrDatabaseUnavailable
	}
	if viewerID == "" {
		return false, nil
	}
	if _, err := uuid.Parse(viewerID); err != nil {
		return false, ErrForbidden
	}
	id, err := uuid.Parse(treeID)
	if err != nil {
		return false, ErrNotFound
	}
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM trees t
			LEFT JOIN tree_collaborators tc ON tc.tree_id = t.id AND tc.user_id = $2
			WHERE t.id = $1 AND (t.owner_id = $2 OR tc.user_id IS NOT NULL)
		)
	`, id, viewerID).Scan(&allowed); err != nil {
		return false, err
	}
	return allowed, nil
}

func (s *Service) CanManageCollaborators(ctx context.Context, treeID, actorID string) (bool, error) {
	if s == nil || s.Pool == nil {
		return false, ErrDatabaseUnavailable
	}
	id, err := uuid.Parse(treeID)
	if err != nil {
		return false, ErrNotFound
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return false, ErrForbidden
	}
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM trees WHERE id = $1 AND owner_id = $2)
	`, id, actorUUID).Scan(&allowed); err != nil {
		return false, err
	}
	return allowed, nil
}

func (s *Service) collaboratorLevel(ctx context.Context, treeID uuid.UUID, viewerID string) (string, error) {
	if viewerID == "" {
		return "", nil
	}
	if _, err := uuid.Parse(viewerID); err != nil {
		return "", ErrForbidden
	}
	var level string
	if err := s.Pool.QueryRow(ctx, `
		SELECT permission_level
		FROM tree_collaborators
		WHERE tree_id = $1 AND user_id = $2
	`, treeID, viewerID).Scan(&level); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return level, nil
}

func (s *Service) canViewDraft(ctx context.Context, tree TreeSummary, viewerID string) (bool, error) {
	if viewerID == "" {
		return false, nil
	}
	if tree.OwnerID == viewerID {
		return true, nil
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM tree_collaborators WHERE tree_id = $1 AND user_id = $2`, tree.ID, viewerID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Service) canView(ctx context.Context, tree TreeSummary, viewerID string) (bool, error) {
	if tree.Visibility == "public" {
		return true, nil
	}
	if viewerID == "" {
		return false, nil
	}
	if tree.OwnerID == viewerID {
		return true, nil
	}
	var count int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM tree_collaborators WHERE tree_id = $1 AND user_id = $2`, tree.ID, viewerID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Service) version(ctx context.Context, treeID, versionID uuid.UUID) (TreeVersionView, error) {
	return s.scanVersion(s.Pool.QueryRow(ctx, `
		SELECT id, version_number, state, publication_note_ar, published_at, created_at
		FROM tree_versions
		WHERE tree_id = $1 AND id = $2
	`, treeID, versionID))
}

func (s *Service) latestVersion(ctx context.Context, treeID uuid.UUID, publishedOnly bool) (TreeVersionView, error) {
	query := `SELECT id, version_number, state, publication_note_ar, published_at, created_at FROM tree_versions WHERE tree_id = $1`
	if publishedOnly {
		query += ` AND state = 'published'`
	}
	query += ` ORDER BY version_number DESC LIMIT 1`
	return s.scanVersion(s.Pool.QueryRow(ctx, query, treeID))
}

func (s *Service) versions(ctx context.Context, treeID uuid.UUID, publishedOnly bool) ([]TreeVersionView, error) {
	query := `SELECT id, version_number, state, publication_note_ar, published_at, created_at FROM tree_versions WHERE tree_id = $1`
	if publishedOnly {
		query += ` AND state = 'published'`
	}
	query += ` ORDER BY version_number DESC`
	rows, err := s.Pool.Query(ctx, query, treeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TreeVersionView, 0)
	for rows.Next() {
		item, err := scanVersionRows(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) nodes(ctx context.Context, versionID string) ([]TreeNodeView, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT tn.id, tn.person_id, tn.display_name_ar, tn.sort_order,
		       p.birth_date_from, p.birth_date_to, p.death_date_from, p.death_date_to, p.identity_status,
		       (SELECT count(DISTINCT tr.source_id)
		        FROM tree_relationships tr
		        WHERE tr.tree_version_id = tn.tree_version_id
		          AND (tr.subject_node_id = tn.id OR tr.object_node_id = tn.id)) AS source_count
		FROM tree_nodes tn
		JOIN people p ON p.id = tn.person_id
		WHERE tn.tree_version_id = $1
		ORDER BY tn.sort_order, tn.created_at
	`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TreeNodeView, 0)
	for rows.Next() {
		var item TreeNodeView
		var nodeID, personID pgtype.UUID
		var birthFrom, birthTo, deathFrom, deathTo pgtype.Date
		var identityStatus string
		if err := rows.Scan(&nodeID, &personID, &item.DisplayName, &item.SortOrder, &birthFrom, &birthTo, &deathFrom, &deathTo, &identityStatus, &item.SourceCount); err != nil {
			return nil, err
		}
		item.ID = uuidString(nodeID)
		item.PersonID = uuidString(personID)
		item.Years = formatYears(birthFrom, birthTo, deathFrom, deathTo)
		item.Role = "شخص في الشجرة"
		item.Tone = "interpretation"
		if identityStatus == "disputed" {
			item.Tone = "disputed"
			item.Note = "هوية مرشحة للمراجعة في سجل الهوية."
		} else {
			item.Note = "مُدرج ضمن تفسير هذه النسخة."
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) relationships(ctx context.Context, versionID string) ([]TreeRelationshipView, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, subject_node_id, object_node_id, predicate, status
		FROM tree_relationships
		WHERE tree_version_id = $1
		ORDER BY created_at
	`, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]TreeRelationshipView, 0)
	for rows.Next() {
		var item TreeRelationshipView
		var id, subjectID, objectID pgtype.UUID
		if err := rows.Scan(&id, &subjectID, &objectID, &item.Predicate, &item.Status); err != nil {
			return nil, err
		}
		item.ID = uuidString(id)
		item.SubjectNodeID = uuidString(subjectID)
		item.ObjectNodeID = uuidString(objectID)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) scanVersion(row pgx.Row) (TreeVersionView, error) {
	var item TreeVersionView
	var id pgtype.UUID
	var note pgtype.Text
	var publishedAt pgtype.Timestamptz
	var createdAt pgtype.Timestamptz
	if err := row.Scan(&id, &item.Number, &item.State, &note, &publishedAt, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreeVersionView{}, ErrNotFound
		}
		return TreeVersionView{}, err
	}
	item.ID = uuidString(id)
	item.PublicationNote = textValue(note)
	if publishedAt.Valid {
		value := publishedAt.Time
		item.PublishedAt = &value
	}
	item.CreatedAt = timeValue(createdAt)
	return item, nil
}

func scanVersionRows(rows pgx.Rows) (TreeVersionView, error) {
	var item TreeVersionView
	var id pgtype.UUID
	var note pgtype.Text
	var publishedAt pgtype.Timestamptz
	var createdAt pgtype.Timestamptz
	if err := rows.Scan(&id, &item.Number, &item.State, &note, &publishedAt, &createdAt); err != nil {
		return TreeVersionView{}, err
	}
	item.ID = uuidString(id)
	item.PublicationNote = textValue(note)
	if publishedAt.Valid {
		value := publishedAt.Time
		item.PublishedAt = &value
	}
	item.CreatedAt = timeValue(createdAt)
	return item, nil
}

func writeTreeChange(ctx context.Context, tx pgx.Tx, treeID uuid.UUID, versionID any, actorID uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any, reason string) error {
	beforeValue := marshalTreeValue(before)
	afterValue := marshalTreeValue(after)
	versionArg := versionIDValue(versionID)
	var reasonArg any
	if strings.TrimSpace(reason) != "" {
		reasonArg = strings.TrimSpace(reason)
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO tree_change_log
			(tree_id, tree_version_id, actor_id, action, entity_type, entity_id, before_value, after_value, reason_ar)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, treeID, versionArg, actorID, action, entityType, entityID, beforeValue, afterValue, reasonArg)
	return err
}

func versionIDValue(value any) any {
	switch current := value.(type) {
	case *uuid.UUID:
		if current != nil {
			return *current
		}
	case uuid.UUID:
		return current
	case *pgtype.UUID:
		if current != nil && current.Valid {
			return uuid.UUID(current.Bytes)
		}
	case pgtype.UUID:
		if current.Valid {
			return uuid.UUID(current.Bytes)
		}
	}
	return nil
}

func marshalTreeValue(value any) any {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return encoded
}

func validateCreateInput(input CreateTreeInput) (CreateTreeInput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Visibility = strings.TrimSpace(input.Visibility)
	if input.Visibility == "" {
		input.Visibility = "private"
	}
	if input.Name == "" || len([]rune(input.Name)) > 200 || len([]rune(input.Description)) > 2000 {
		return CreateTreeInput{}, ErrValidation
	}
	if input.Visibility != "private" && input.Visibility != "unlisted" && input.Visibility != "public" {
		return CreateTreeInput{}, ErrValidation
	}
	if len(input.People) > 100 {
		return CreateTreeInput{}, ErrValidation
	}
	for index := range input.People {
		person, err := validatePersonInput(input.People[index])
		if err != nil {
			return CreateTreeInput{}, err
		}
		input.People[index] = person
	}
	return input, nil
}

func validatePersonInput(input PersonInput) (PersonInput, error) {
	input.CanonicalName = strings.TrimSpace(input.CanonicalName)
	input.Gender = strings.TrimSpace(input.Gender)
	input.BirthDateFrom = strings.TrimSpace(input.BirthDateFrom)
	input.BirthDateTo = strings.TrimSpace(input.BirthDateTo)
	input.DeathDateFrom = strings.TrimSpace(input.DeathDateFrom)
	input.DeathDateTo = strings.TrimSpace(input.DeathDateTo)
	if input.Gender == "" {
		input.Gender = "unknown"
	}
	if input.CanonicalName == "" || len([]rune(input.CanonicalName)) > 200 {
		return PersonInput{}, ErrValidation
	}
	if input.Gender != "male" && input.Gender != "female" && input.Gender != "unknown" {
		return PersonInput{}, ErrValidation
	}
	return input, nil
}

func validateRelationshipInput(input AddRelationshipInput) (AddRelationshipInput, error) {
	input.SubjectNodeID = strings.TrimSpace(input.SubjectNodeID)
	input.ObjectNodeID = strings.TrimSpace(input.ObjectNodeID)
	input.Predicate = strings.TrimSpace(input.Predicate)
	input.Status = strings.TrimSpace(input.Status)
	if input.Status == "" {
		input.Status = "interpreted"
	}
	if input.SubjectNodeID == "" || input.ObjectNodeID == "" {
		return AddRelationshipInput{}, ErrValidation
	}
	if input.Predicate != "parent_of" && input.Predicate != "spouse_of" && input.Predicate != "sibling_of" {
		return AddRelationshipInput{}, ErrValidation
	}
	if input.Status != "interpreted" && input.Status != "disputed" && input.Status != "unresolved" {
		return AddRelationshipInput{}, ErrValidation
	}
	return input, nil
}

func validateUpdateRelationshipInput(input UpdateRelationshipInput) (UpdateRelationshipInput, error) {
	input.Status = strings.TrimSpace(input.Status)
	input.ExpectedVersionID = strings.TrimSpace(input.ExpectedVersionID)
	input.ReasonAR = strings.TrimSpace(input.ReasonAR)
	if input.Status != "interpreted" && input.Status != "disputed" && input.Status != "unresolved" {
		return UpdateRelationshipInput{}, ErrValidation
	}
	if input.ExpectedVersionID == "" || input.ReasonAR == "" || len([]rune(input.ReasonAR)) > 1000 {
		return UpdateRelationshipInput{}, ErrValidation
	}
	return input, nil
}

func parsePersonDates(input PersonInput) (pgtype.Date, pgtype.Date, pgtype.Date, pgtype.Date, error) {
	birthFrom, err := parseDate(input.BirthDateFrom)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, err
	}
	birthTo, err := parseDate(input.BirthDateTo)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, err
	}
	deathFrom, err := parseDate(input.DeathDateFrom)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, err
	}
	deathTo, err := parseDate(input.DeathDateTo)
	if err != nil {
		return pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, err
	}
	if birthFrom.Valid && birthTo.Valid && birthFrom.Time.After(birthTo.Time) {
		return pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, ErrValidation
	}
	if deathFrom.Valid && deathTo.Valid && deathFrom.Time.After(deathTo.Time) {
		return pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, pgtype.Date{}, ErrValidation
	}
	return birthFrom, birthTo, deathFrom, deathTo, nil
}

func parseDate(value string) (pgtype.Date, error) {
	if strings.TrimSpace(value) == "" {
		return pgtype.Date{}, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return pgtype.Date{}, ErrValidation
	}
	return pgtype.Date{Time: parsed, Valid: true}, nil
}

func formatYears(birthFrom, birthTo, deathFrom, deathTo pgtype.Date) string {
	start := ""
	if birthFrom.Valid {
		start = birthFrom.Time.Format("2006")
	} else if deathFrom.Valid {
		start = deathFrom.Time.Format("2006")
	}
	end := ""
	if deathTo.Valid {
		end = deathTo.Time.Format("2006")
	} else if birthTo.Valid {
		end = birthTo.Time.Format("2006")
	} else if deathFrom.Valid {
		end = deathFrom.Time.Format("2006")
	}
	if start == "" && end == "" {
		return "غير محددة"
	}
	if start == "" {
		return "حتى " + end
	}
	if end == "" || start == end {
		return start
	}
	return start + " — " + end
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func timeValue(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
