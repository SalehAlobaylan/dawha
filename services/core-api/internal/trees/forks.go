package trees

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrForkSourceNotPublished = errors.New("fork source version is not published")
	ErrForkConflict           = errors.New("fork already exists")
	ErrInvalidDiff            = errors.New("tree diff input is invalid")
)

type ForkTreeInput struct {
	VersionID   string `json:"version_id"`
	Name        string `json:"name_ar"`
	Description string `json:"description_ar"`
	Visibility  string `json:"visibility"`
}

type TreeDiffEndpoint struct {
	TreeID        string `json:"treeId"`
	TreeName      string `json:"treeName"`
	VersionID     string `json:"versionId"`
	VersionNumber int    `json:"versionNumber"`
}

type TreeDiffPerson struct {
	PersonID    string `json:"personId"`
	DisplayName string `json:"displayName"`
	Years       string `json:"years"`
}

type TreeDiffDateChange struct {
	PersonID    string `json:"personId"`
	DisplayName string `json:"displayName"`
	BeforeYears string `json:"beforeYears"`
	AfterYears  string `json:"afterYears"`
}

type TreeDiffRelationship struct {
	Subject   string `json:"subject"`
	Object    string `json:"object"`
	Predicate string `json:"predicate"`
	Status    string `json:"status"`
	SourceID  string `json:"sourceId,omitempty"`
}

type TreeDiffRelationshipChange struct {
	Subject        string `json:"subject"`
	Object         string `json:"object"`
	Predicate      string `json:"predicate"`
	BeforeStatus   string `json:"beforeStatus"`
	AfterStatus    string `json:"afterStatus"`
	BeforeSourceID string `json:"beforeSourceId,omitempty"`
	AfterSourceID  string `json:"afterSourceId,omitempty"`
}

type TreeDiffSourceChange struct {
	SourceID          string `json:"sourceId"`
	RelationshipLabel string `json:"relationshipLabel"`
}

type TreeDiff struct {
	From                 TreeDiffEndpoint             `json:"from"`
	To                   TreeDiffEndpoint             `json:"to"`
	PeopleAdded          []TreeDiffPerson             `json:"peopleAdded"`
	PeopleRemoved        []TreeDiffPerson             `json:"peopleRemoved"`
	DateChanges          []TreeDiffDateChange         `json:"dateChanges"`
	RelationshipsAdded   []TreeDiffRelationship       `json:"relationshipsAdded"`
	RelationshipsRemoved []TreeDiffRelationship       `json:"relationshipsRemoved"`
	RelationshipChanges  []TreeDiffRelationshipChange `json:"relationshipChanges"`
	SourcesAdded         []TreeDiffSourceChange       `json:"sourcesAdded"`
	SourcesRemoved       []TreeDiffSourceChange       `json:"sourcesRemoved"`
	AffectedDescendants  int                          `json:"affectedDescendants"`
}

type diffNode struct {
	nodeID      string
	personID    string
	displayName string
	years       string
	dateKey     string
}

type diffRelationship struct {
	id            string
	subjectNodeID string
	objectNodeID  string
	predicate     string
	status        string
	sourceID      string
}

type forkNodeSnapshot struct {
	id          uuid.UUID
	personID    uuid.UUID
	displayName string
	sortOrder   int
}

type forkRelationshipSnapshot struct {
	subjectID uuid.UUID
	objectID  uuid.UUID
	predicate string
	status    string
	sourceID  *uuid.UUID
}

type diffNodePair struct {
	from *diffNode
	to   *diffNode
}

