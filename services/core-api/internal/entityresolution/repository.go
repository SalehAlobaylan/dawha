package entityresolution

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxEntitySnapshots = 1000

func (s *Service) loadSnapshots(ctx context.Context, entityType EntityType) ([]*entitySnapshot, error) {
	result := make([]*entitySnapshot, 0)
	if entityType == EntityPerson || entityType == EntityAll {
		people, err := s.loadPeople(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, people...)
	}
	if entityType == EntityFamily || entityType == EntityAll {
		families, err := s.loadFamilies(ctx)
		if err != nil {
			return nil, err
		}
		result = append(result, families...)
	}
	return result, nil
}

func (s *Service) loadPeople(ctx context.Context) ([]*entitySnapshot, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, canonical_name_ar, normalized_name_ar, gender, birth_date_from, birth_date_to, death_date_from, death_date_to
		FROM people
		WHERE merged_into_id IS NULL
		ORDER BY id
		LIMIT $1
	`, maxEntitySnapshots)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*entitySnapshot, 0)
	byID := make(map[uuid.UUID]*entitySnapshot)
	for rows.Next() {
		var id uuid.UUID
		var name, normalized string
		var gender pgtype.Text
		var birthFrom, birthTo, deathFrom, deathTo pgtype.Date
		if err := rows.Scan(&id, &name, &normalized, &gender, &birthFrom, &birthTo, &deathFrom, &deathTo); err != nil {
			return nil, err
		}
		snapshot := newEntitySnapshot(id, EntityPerson, name, normalized)
		snapshot.Gender = textValue(gender)
		snapshot.BirthFrom = birthFrom
		snapshot.BirthTo = birthTo
		snapshot.DeathFrom = deathFrom
		snapshot.DeathTo = deathTo
		result = append(result, snapshot)
		byID[id] = snapshot
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	aliasRows, err := s.Pool.Query(ctx, `SELECT person_id, normalized_value_ar FROM person_aliases ORDER BY person_id, created_at`)
	if err != nil {
		return nil, err
	}
	for aliasRows.Next() {
		var personID uuid.UUID
		var alias string
		if err := aliasRows.Scan(&personID, &alias); err != nil {
			aliasRows.Close()
			return nil, err
		}
		if snapshot := byID[personID]; snapshot != nil && strings.TrimSpace(alias) != "" {
			snapshot.Aliases = append(snapshot.Aliases, alias)
		}
	}
	if err := aliasRows.Err(); err != nil {
		aliasRows.Close()
		return nil, err
	}
	aliasRows.Close()

	relationshipRows, err := s.Pool.Query(ctx, `
		SELECT subject_id, predicate, object_id
		FROM entity_relationships
		WHERE subject_type = 'person' AND object_type = 'person'
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	for relationshipRows.Next() {
		var subjectID, objectID uuid.UUID
		var predicate string
		if err := relationshipRows.Scan(&subjectID, &predicate, &objectID); err != nil {
			relationshipRows.Close()
			return nil, err
		}
		applyRelation(byID, subjectID, predicate, objectID)
	}
	if err := relationshipRows.Err(); err != nil {
		relationshipRows.Close()
		return nil, err
	}
	relationshipRows.Close()

	claimRows, err := s.Pool.Query(ctx, `
		SELECT subject_id, predicate, object_id, place_id
		FROM claims
		WHERE subject_type = 'person' AND object_type = 'person'
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	for claimRows.Next() {
		var subjectID, objectID uuid.UUID
		var predicate string
		var placeID pgtype.UUID
		if err := claimRows.Scan(&subjectID, &predicate, &objectID, &placeID); err != nil {
			claimRows.Close()
			return nil, err
		}
		applyRelation(byID, subjectID, predicate, objectID)
		if placeID.Valid {
			place := placeID.Bytes
			if subject := byID[subjectID]; subject != nil {
				addSet(subject.Places, uuid.UUID(place).String())
			}
			if object := byID[objectID]; object != nil {
				addSet(object.Places, uuid.UUID(place).String())
			}
		}
	}
	if err := claimRows.Err(); err != nil {
		claimRows.Close()
		return nil, err
	}
	claimRows.Close()

	geoRows, err := s.Pool.Query(ctx, `
		SELECT entity_id, place_id
		FROM geographic_associations
		WHERE entity_type = 'person'
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	for geoRows.Next() {
		var personID, placeID uuid.UUID
		if err := geoRows.Scan(&personID, &placeID); err != nil {
			geoRows.Close()
			return nil, err
		}
		if snapshot := byID[personID]; snapshot != nil {
			addSet(snapshot.Places, placeID.String())
		}
	}
	if err := geoRows.Err(); err != nil {
		geoRows.Close()
		return nil, err
	}
	geoRows.Close()

	sourceRows, err := s.Pool.Query(ctx, `
		SELECT c.subject_id, c.object_id, ss.source_id
		FROM claims c
		JOIN claim_evidence ce ON ce.claim_id = c.id
		JOIN source_statements ss ON ss.id = ce.source_statement_id
		WHERE c.subject_type = 'person' AND c.object_type = 'person' AND ss.review_status = 'accepted'
		ORDER BY c.id
	`)
	if err != nil {
		return nil, err
	}
	for sourceRows.Next() {
		var subjectID, objectID, sourceID uuid.UUID
		if err := sourceRows.Scan(&subjectID, &objectID, &sourceID); err != nil {
			sourceRows.Close()
			return nil, err
		}
		if snapshot := byID[subjectID]; snapshot != nil {
			addSet(snapshot.Sources, sourceID.String())
		}
		if snapshot := byID[objectID]; snapshot != nil {
			addSet(snapshot.Sources, sourceID.String())
		}
	}
	if err := sourceRows.Err(); err != nil {
		sourceRows.Close()
		return nil, err
	}
	sourceRows.Close()

	treeRows, err := s.Pool.Query(ctx, `
		SELECT DISTINCT tn.person_id, tv.tree_id
		FROM tree_nodes tn
		JOIN tree_versions tv ON tv.id = tn.tree_version_id
		ORDER BY tn.person_id, tv.tree_id
	`)
	if err != nil {
		return nil, err
	}
	for treeRows.Next() {
		var personID, treeID uuid.UUID
		if err := treeRows.Scan(&personID, &treeID); err != nil {
			treeRows.Close()
			return nil, err
		}
		if snapshot := byID[personID]; snapshot != nil {
			addSet(snapshot.Trees, treeID.String())
		}
	}
	if err := treeRows.Err(); err != nil {
		treeRows.Close()
		return nil, err
	}
	treeRows.Close()

	for _, snapshot := range result {
		for fatherID := range snapshot.Fathers {
			parsed, err := uuid.Parse(fatherID)
			if err != nil {
				continue
			}
			if father := byID[parsed]; father != nil {
				addSet(snapshot.Grandfathers, setValues(father.Fathers)...)
			}
		}
	}
	return result, nil
}

func applyRelation(byID map[uuid.UUID]*entitySnapshot, subjectID uuid.UUID, predicate string, objectID uuid.UUID) {
	subject, subjectOK := byID[subjectID]
	object, objectOK := byID[objectID]
	if !subjectOK || !objectOK {
		return
	}
	predicate = strings.ToLower(strings.TrimSpace(predicate))
	switch predicate {
	case "father_of":
		addSet(subject.Fathers, objectID.String())
		addSet(object.Children, subjectID.String())
	case "parent_of":
		addSet(object.Fathers, subjectID.String())
		addSet(subject.Children, objectID.String())
	case "child_of", "son_of", "daughter_of":
		addSet(object.Fathers, subjectID.String())
		addSet(subject.Children, objectID.String())
	case "sibling_of":
		addSet(subject.Siblings, objectID.String())
		addSet(object.Siblings, subjectID.String())
	case "spouse_of":
		addSet(subject.Spouses, objectID.String())
		addSet(object.Spouses, subjectID.String())
	}
}

func (s *Service) loadFamilies(ctx context.Context) ([]*entitySnapshot, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT id, canonical_name_ar, normalized_name_ar, origin_place_id
		FROM families
		WHERE merged_into_id IS NULL
		ORDER BY id
		LIMIT $1
	`, maxEntitySnapshots)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]*entitySnapshot, 0)
	byID := make(map[uuid.UUID]*entitySnapshot)
	for rows.Next() {
		var id uuid.UUID
		var name, normalized string
		var placeID pgtype.UUID
		if err := rows.Scan(&id, &name, &normalized, &placeID); err != nil {
			return nil, err
		}
		snapshot := newEntitySnapshot(id, EntityFamily, name, normalized)
		if placeID.Valid {
			snapshot.OriginPlace = uuid.UUID(placeID.Bytes).String()
			addSet(snapshot.Places, snapshot.OriginPlace)
		}
		result = append(result, snapshot)
		byID[id] = snapshot
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	aliasRows, err := s.Pool.Query(ctx, `SELECT family_id, normalized_value_ar FROM family_aliases ORDER BY family_id, created_at`)
	if err != nil {
		return nil, err
	}
	for aliasRows.Next() {
		var familyID uuid.UUID
		var alias string
		if err := aliasRows.Scan(&familyID, &alias); err != nil {
			aliasRows.Close()
			return nil, err
		}
		if snapshot := byID[familyID]; snapshot != nil && strings.TrimSpace(alias) != "" {
			snapshot.Aliases = append(snapshot.Aliases, alias)
		}
	}
	if err := aliasRows.Err(); err != nil {
		aliasRows.Close()
		return nil, err
	}
	aliasRows.Close()

	branchRows, err := s.Pool.Query(ctx, `SELECT family_id, id, canonical_name_ar, normalized_name_ar FROM branches ORDER BY family_id, id`)
	if err != nil {
		return nil, err
	}
	for branchRows.Next() {
		var familyID, branchID uuid.UUID
		var name, normalized string
		if err := branchRows.Scan(&familyID, &branchID, &name, &normalized); err != nil {
			branchRows.Close()
			return nil, err
		}
		if snapshot := byID[familyID]; snapshot != nil {
			addSet(snapshot.Branches, branchID.String(), normalized, strings.ToLower(strings.TrimSpace(name)))
		}
	}
	if err := branchRows.Err(); err != nil {
		branchRows.Close()
		return nil, err
	}
	branchRows.Close()

	claimRows, err := s.Pool.Query(ctx, `
		SELECT subject_id, object_id, place_id
		FROM claims
		WHERE subject_type = 'family' OR object_type = 'family'
		ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	for claimRows.Next() {
		var subjectID, objectID uuid.UUID
		var placeID pgtype.UUID
		if err := claimRows.Scan(&subjectID, &objectID, &placeID); err != nil {
			claimRows.Close()
			return nil, err
		}
		if placeID.Valid {
			if snapshot := byID[subjectID]; snapshot != nil {
				addSet(snapshot.Places, uuid.UUID(placeID.Bytes).String())
			}
			if snapshot := byID[objectID]; snapshot != nil {
				addSet(snapshot.Places, uuid.UUID(placeID.Bytes).String())
			}
		}
	}
	if err := claimRows.Err(); err != nil {
		claimRows.Close()
		return nil, err
	}
	claimRows.Close()
	return result, nil
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
