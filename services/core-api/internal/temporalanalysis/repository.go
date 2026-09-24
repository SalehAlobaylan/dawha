package temporalanalysis

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type treeScope struct {
	TreeID        string
	TreeVersionID string
	Visibility    string
	VersionState  string
	VersionNumber int
	TargetPresent bool
}

type edgeRow struct {
	RelationshipID  string
	ParentPersonID  string
	ChildPersonID   string
	ClaimID         string
	SourceID        string
	ExclusionReason string
	ParentBirthFrom time.Time
	ParentBirthTo   time.Time
	ChildBirthFrom  time.Time
	ChildBirthTo    time.Time
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadTreeScope(ctx context.Context, q queryer, treeID, versionID, targetPersonID string) (treeScope, error) {
	var result treeScope
	var targetPresent bool
	if err := q.QueryRow(ctx, `
		SELECT t.id::text, tv.id::text, t.visibility, tv.state, tv.version_number,
		       EXISTS (SELECT 1 FROM tree_nodes tn WHERE tn.tree_version_id = tv.id AND tn.person_id = $3)
		FROM trees t
		JOIN tree_versions tv ON tv.tree_id = t.id
		WHERE t.id = $1 AND tv.id = $2
	`, treeID, versionID, targetPersonID).Scan(&result.TreeID, &result.TreeVersionID, &result.Visibility, &result.VersionState, &result.VersionNumber, &targetPresent); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return treeScope{}, ErrNotFound
		}
		return treeScope{}, err
	}
	result.TargetPresent = targetPresent
	return result, nil
}

func loadGenerationEdges(ctx context.Context, q queryer, versionID string) ([]edgeRow, bool, error) {
	rows, err := q.Query(ctx, `
		WITH candidate_edges AS (
			SELECT tr.id AS relationship_id,
			       parent.id AS parent_person_id,
			       child.id AS child_person_id,
			       tr.source_id,
			       c.id AS claim_id,
			       parent.identity_status AS parent_identity_status,
			       parent.merged_into_id AS parent_merged_into_id,
			       child.identity_status AS child_identity_status,
			       child.merged_into_id AS child_merged_into_id,
			       parent.birth_date_from AS parent_birth_from,
			       parent.birth_date_to AS parent_birth_to,
			       child.birth_date_from AS child_birth_from,
			       child.birth_date_to AS child_birth_to,
			       s.visibility AS source_visibility,
			       s.dependency_status AS source_dependency_status,
			       COALESCE(c.accepted_evidence, false) AS accepted_evidence,
			       COALESCE(c.counter_evidence, false) AS counter_evidence
			FROM tree_relationships tr
			JOIN tree_versions tv ON tv.id = tr.tree_version_id
			JOIN tree_nodes parent_node ON parent_node.id = tr.subject_node_id
			JOIN tree_nodes child_node ON child_node.id = tr.object_node_id
			JOIN people parent ON parent.id = parent_node.person_id
			JOIN people child ON child.id = child_node.person_id
			LEFT JOIN LATERAL (
				SELECT c.id,
				       EXISTS (
				         SELECT 1 FROM claim_evidence ce
				         JOIN source_statements ss ON ss.id = ce.source_statement_id
				         WHERE ce.claim_id = c.id AND ss.source_id = tr.source_id AND ss.review_status = 'accepted'
				       ) AS accepted_evidence,
				       EXISTS (SELECT 1 FROM claim_counter_evidence cce WHERE cce.claim_id = c.id) AS counter_evidence
				FROM claims c
				WHERE c.subject_type = 'person'
				  AND c.object_type = 'person'
				  AND c.subject_id = parent.id
				  AND c.object_id = child.id
				  AND c.predicate = 'parent_of'
				  AND c.status = 'supported'
				ORDER BY accepted_evidence DESC, c.id
				LIMIT 1
			) c ON true
			LEFT JOIN sources s ON s.id = tr.source_id
			WHERE tv.id = $1
			  AND tv.state = 'published'
			  AND tr.predicate = 'parent_of'
			  AND tr.status = 'interpreted'
		), ranked_edges AS (
			SELECT candidate_edges.*,
			       CASE
			         WHEN parent_identity_status <> 'reviewed' OR parent_merged_into_id IS NOT NULL OR child_identity_status <> 'reviewed' OR child_merged_into_id IS NOT NULL THEN 'unreviewed_identity'
			         WHEN source_id IS NULL THEN 'missing_source'
			         WHEN source_visibility <> 'public' THEN 'private_source'
			         WHEN source_dependency_status <> 'independent' OR EXISTS (
			           SELECT 1 FROM source_dependencies sd
			           WHERE sd.status IN ('needs_review', 'confirmed')
			             AND (sd.source_id = candidate_edges.source_id OR sd.depends_on_source_id = candidate_edges.source_id)
			         ) THEN 'dependent_source'
			         WHEN claim_id IS NULL THEN 'missing_claim'
			         WHEN NOT accepted_evidence THEN 'unaccepted_evidence'
			         WHEN counter_evidence THEN 'counter_evidence'
			         WHEN parent_birth_from IS NULL OR parent_birth_to IS NULL OR child_birth_from IS NULL OR child_birth_to IS NULL
			              OR parent_birth_from > parent_birth_to OR child_birth_from > child_birth_to OR parent_birth_from > child_birth_to THEN 'invalid_dates'
			         ELSE 'qualified'
			       END AS exclusion_reason
			FROM candidate_edges
		)
		SELECT DISTINCT ON (parent_person_id, child_person_id)
		       relationship_id, parent_person_id, child_person_id, claim_id, source_id, exclusion_reason,
		       parent_birth_from, parent_birth_to, child_birth_from, child_birth_to
		FROM ranked_edges
		ORDER BY parent_person_id, child_person_id, (exclusion_reason = 'qualified') DESC, relationship_id
		LIMIT $2
	`, versionID, MaximumReferenceEdges+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]edgeRow, 0)
	for rows.Next() {
		var item edgeRow
		var relationshipID, parentID, childID, claimID, sourceID pgtype.UUID
		var parentFrom, parentTo, childFrom, childTo pgtype.Date
		if err := rows.Scan(&relationshipID, &parentID, &childID, &claimID, &sourceID, &item.ExclusionReason, &parentFrom, &parentTo, &childFrom, &childTo); err != nil {
			return nil, false, err
		}
		item.RelationshipID = uuidString(relationshipID)
		item.ParentPersonID = uuidString(parentID)
		item.ChildPersonID = uuidString(childID)
		item.ClaimID = uuidString(claimID)
		item.SourceID = uuidString(sourceID)
		item.ParentBirthFrom = dateValue(parentFrom)
		item.ParentBirthTo = dateValue(parentTo)
		item.ChildBirthFrom = dateValue(childFrom)
		item.ChildBirthTo = dateValue(childTo)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(items) > MaximumReferenceEdges
	if truncated {
		items = items[:MaximumReferenceEdges]
	}
	return items, truncated, nil
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	return uuid.UUID(value.Bytes).String()
}

func dateValue(value pgtype.Date) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
