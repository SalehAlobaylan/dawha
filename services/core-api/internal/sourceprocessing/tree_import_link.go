package sourceprocessing

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// The import's identity linking.
//
// Three rules hold here, and every one of them exists because a silent act is
// the failure mode this feature must never have:
//
//   - An occurrence whose name matches an existing person is LINKED to that
//     person, never duplicated beside them. The link is shown to the reviewer
//     in the dry run before anything is written, and the apply run refuses to
//     proceed without that report.
//   - An occurrence with no exact match becomes a new person, and if a
//     plausibly-similar person already exists, a candidate is PROPOSED to the
//     entity-resolution review surface. The import itself merges nothing:
//     no entity_merges row, no merged_into_id, no identity_status change.
//   - The reading is never destroyed: the verbatim page string is kept as a
//     source-scoped alias beside the canonical name.

// TreeImportMatchThreshold is the score at or above which an occurrence is
// linked instead of duplicated. It is the same threshold the extraction path
// applies to an AI-resolved reference (`resolveEntity` skips matches below
// 0.5), so one decision uses one threshold on both paths.
const TreeImportMatchThreshold = 0.5

// Note: treeImportNormalizationVersion is the same value the resolution sweep
// uses (`entityresolution.NormalizationVersion`), restated locally because
// importing that package would invert its dependency direction.
const (
	treeImportExactScore           = 1.0  // an exact normalised canonical-name match
	treeImportAliasScore           = 0.8  // an exact normalised alias match
	treeImportFuzzyFloor           = 0.45 // the smallest similarity worth proposing
	treeImportFuzzyPerOccurrence   = 5    // the most fuzzy proposals per occurrence
	treeImportFuzzyPerImport       = 20   // the most fuzzy proposals per import
	treeImportAlgorithmVersion     = "tree-import-link-v1"
	treeImportNormalizationVersion = "arabic-normalization-v1"
)

// treeImportPhrase maps the representable predicates to Arabic wording for a
// statement. The wording stays gender-neutral, because a chart does not always
// state the gender.
var treeImportPhrase = map[string]string{
	"parent_of":  "والدٌ لِ",
	"spouse_of":  "متزوج/متزوجة من",
	"sibling_of": "شقيق/شقيقة لِ",
}

// TreeDraftCollision is one occurrence proposed against an existing person.
type TreeDraftCollision struct {
	DraftNodeID      string  `json:"draftNodeId"`
	DraftNameAR      string  `json:"draftNameAr"`
	ExistingPersonID string  `json:"existingPersonId"`
	ExistingNameAR   string  `json:"existingNameAr"`
	MatchClass       string  `json:"matchClass"`
	Score            float64 `json:"score"`
	Basis            string  `json:"basis"`
}

// TreeDraftImportPreview is what a dry run reports. It writes nothing, and its
// counts are the counts the apply run would realise if nothing changed.
type TreeDraftImportPreview struct {
	DraftID            string               `json:"draftId"`
	ExpectedRevision   int                  `json:"expectedRevision"`
	SourceFileChecksum string               `json:"sourceFileChecksum"`
	NewPeople          int                  `json:"newPeople"`
	LinkedExisting     int                  `json:"linkedExisting"`
	Collisions         []TreeDraftCollision `json:"collisions"`
	Relations          int                  `json:"relations"`
	StagedRelations    int                  `json:"stagedRelations"`
	Statements         int                  `json:"statements"`
	Aliases            int                  `json:"aliases"`
	Candidates         int                  `json:"candidates"`
	ExcludedNodes      int                  `json:"excludedNodes"`
	Warnings           []string             `json:"warnings"`
}

// TreeDraftImportInput is the apply-time acknowledgement of the preview. A dry
// run that the apply run may ignore is not a dry run, so the apply carries the
// report's preconditions back and refuses on any change.
type TreeDraftImportInput struct {
	ExpectedRevision     int    `json:"expectedRevision"`
	AcknowledgePreview   bool   `json:"acknowledgePreview"`
	SourceFileChecksum   string `json:"sourceFileChecksum"`
	ExpectedNewPeople    int    `json:"expectedNewPeople"`
	ExpectedLinkedPeople int    `json:"expectedLinkedPeople"`
	ExpectedRelations    int    `json:"expectedRelations"`
	ExpectedCollisions   int    `json:"expectedCollisions"`
	ExpectedStatements   int    `json:"expectedStatements"`
	ExpectedAliases      int    `json:"expectedAliases"`
}

