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
	ErrDatabaseUnavailable = errors.New("tree database is unavailable")
	ErrNotFound            = errors.New("tree not found")
	ErrForbidden           = errors.New("tree access is forbidden")
	ErrNoDraft             = errors.New("tree has no draft version")
	ErrValidation          = errors.New("tree input is invalid")
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

type TreeDetail struct {
	Tree          TreeSummary            `json:"tree"`
	Versions      []TreeVersionView      `json:"versions"`
	Nodes         []TreeNodeView         `json:"nodes"`
	Relationships []TreeRelationshipView `json:"relationships"`
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
		birthFrom, err := parseDate(person.BirthDateFrom)
		if err != nil {
			return TreeDetail{}, err
		}
		birthTo, err := parseDate(person.BirthDateTo)
		if err != nil {
			return TreeDetail{}, err
		}
		deathFrom, err := parseDate(person.DeathDateFrom)
		if err != nil {
			return TreeDetail{}, err
		}
		deathTo, err := parseDate(person.DeathDateTo)
		if err != nil {
			return TreeDetail{}, err
		}
		personID := uuid.New()
		nodeID := uuid.New()
		if birthFrom.Valid && birthTo.Valid && birthFrom.Time.After(birthTo.Time) {
			return TreeDetail{}, ErrValidation
		}
		if deathFrom.Valid && deathTo.Valid && deathFrom.Time.After(deathTo.Time) {
			return TreeDetail{}, ErrValidation
		}
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
	if err := tx.Commit(ctx); err != nil {
		return TreeDetail{}, err
	}
	return s.GetTree(ctx, treeID.String(), ownerID)
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
	version, err := s.latestVersion(ctx, id, publishedOnlyForViewer(tree, viewerID))
	if err != nil {
		return TreeDetail{}, err
	}
	versions, err := s.versions(ctx, id, publishedOnlyForViewer(tree, viewerID))
	if err != nil {
		return TreeDetail{}, err
	}
	nodes, err := s.nodes(ctx, version.ID)
	if err != nil {
		return TreeDetail{}, err
	}
	relationships, err := s.relationships(ctx, version.ID)
	if err != nil {
		return TreeDetail{}, err
	}
	tree.LatestVersionID = version.ID
	tree.LatestVersionNumber = version.Number
	tree.LatestState = version.State
	tree.People = len(nodes)
	tree.Relationships = len(relationships)
	for _, relationship := range relationships {
		if relationship.Status == "unresolved" {
			tree.Unresolved++
		}
	}
	return TreeDetail{Tree: tree, Versions: versions, Nodes: nodes, Relationships: relationships}, nil
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
	return s.versions(ctx, id, publishedOnlyForViewer(tree, viewerID))
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
	var versionID pgtype.UUID
	var versionNumber int
	if err := tx.QueryRow(ctx, `
		SELECT id, version_number
		FROM tree_versions
		WHERE tree_id = $1 AND state = 'draft'
		ORDER BY version_number DESC
		LIMIT 1
		FOR UPDATE
	`, treeUUID).Scan(&versionID, &versionNumber); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreeDetail{}, ErrNoDraft
		}
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
	if err := tx.Commit(ctx); err != nil {
		return TreeDetail{}, err
	}
	return s.GetTree(ctx, treeID, ownerID)
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

func publishedOnlyForViewer(tree TreeSummary, viewerID string) bool {
	return tree.Visibility == "public" && tree.OwnerID != viewerID
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
		input.People[index].CanonicalName = strings.TrimSpace(input.People[index].CanonicalName)
		input.People[index].Gender = strings.TrimSpace(input.People[index].Gender)
		input.People[index].BirthDateFrom = strings.TrimSpace(input.People[index].BirthDateFrom)
		input.People[index].BirthDateTo = strings.TrimSpace(input.People[index].BirthDateTo)
		input.People[index].DeathDateFrom = strings.TrimSpace(input.People[index].DeathDateFrom)
		input.People[index].DeathDateTo = strings.TrimSpace(input.People[index].DeathDateTo)
		if input.People[index].Gender == "" {
			input.People[index].Gender = "unknown"
		}
		if input.People[index].CanonicalName == "" || len([]rune(input.People[index].CanonicalName)) > 200 {
			return CreateTreeInput{}, ErrValidation
		}
		if input.People[index].Gender != "male" && input.People[index].Gender != "female" && input.People[index].Gender != "unknown" {
			return CreateTreeInput{}, ErrValidation
		}
	}
	return input, nil
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