func (s *Service) ForkPublishedVersion(ctx context.Context, treeID, actorID string, input ForkTreeInput) (TreeDetail, error) {
	if s == nil || s.Pool == nil {
		return TreeDetail{}, ErrDatabaseUnavailable
	}
	treeUUID, err := uuid.Parse(treeID)
	if err != nil {
		return TreeDetail{}, ErrNotFound
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		return TreeDetail{}, ErrForbidden
	}
	input, err = validateForkInput(input)
	if err != nil {
		return TreeDetail{}, err
	}
	sourceVersionUUID, err := uuid.Parse(input.VersionID)
	if err != nil {
		return TreeDetail{}, ErrNotFound
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return TreeDetail{}, err
	}
	defer tx.Rollback(ctx)

	var sourceName, sourceVisibility string
	var sourceOwnerID uuid.UUID
	if err := tx.QueryRow(ctx, `
		SELECT name_ar, visibility, owner_id
		FROM trees
		WHERE id = $1
		FOR SHARE
	`, treeUUID).Scan(&sourceName, &sourceVisibility, &sourceOwnerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreeDetail{}, ErrNotFound
		}
		return TreeDetail{}, err
	}
	var sourceState string
	if err := tx.QueryRow(ctx, `
		SELECT state
		FROM tree_versions
		WHERE tree_id = $1 AND id = $2
		FOR SHARE
	`, treeUUID, sourceVersionUUID).Scan(&sourceState); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TreeDetail{}, ErrNotFound
		}
		return TreeDetail{}, err
	}
	if sourceState != "published" {
		return TreeDetail{}, ErrForkSourceNotPublished
	}
	if sourceVisibility != "public" && sourceOwnerID != actorUUID {
		var member bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM tree_collaborators
				WHERE tree_id = $1 AND user_id = $2
			)
		`, treeUUID, actorUUID).Scan(&member); err != nil {
			return TreeDetail{}, err
		}
		if !member {
			return TreeDetail{}, ErrForbidden
		}
	}
	var existingFork bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM tree_forks
			WHERE parent_tree_id = $1 AND parent_version_id = $2 AND forked_by = $3
		)
	`, treeUUID, sourceVersionUUID, actorUUID).Scan(&existingFork); err != nil {
		return TreeDetail{}, err
	}
	if existingFork {
		return TreeDetail{}, ErrForkConflict
	}

	newTreeID := uuid.New()
	newVersionID := uuid.New()
	newName := strings.TrimSpace(input.Name)
	if newName == "" {
		newName = sourceName + " — نسخة مستقلة"
	}
	newDescription := strings.TrimSpace(input.Description)
	if newDescription == "" {
		newDescription = "تفريع مستقل من تفسير منشور."
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO trees
			(id, name_ar, description_ar, visibility, owner_id, parent_tree_id, parent_version_id, forked_by, forked_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, now())
	`, newTreeID, newName, newDescription, input.Visibility, actorUUID, treeUUID, sourceVersionUUID, actorUUID); err != nil {
		return TreeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO tree_versions (id, tree_id, version_number, state, created_by)
		VALUES ($1, $2, 1, 'draft', $3)
	`, newVersionID, newTreeID, actorUUID); err != nil {
		return TreeDetail{}, err
	}

	nodeMap := make(map[string]uuid.UUID)
	sourceNodes := make([]forkNodeSnapshot, 0)
	rows, err := tx.Query(ctx, `
		SELECT id, person_id, display_name_ar, sort_order
		FROM tree_nodes
		WHERE tree_version_id = $1
		ORDER BY sort_order, created_at
	`, sourceVersionUUID)
	if err != nil {
		return TreeDetail{}, err
	}
	for rows.Next() {
		var oldNodeID, personID pgtype.UUID
		var displayName string
		var sortOrder int32
		if err := rows.Scan(&oldNodeID, &personID, &displayName, &sortOrder); err != nil {
			rows.Close()
			return TreeDetail{}, err
		}
		sourceNodes = append(sourceNodes, forkNodeSnapshot{id: uuid.UUID(oldNodeID.Bytes), personID: uuid.UUID(personID.Bytes), displayName: displayName, sortOrder: int(sortOrder)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return TreeDetail{}, err
	}
	rows.Close()
	for _, sourceNode := range sourceNodes {
		newNodeID := uuid.New()
		if _, err := tx.Exec(ctx, `
			INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order)
			VALUES ($1, $2, $3, $4, $5)
		`, newNodeID, newVersionID, sourceNode.personID, sourceNode.displayName, sourceNode.sortOrder); err != nil {
			return TreeDetail{}, err
		}
		nodeMap[sourceNode.id.String()] = newNodeID
	}

	sourceRelationships := make([]forkRelationshipSnapshot, 0)
	rows, err = tx.Query(ctx, `
		SELECT id, subject_node_id, object_node_id, predicate, status, source_id
		FROM tree_relationships
		WHERE tree_version_id = $1
		ORDER BY created_at
	`, sourceVersionUUID)
	if err != nil {
		return TreeDetail{}, err
	}
	for rows.Next() {
		var relationshipID, subjectID, objectID, sourceID pgtype.UUID
		var predicate, status string
		if err := rows.Scan(&relationshipID, &subjectID, &objectID, &predicate, &status, &sourceID); err != nil {
			rows.Close()
			return TreeDetail{}, err
		}
		var source *uuid.UUID
		if sourceID.Valid {
			value := uuid.UUID(sourceID.Bytes)
			source = &value
		}
		sourceRelationships = append(sourceRelationships, forkRelationshipSnapshot{subjectID: uuid.UUID(subjectID.Bytes), objectID: uuid.UUID(objectID.Bytes), predicate: predicate, status: status, sourceID: source})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return TreeDetail{}, err
	}
	rows.Close()
	for _, sourceRelationship := range sourceRelationships {
		newSubjectID, subjectOK := nodeMap[sourceRelationship.subjectID.String()]
		newObjectID, objectOK := nodeMap[sourceRelationship.objectID.String()]
		if !subjectOK || !objectOK {
			return TreeDetail{}, ErrValidation
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tree_relationships
				(id, tree_version_id, subject_node_id, object_node_id, predicate, status, source_id, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, uuid.New(), newVersionID, newSubjectID, newObjectID, sourceRelationship.predicate, sourceRelationship.status, sourceRelationship.sourceID, actorUUID); err != nil {
			return TreeDetail{}, err
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO tree_forks (tree_id, parent_tree_id, parent_version_id, forked_by)
		VALUES ($1, $2, $3, $4)
	`, newTreeID, treeUUID, sourceVersionUUID, actorUUID); err != nil {
		return TreeDetail{}, err
	}
	after := map[string]any{
		"parent_tree_id":    treeUUID.String(),
		"parent_version_id": sourceVersionUUID.String(),
		"source_version":    sourceVersionUUID.String(),
	}
	if err := writeTreeChange(ctx, tx, newTreeID, &newVersionID, actorUUID, "tree_forked", "tree", newTreeID, nil, after, ""); err != nil {
		return TreeDetail{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, after_value)
		VALUES ($1, 'tree_forked', 'tree', $2, $3)
	`, actorUUID, newTreeID, mustJSON(after)); err != nil {
		return TreeDetail{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TreeDetail{}, err
	}
	return s.GetTree(ctx, newTreeID.String(), actorID)
}

func (s *Service) CompareVersions(ctx context.Context, treeID, fromTreeID, fromVersionID, toVersionID, viewerID string) (TreeDiff, error) {
	if s == nil || s.Pool == nil {
		return TreeDiff{}, ErrDatabaseUnavailable
	}
	targetTreeUUID, err := uuid.Parse(treeID)
	if err != nil {
		return TreeDiff{}, ErrNotFound
	}
	if strings.TrimSpace(fromTreeID) == "" {
		fromTreeID = treeID
	}
	fromTreeUUID, err := uuid.Parse(fromTreeID)
	if err != nil {
		return TreeDiff{}, ErrNotFound
	}
	fromVersionUUID, err := uuid.Parse(fromVersionID)
	if err != nil {
		return TreeDiff{}, ErrInvalidDiff
	}
	toVersionUUID, err := uuid.Parse(toVersionID)
	if err != nil {
		return TreeDiff{}, ErrInvalidDiff
	}
	fromTree, fromVersion, fromNodes, fromRelationships, err := s.loadAuthorizedDiffVersion(ctx, fromTreeUUID, fromVersionUUID, viewerID)
	if err != nil {
		return TreeDiff{}, err
	}
	toTree, toVersion, toNodes, toRelationships, err := s.loadAuthorizedDiffVersion(ctx, targetTreeUUID, toVersionUUID, viewerID)
	if err != nil {
		return TreeDiff{}, err
	}
	return buildTreeDiff(fromTree, fromVersion, fromNodes, fromRelationships, toTree, toVersion, toNodes, toRelationships), nil
}

func (s *Service) loadAuthorizedDiffVersion(ctx context.Context, treeID, versionID uuid.UUID, viewerID string) (TreeSummary, TreeVersionView, []diffNode, []diffRelationship, error) {
	tree, err := s.treeSummary(ctx, treeID)
	if err != nil {
		return TreeSummary{}, TreeVersionView{}, nil, nil, err
	}
	version, err := s.version(ctx, treeID, versionID)
	if err != nil {
		return TreeSummary{}, TreeVersionView{}, nil, nil, err
	}
	allowed, err := s.canView(ctx, tree, viewerID)
	if err != nil {
		return TreeSummary{}, TreeVersionView{}, nil, nil, err
	}
	if !allowed {
		return TreeSummary{}, TreeVersionView{}, nil, nil, ErrForbidden
	}
	if version.State != "published" {
		draftVisible, err := s.canViewDraft(ctx, tree, viewerID)
		if err != nil {
			return TreeSummary{}, TreeVersionView{}, nil, nil, err
		}
		if !draftVisible {
			return TreeSummary{}, TreeVersionView{}, nil, nil, ErrNotFound
		}
	}
	nodes, relationships, err := s.loadDiffData(ctx, version.ID)
	if err != nil {
		return TreeSummary{}, TreeVersionView{}, nil, nil, err
	}
	return tree, version, nodes, relationships, nil
}

func (s *Service) loadDiffData(ctx context.Context, versionID string) ([]diffNode, []diffRelationship, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT tn.id, tn.person_id, tn.display_name_ar,
		       p.birth_date_from, p.birth_date_to, p.death_date_from, p.death_date_to
		FROM tree_nodes tn
		JOIN people p ON p.id = tn.person_id
		WHERE tn.tree_version_id = $1
		ORDER BY tn.sort_order, tn.created_at
	`, versionID)
	if err != nil {
		return nil, nil, err
	}
	nodes := make([]diffNode, 0)
	for rows.Next() {
		var nodeID, personID pgtype.UUID
		var displayName string
		var birthFrom, birthTo, deathFrom, deathTo pgtype.Date
		if err := rows.Scan(&nodeID, &personID, &displayName, &birthFrom, &birthTo, &deathFrom, &deathTo); err != nil {
			rows.Close()
			return nil, nil, err
		}
		node := diffNode{
			nodeID:      uuidString(nodeID),
			personID:    uuidString(personID),
			displayName: displayName,
			years:       formatYears(birthFrom, birthTo, deathFrom, deathTo),
			dateKey:     diffDateKey(birthFrom, birthTo, deathFrom, deathTo),
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()

	rows, err = s.Pool.Query(ctx, `
		SELECT id, subject_node_id, object_node_id, predicate, status, source_id
		FROM tree_relationships
		WHERE tree_version_id = $1
		ORDER BY created_at
	`, versionID)
	if err != nil {
		return nil, nil, err
	}
	relationships := make([]diffRelationship, 0)
	for rows.Next() {
		var relationshipID, subjectID, objectID, sourceID pgtype.UUID
		var predicate, status string
		if err := rows.Scan(&relationshipID, &subjectID, &objectID, &predicate, &status, &sourceID); err != nil {
			rows.Close()
			return nil, nil, err
		}
		relationships = append(relationships, diffRelationship{
			id:            uuidString(relationshipID),
			subjectNodeID: uuidString(subjectID),
			objectNodeID:  uuidString(objectID),
			predicate:     predicate,
			status:        status,
			sourceID:      uuidString(sourceID),
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()
	return nodes, relationships, nil
}

func buildTreeDiff(fromTree TreeSummary, fromVersion TreeVersionView, fromNodes []diffNode, fromRelationships []diffRelationship, toTree TreeSummary, toVersion TreeVersionView, toNodes []diffNode, toRelationships []diffRelationship) TreeDiff {
	result := TreeDiff{
		From:                 TreeDiffEndpoint{TreeID: fromTree.ID, TreeName: fromTree.Name, VersionID: fromVersion.ID, VersionNumber: fromVersion.Number},
		To:                   TreeDiffEndpoint{TreeID: toTree.ID, TreeName: toTree.Name, VersionID: toVersion.ID, VersionNumber: toVersion.Number},
		PeopleAdded:          make([]TreeDiffPerson, 0),
		PeopleRemoved:        make([]TreeDiffPerson, 0),
		DateChanges:          make([]TreeDiffDateChange, 0),
		RelationshipsAdded:   make([]TreeDiffRelationship, 0),
		RelationshipsRemoved: make([]TreeDiffRelationship, 0),
		RelationshipChanges:  make([]TreeDiffRelationshipChange, 0),
		SourcesAdded:         make([]TreeDiffSourceChange, 0),
		SourcesRemoved:       make([]TreeDiffSourceChange, 0),
	}
	pairs, added, removed := matchDiffNodes(fromNodes, toNodes)
	fromCanonical, toCanonical := canonicalNodeMaps(pairs, added, removed)
	for _, pair := range pairs {
		if pair.from.dateKey != pair.to.dateKey {
			result.DateChanges = append(result.DateChanges, TreeDiffDateChange{PersonID: pair.to.personID, DisplayName: pair.to.displayName, BeforeYears: pair.from.years, AfterYears: pair.to.years})
		}
	}
	for _, node := range added {
		result.PeopleAdded = append(result.PeopleAdded, TreeDiffPerson{PersonID: node.personID, DisplayName: node.displayName, Years: node.years})
	}
	for _, node := range removed {
		result.PeopleRemoved = append(result.PeopleRemoved, TreeDiffPerson{PersonID: node.personID, DisplayName: node.displayName, Years: node.years})
	}

	fromRelationshipsByKey := make(map[string]diffRelationship)
	for _, relationship := range fromRelationships {
		fromRelationshipsByKey[diffRelationshipKey(relationship, fromCanonical)] = relationship
	}
	toRelationshipsByKey := make(map[string]diffRelationship)
	for _, relationship := range toRelationships {
		toRelationshipsByKey[diffRelationshipKey(relationship, toCanonical)] = relationship
	}
	fromNames := diffNodeNames(fromCanonical, fromNodes)
	toNames := diffNodeNames(toCanonical, toNodes)
	changedChildren := make([]string, 0)
	for key, relationship := range toRelationshipsByKey {
		before, exists := fromRelationshipsByKey[key]
		if !exists {
			item := treeDiffRelationship(relationship, toCanonical, toNames)
			result.RelationshipsAdded = append(result.RelationshipsAdded, item)
			if relationship.predicate == "parent_of" {
				changedChildren = append(changedChildren, toCanonical[relationship.objectNodeID])
			}
			if relationship.sourceID != "" {
				result.SourcesAdded = append(result.SourcesAdded, TreeDiffSourceChange{SourceID: relationship.sourceID, RelationshipLabel: relationshipLabel(item)})
			}
			continue
		}
		if before.status != relationship.status || before.sourceID != relationship.sourceID {
			beforeItem := treeDiffRelationship(before, fromCanonical, fromNames)
			afterItem := treeDiffRelationship(relationship, toCanonical, toNames)
			result.RelationshipChanges = append(result.RelationshipChanges, TreeDiffRelationshipChange{Subject: afterItem.Subject, Object: afterItem.Object, Predicate: afterItem.Predicate, BeforeStatus: before.status, AfterStatus: relationship.status, BeforeSourceID: before.sourceID, AfterSourceID: relationship.sourceID})
			if relationship.predicate == "parent_of" {
				changedChildren = append(changedChildren, toCanonical[relationship.objectNodeID])
			}
			if before.sourceID != relationship.sourceID {
				if before.sourceID != "" {
					result.SourcesRemoved = append(result.SourcesRemoved, TreeDiffSourceChange{SourceID: before.sourceID, RelationshipLabel: relationshipLabel(beforeItem)})
				}
				if relationship.sourceID != "" {
					result.SourcesAdded = append(result.SourcesAdded, TreeDiffSourceChange{SourceID: relationship.sourceID, RelationshipLabel: relationshipLabel(afterItem)})
				}
			}
		}
		delete(fromRelationshipsByKey, key)
	}
	for _, relationship := range fromRelationshipsByKey {
		item := treeDiffRelationship(relationship, fromCanonical, fromNames)
		result.RelationshipsRemoved = append(result.RelationshipsRemoved, item)
		if relationship.predicate == "parent_of" {
			changedChildren = append(changedChildren, fromCanonical[relationship.objectNodeID])
		}
		if relationship.sourceID != "" {
			result.SourcesRemoved = append(result.SourcesRemoved, TreeDiffSourceChange{SourceID: relationship.sourceID, RelationshipLabel: relationshipLabel(item)})
		}
	}
	result.AffectedDescendants = countAffectedDescendants(changedChildren, toRelationships, toCanonical)
	return result
}

func validateForkInput(input ForkTreeInput) (ForkTreeInput, error) {
	input.VersionID = strings.TrimSpace(input.VersionID)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.Visibility = strings.TrimSpace(input.Visibility)
	if input.Visibility == "" {
		input.Visibility = "private"
	}
	if input.VersionID == "" || len([]rune(input.Name)) > 200 || len([]rune(input.Description)) > 2000 {
		return ForkTreeInput{}, ErrValidation
	}
	if input.Visibility != "private" && input.Visibility != "unlisted" && input.Visibility != "public" {
		return ForkTreeInput{}, ErrValidation
	}
	return input, nil
}

func matchDiffNodes(fromNodes, toNodes []diffNode) ([]diffNodePair, []diffNode, []diffNode) {
	pairs := make([]diffNodePair, 0)
	added := make([]diffNode, 0)
	removed := make([]diffNode, 0)
	used := make(map[int]bool)
	for toIndex := range toNodes {
		toNode := &toNodes[toIndex]
		fromIndex := -1
		for index, fromNode := range fromNodes {
			if used[index] {
				continue
			}
			if fromNode.personID == toNode.personID {
				fromIndex = index
				break
			}
		}
		if fromIndex == -1 {
			for index, fromNode := range fromNodes {
				if used[index] {
					continue
				}
				if identity.NormalizeArabicName(fromNode.displayName) == identity.NormalizeArabicName(toNode.displayName) {
					fromIndex = index
					break
				}
			}
		}
		if fromIndex == -1 {
			added = append(added, *toNode)
			continue
		}
		used[fromIndex] = true
		pairs = append(pairs, diffNodePair{from: &fromNodes[fromIndex], to: toNode})
	}
	for index, fromNode := range fromNodes {
		if !used[index] {
			removed = append(removed, fromNode)
		}
	}
	return pairs, added, removed
}

func canonicalNodeMaps(pairs []diffNodePair, added, removed []diffNode) (map[string]string, map[string]string) {
	fromMap := make(map[string]string)
	toMap := make(map[string]string)
	for _, pair := range pairs {
		canonical := pair.to.personID
		if canonical == "" {
			canonical = pair.from.personID
		}
		fromMap[pair.from.nodeID] = canonical
		toMap[pair.to.nodeID] = canonical
	}
	for _, node := range added {
		toMap[node.nodeID] = node.personID
	}
	for _, node := range removed {
		fromMap[node.nodeID] = node.personID
	}
	return fromMap, toMap
}

func diffRelationshipKey(relationship diffRelationship, canonical map[string]string) string {
	return canonical[relationship.subjectNodeID] + "|" + relationship.predicate + "|" + canonical[relationship.objectNodeID]
}

func diffNodeNames(canonical map[string]string, nodes []diffNode) map[string]string {
	names := make(map[string]string)
	for _, node := range nodes {
		names[canonical[node.nodeID]] = node.displayName
	}
	return names
}

func treeDiffRelationship(relationship diffRelationship, canonical, names map[string]string) TreeDiffRelationship {
	return TreeDiffRelationship{
		Subject:   names[canonical[relationship.subjectNodeID]],
		Object:    names[canonical[relationship.objectNodeID]],
		Predicate: relationship.predicate,
		Status:    relationship.status,
		SourceID:  relationship.sourceID,
	}
}

func relationshipLabel(relationship TreeDiffRelationship) string {
	return relationship.Subject + " → " + relationship.Object + " · " + relationship.Predicate
}

func countAffectedDescendants(roots []string, relationships []diffRelationship, canonical map[string]string) int {
	children := make(map[string][]string)
	for _, relationship := range relationships {
		if relationship.predicate == "parent_of" {
			subject := canonical[relationship.subjectNodeID]
			object := canonical[relationship.objectNodeID]
			children[subject] = append(children[subject], object)
		}
	}
	visited := make(map[string]bool)
	queued := make(map[string]bool)
	queue := make([]string, 0, len(roots))
	for _, root := range roots {
		if root == "" || queued[root] {
			continue
		}
		queued[root] = true
		queue = append(queue, root)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, child := range children[current] {
			if !visited[child] {
				visited[child] = true
				queue = append(queue, child)
			}
		}
	}
	return len(visited)
}

func diffDateKey(values ...pgtype.Date) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if !value.Valid {
			parts = append(parts, "")
			continue
		}
		parts = append(parts, value.Time.UTC().Format("2006-01-02"))
	}
	return strings.Join(parts, "|")
}

func mustJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