// existingPerson is the person an occurrence links to.
type existingPerson struct {
	ID         uuid.UUID
	Name       string
	Basis      string // "canonical" or "alias"
	Score      float64
	OccupiedBy string // the node that already linked this person, if any
}

// treeCandidate is one fuzzy proposal for the resolution surface.
type treeCandidate struct {
	ExistingID   uuid.UUID
	ExistingName string
	Score        float64
}

// treeImportNodePlan is one included node and what the import would do with it.
type treeImportNodePlan struct {
	Node     TreeDraftNode
	New      bool
	PersonID uuid.UUID // the created person, when New; the existing person, when linked
	Linked   *existingPerson
	KunyahAR string
	Fuzzy    []treeCandidate
	// Written ids, recorded by the executor.
	SourceSpellingAliasID *uuid.UUID
	KunyahAliasID         *uuid.UUID
}

// treeImportEdgeResult records the ids one reviewed edge wrote.
type treeImportEdgeResult struct {
	Edge                 TreeDraftEdge
	SubjectPerson        uuid.UUID
	ObjectPerson         uuid.UUID
	SubjectName          string
	ObjectName           string
	TreeRelationshipID   uuid.UUID
	StatementID          uuid.UUID
	EntityRelationshipID uuid.UUID
}

// treeImportPlan is everything the import computes before it writes anything.
// The preview runs only the build; the apply runs build and execute, so the
// two cannot drift.
type treeImportPlan struct {
	SourceID           uuid.UUID
	SourceFileID       uuid.UUID
	SourceFileChecksum string
	DraftRevision      int

	Nodes []treeImportNodePlan
	Edges []TreeDraftEdge // the reviewed, representable edges, in order

	Collisions []TreeDraftCollision

	NewPeople       int
	LinkedPeople    int
	StagedRelations int
	Statements      int
	Aliases         int
	Candidates      int
	ExcludedNodes   int
	Warnings        []string
}

// buildTreeImportPlan computes the whole import without writing anything.
//
// It is the same computation the apply run executes; the dry run is this
// function plus a rollback, not a separate estimate.
func buildTreeImportPlan(ctx context.Context, tx pgx.Tx, row treeDraftRow, view TreeDraftView) (treeImportPlan, error) {
	plan := treeImportPlan{
		SourceID:      row.SourceID,
		SourceFileID:  row.SourceFileID,
		DraftRevision: row.Revision,
	}
	if err := tx.QueryRow(ctx, `SELECT checksum_sha256 FROM source_files WHERE id = $1`, row.SourceFileID).Scan(&plan.SourceFileChecksum); err != nil {
		return plan, fmt.Errorf("source file checksum: %w", err)
	}

	occupied := make(map[uuid.UUID]string) // one person is linked at most once per import
	fuzzyBudget := treeImportFuzzyPerImport
	for _, node := range view.Nodes {
		if !node.Included {
			plan.ExcludedNodes++
			continue
		}
		entry := treeImportNodePlan{Node: node, New: true}
		normalized := identity.NormalizeArabicName(node.NameAR)

		match, found, err := matchExistingPerson(ctx, tx, normalized)
		if err != nil {
			return plan, err
		}
		if found && occupied[match.ID] == "" {
			occupied[match.ID] = node.ID
			match.OccupiedBy = node.ID
			entry.New = false
			entry.Linked = &match
			entry.PersonID = match.ID
			entry.KunyahAR = extractKunyah(node.NameAR)
			plan.LinkedPeople++
			plan.Collisions = append(plan.Collisions, TreeDraftCollision{
				DraftNodeID:      node.ID,
				DraftNameAR:      node.NameAR,
				ExistingPersonID: match.ID.String(),
				ExistingNameAR:   match.Name,
				MatchClass:       "strong_candidate",
				Score:            match.Score,
				Basis:            match.Basis,
			})
		} else {
			if found {
				// Two occurrences of one name are two people, not one. A chart
				// of cousins can hold two عبداللهs, and collapsing them would
				// be a silent merge of real ancestors.
				plan.Warnings = append(plan.Warnings, fmt.Sprintf(
					"الاسم «%s» يطابق شخصاً رُبط سابقاً في هذه المسودة؛ أُنشئ شخص جديد بدل الربط الثاني، فمرجعية كل اسم تبقى مستقلة.", node.NameAR))
			}
			entry.KunyahAR = extractKunyah(node.NameAR)
			fuzzy, err := fuzzyPersonMatches(ctx, tx, normalized)
			if err != nil {
				return plan, err
			}
			// The proposal list is bounded per import; the occurrences that
			// fall past the budget still become people, and the full sweep
			// remains the place for the rest.
			if len(fuzzy) > fuzzyBudget {
				fuzzy = fuzzy[:fuzzyBudget]
			}
			fuzzyBudget -= len(fuzzy)
			entry.Fuzzy = fuzzy
			plan.Candidates += len(fuzzy)
		}
		if entry.New {
			if k := identity.NormalizeArabicName(entry.KunyahAR); k == "" || k == normalized {
				entry.KunyahAR = ""
			} else {
				plan.Aliases++
			}
			plan.Aliases++ // the source_spelling row every occurrence writes
			plan.NewPeople++
		} else {
			entry.KunyahAR = ""
		}
		plan.Nodes = append(plan.Nodes, entry)
	}

	nodePlanByID := make(map[string]int, len(plan.Nodes))
	for index := range plan.Nodes {
		nodePlanByID[plan.Nodes[index].Node.ID] = index
	}
	for _, edge := range view.Edges {
		if edge.Status != "reviewed" {
			continue
		}
		if !supportedTreePredicate(edge.Predicate) {
			plan.StagedRelations++
			plan.Warnings = append(plan.Warnings, "A reviewed relationship uses a meaning Dawha's tree model cannot yet represent; it remains in the source draft.")
			continue
		}
		subject, subjectOK := nodePlanByID[edge.SubjectNodeID]
		object, objectOK := nodePlanByID[edge.ObjectNodeID]
		if !subjectOK || !objectOK {
			return plan, ErrValidation
		}
		plan.Edges = append(plan.Edges, edge)
		_ = subject
		_ = object
		plan.Statements++
	}
	return plan, nil
}

// executeTreeImportPlan writes everything the plan computed, inside the
// import's own transaction, so the tree rows, the graph rows, the statements,
// the aliases and the resolution candidates commit together or not at all.
func executeTreeImportPlan(ctx context.Context, s *Service, tx pgx.Tx, row treeDraftRow, draftUUID, actorUUID uuid.UUID, plan *treeImportPlan) (treeID, versionID uuid.UUID, edges []treeImportEdgeResult, runID string, wrote treeImportWriteCounts, err error) {
	var sourceTitle string
	if err = tx.QueryRow(ctx, `SELECT title_ar FROM sources WHERE id = $1`, row.SourceID).Scan(&sourceTitle); err != nil {
		return
	}
	treeID, versionID = uuid.New(), uuid.New()
	if _, err = tx.Exec(ctx, `INSERT INTO trees (id, name_ar, description_ar, visibility, owner_id) VALUES ($1, $2, $3, 'private', $4)`, treeID, row.TitleAR, "مسودة خاصة مستوردة من مصدر: "+sourceTitle, actorUUID); err != nil {
		return
	}
	if _, err = tx.Exec(ctx, `INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'draft', $3)`, versionID, treeID, actorUUID); err != nil {
		return
	}

	nodeIDMap := make(map[string]uuid.UUID, len(plan.Nodes))
	for index := range plan.Nodes {
		entry := &plan.Nodes[index]
		node := entry.Node
		if entry.New {
			// The generated id lives with the executor: the dry run runs the
			// same builder and must not mint ids it can never use. A linked
			// occurrence creates no row — it reuses the existing person, which
			// is the whole point of linking instead of duplicating.
			if entry.PersonID == uuid.Nil {
				entry.PersonID = uuid.New()
			}
			if _, err = tx.Exec(ctx, `INSERT INTO people (id, canonical_name_ar, normalized_name_ar, gender, created_by) VALUES ($1, $2, $3, 'unknown', $4)`, entry.PersonID, node.NameAR, identity.NormalizeArabicName(node.NameAR), actorUUID); err != nil {
				return
			}
		}
		// The verbatim reading survives as a source-scoped alias beside the
		// canonical name, for new and linked people alike. A repeat of the same
		// alias is a repeat of the same fact, so the conflict is a no-op.
		aliasID, written, writeErr := writeSourceAlias(ctx, tx, entry.PersonID, node.NameAR, row.SourceID)
		if writeErr != nil {
			err = writeErr
			return
		}
		if written {
			wrote.Aliases++
			if aliasID != nil {
				entry.SourceSpellingAliasID = aliasID
				wrote.AliasIDs = append(wrote.AliasIDs, *aliasID)
			}
		}
		if entry.KunyahAR != "" {
			kunyahID, kunyahWritten, kunyahErr := writeSourceAliasType(ctx, tx, entry.PersonID, entry.KunyahAR, "kunyah", row.SourceID)
			if kunyahErr != nil {
				err = kunyahErr
				return
			}
			if kunyahWritten {
				wrote.Aliases++
				if kunyahID != nil {
					entry.KunyahAliasID = kunyahID
					wrote.AliasIDs = append(wrote.AliasIDs, *kunyahID)
				}
			}
		}
		treeNodeID := uuid.New()
		if _, err = tx.Exec(ctx, `INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, $4, $5)`, treeNodeID, versionID, entry.PersonID, node.NameAR, index); err != nil {
			return
		}
		nodeIDMap[node.ID] = treeNodeID
	}

	runID, wrote.Candidates, err = writeImportResolutionCandidates(ctx, tx, actorUUID, plan)
	if err != nil {
		return
	}

	for _, edge := range plan.Edges {
		subject := plan.NodeByDraftID(edge.SubjectNodeID)
		object := plan.NodeByDraftID(edge.ObjectNodeID)
		if subject == nil || object == nil {
			err = ErrValidation
			return
		}
		// The reviewed edge reaches the research graph as a statement under the
		// extraction_method value the schema has accepted since migration 0003
		// made room for: 'imported'. No claim is created at import time; a
		// claim waits for the human resolution of the fuzzy candidates.
		statementID := uuid.New()
		phrase := treeImportPhrase[edge.Predicate]
		if phrase == "" {
			phrase = edge.Predicate
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO source_statements (id, source_id, statement_text_ar, locator_ar, extraction_method, review_status, created_by)
			VALUES ($1, $2, $3, $4, 'imported', 'accepted', $5)
		`, statementID, row.SourceID,
			fmt.Sprintf("وفق مسودة الشجرة المُراجَعة: %s %s %s.", subject.Node.NameAR, phrase, object.Node.NameAR),
			"شجرة أسرية مُراجَعة؛ وصلة المسودة: "+edge.ID, actorUUID); err != nil {
			return
		}
		entityRelationshipID := uuid.New()
		if _, err = tx.Exec(ctx, `INSERT INTO entity_relationships (id, subject_type, subject_id, predicate, object_type, object_id, status, created_by) VALUES ($1, 'person', $2, $3, 'person', $4, 'unresolved', $5)`,
			entityRelationshipID, subject.PersonID, edge.Predicate, object.PersonID, actorUUID); err != nil {
			return
		}
		relationshipID := uuid.New()
		if _, err = tx.Exec(ctx, `INSERT INTO tree_relationships (id, tree_version_id, subject_node_id, object_node_id, predicate, status, source_id, created_by) VALUES ($1, $2, $3, $4, $5, 'unresolved', $6, $7)`,
			relationshipID, versionID, nodeIDMap[edge.SubjectNodeID], nodeIDMap[edge.ObjectNodeID], edge.Predicate, row.SourceID, actorUUID); err != nil {
			return
		}
		wrote.Statements++
		edges = append(edges, treeImportEdgeResult{
			Edge: edge, SubjectPerson: subject.PersonID, ObjectPerson: object.PersonID,
			SubjectName: subject.Node.NameAR, ObjectName: object.Node.NameAR,
			TreeRelationshipID: relationshipID, StatementID: statementID, EntityRelationshipID: entityRelationshipID,
		})
	}
	wrote.NodeIDMap = nodeIDMap
	return
}

// treeImportWriteCounts carries the actual write counts back to the result, so
// the reported numbers are what the transaction wrote and not what the plan
// predicted.
type treeImportWriteCounts struct {
	Aliases    int
	AliasIDs   []uuid.UUID
	Statements int
	Candidates int
	NodeIDMap  map[string]uuid.UUID
}

// writeSourceAlias writes the source_spelling alias: value verbatim, value
// normalised, source recorded. It is the only record of what the page said.
func writeSourceAlias(ctx context.Context, tx pgx.Tx, personID uuid.UUID, verbatim string, sourceID uuid.UUID) (*uuid.UUID, bool, error) {
	return writeSourceAliasType(ctx, tx, personID, verbatim, "source_spelling", sourceID)
}

func writeSourceAliasType(ctx context.Context, tx pgx.Tx, personID uuid.UUID, verbatim, aliasType string, sourceID uuid.UUID) (*uuid.UUID, bool, error) {
	normalized := identity.NormalizeArabicName(verbatim)
	if strings.TrimSpace(normalized) == "" {
		return nil, false, nil
	}
	var aliasID uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO person_aliases (person_id, value_ar, normalized_value_ar, alias_type, source_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (person_id, normalized_value_ar, alias_type) DO NOTHING
		RETURNING id
	`, personID, verbatim, normalized, aliasType, sourceID).Scan(&aliasID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &aliasID, true, nil
}

// NodeByDraftID finds a node plan by its draft node id.
func (p *treeImportPlan) NodeByDraftID(id string) *treeImportNodePlan {
	for index := range p.Nodes {
		if p.Nodes[index].Node.ID == id {
			return &p.Nodes[index]
		}
	}
	return nil
}

// matchExistingPerson is the deterministic matcher. It is the same vocabulary
// the extraction path uses: the normalised name against people and person
// aliases, exact equality. An alias hit scores lower than a canonical hit but
// both clear the shared threshold.
func matchExistingPerson(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, normalized string) (existingPerson, bool, error) {
	var match existingPerson
	if strings.TrimSpace(normalized) == "" {
		return match, false, nil
	}
	err := q.QueryRow(ctx, `
		SELECT p.id, p.canonical_name_ar, 'canonical', $2::double precision
		FROM people p WHERE p.normalized_name_ar = $1
		ORDER BY (p.canonical_name_ar = $1) DESC, p.created_at, p.id
		LIMIT 1
	`, normalized, treeImportExactScore).Scan(&match.ID, &match.Name, &match.Basis, &match.Score)
	if errors.Is(err, pgx.ErrNoRows) {
		err = q.QueryRow(ctx, `
			SELECT p.id, p.canonical_name_ar, 'alias', $2::double precision
			FROM person_aliases a JOIN people p ON p.id = a.person_id
			WHERE a.normalized_value_ar = $1
			ORDER BY (p.canonical_name_ar = $1) DESC, p.created_at, p.id
			LIMIT 1
		`, normalized, treeImportAliasScore).Scan(&match.ID, &match.Name, &match.Basis, &match.Score)
	}
	switch {
	case err == nil:
		return match, true, nil
	case errors.Is(err, pgx.ErrNoRows):
		return match, false, nil
	default:
		return existingPerson{}, false, err
	}
}

// fuzzyPersonMatches proposes people a bare trigram similarity puts in range.
// It is a *proposal* for the resolution surface: the import creates the new
// person and proposes them against these, and a human decides. The lookup is
// bounded (per occurrence and per import) so a 60-person import does not open
// candidates against the whole database.
func fuzzyPersonMatches(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, normalized string) ([]treeCandidate, error) {
	if strings.TrimSpace(normalized) == "" {
		return nil, nil
	}
	rows, err := q.Query(ctx, `
		SELECT refs.id, refs.name, refs.score FROM (
			SELECT p.id, p.canonical_name_ar AS name, similarity(p.normalized_name_ar, $1) AS score
			FROM people p WHERE p.normalized_name_ar <> $1
			UNION ALL
			SELECT p.id, p.canonical_name_ar, similarity(a.normalized_value_ar, $1)
			FROM person_aliases a JOIN people p ON p.id = a.person_id
			WHERE a.normalized_value_ar <> $1
		) refs
		WHERE refs.score >= $2
		ORDER BY refs.score DESC, refs.name, refs.id
		LIMIT $3
	`, normalized, treeImportFuzzyFloor, treeImportFuzzyPerOccurrence)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := make(map[uuid.UUID]bool)
	proposals := make([]treeCandidate, 0, treeImportFuzzyPerOccurrence)
	for rows.Next() {
		var candidate treeCandidate
		var id uuid.UUID
		var name string
		var score float64
		if err := rows.Scan(&id, &name, &score); err != nil {
			return nil, err
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		candidate.ExistingID = id
		candidate.ExistingName = name
		candidate.Score = score
		proposals = append(proposals, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return proposals, nil
}

// treeImportPair is one proposed (created person, existing person) pair.
type treeImportPair struct {
	left, right         uuid.UUID
	leftName, rightName string
	score               float64
}

// writeImportResolutionCandidates opens the import's own resolution run and
// writes every fuzzy proposal into it, in the same transaction as the tree.
// The candidate rows use the same shape the sweep writes, so the existing
// review surface lists and acts on them without a parallel surface.
func writeImportResolutionCandidates(ctx context.Context, tx pgx.Tx, actorUUID uuid.UUID, plan *treeImportPlan) (string, int, error) {
	pairs := make([]struct {
		left, right         uuid.UUID
		leftName, rightName string
		score               float64
	}, 0, plan.Candidates)
	for index := range plan.Nodes {
		entry := &plan.Nodes[index]
		if !entry.New {
			continue
		}
		for _, fuzzy := range entry.Fuzzy {
			left, right := entry.PersonID, fuzzy.ExistingID
			leftName, rightName := entry.Node.NameAR, fuzzy.ExistingName
			if left.String() > right.String() {
				left, right = right, left
				leftName, rightName = rightName, leftName
			}
			pair := struct {
				left, right         uuid.UUID
				leftName, rightName string
				score               float64
			}{left, right, leftName, rightName, fuzzy.Score}
			duplicate := false
			for _, existing := range pairs {
				if existing.left == left && existing.right == right {
					duplicate = true
					break
				}
			}
			if !duplicate {
				pairs = append(pairs, pair)
			}
		}
	}
	if len(pairs) == 0 {
		return "", 0, nil
	}
	var runID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO entity_resolution_runs (
			requested_by, entity_type, status, stage, algorithm_version, normalization_version, candidate_count,
			started_at, completed_at
		)
		VALUES ($1, 'person', 'succeeded', 'complete', $2, $3, $4, now(), now())
		RETURNING id
	`, actorUUID, treeImportAlgorithmVersion, treeImportNormalizationVersion, len(pairs)).Scan(&runID); err != nil {
		return "", 0, err
	}
	written := 0
	for _, pair := range pairs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO entity_resolution_candidates (
				run_id, entity_type, left_entity_id, right_entity_id, left_name_ar, right_name_ar,
				match_class, score, score_components, matching_signals, conflicting_signals, explanation_ar
			)
			VALUES ($1, 'person', $2, $3, $4, $5, 'possible_match', $6, $7, '["normalized_name_trigram"]'::jsonb, '[]'::jsonb, $8)
			ON CONFLICT (run_id, entity_type, left_entity_id, right_entity_id) DO NOTHING
		`, runID, pair.left, pair.right, pair.leftName, pair.rightName, pair.score,
			mustJSON(map[string]float64{"normalized_name_trigram": pair.score}),
			"اقتراح ربط آلي في استيراد الشجرة على أساس التشابه الحرفي فقط؛ لا يُدمج شيء إلا بقرار شخص."); err != nil {
			return "", 0, err
		}
		written++
	}
	return runID.String(), written, nil
}

// extractKunyah is the bounded classical-component helper. It records a kunyah
// only when the reading opens with أبو/أم and carries a name the kunyah owns —
// including the عبد compound (أبو عبد الله). It never rewrites the canonical
// name: the split must stay reviewable and correctable through the alias rows.
func extractKunyah(verbatim string) string {
	tokens := strings.Fields(strings.TrimSpace(verbatim))
	if len(tokens) < 2 || (tokens[0] != "أبو" && tokens[0] != "أم") {
		return ""
	}
	second := tokens[1]
	for _, prefix := range []string{"بن", "بنت", "من", "و"} {
		if strings.HasPrefix(second, prefix) {
			return ""
		}
	}
	if second == "عبد" && len(tokens) >= 3 {
		second = "عبد " + tokens[2]
	}
	return tokens[0] + " " + second
}
