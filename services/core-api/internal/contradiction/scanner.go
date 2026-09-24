package contradiction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type personFact struct {
	ID         uuid.UUID
	Name       string
	Normalized string
	Aliases    []string
	BirthFrom  pgtype.Date
	BirthTo    pgtype.Date
	DeathFrom  pgtype.Date
	DeathTo    pgtype.Date
	Trees      map[string]struct{}
}

type claimFact struct {
	ID        uuid.UUID
	SubjectID uuid.UUID
	ObjectID  uuid.UUID
	Predicate string
	TimeFrom  pgtype.Date
	TimeTo    pgtype.Date
}

type relationFact struct {
	ChildID  uuid.UUID
	ParentID uuid.UUID
	ClaimIDs []string
}

type eventFact struct {
	ID       string
	Kind     string
	PersonID uuid.UUID
	From     pgtype.Date
	To       pgtype.Date
	Detail   string
}

type treeEdge struct {
	ID        uuid.UUID
	VersionID uuid.UUID
	ParentID  uuid.UUID
	ChildID   uuid.UUID
}

func (s *Service) Process(ctx context.Context, job jobs.JobView) error {
	var payload jobPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return fmt.Errorf("decode contradiction job: %w", err)
	}
	runID, err := uuid.Parse(strings.TrimSpace(payload.RunID))
	if err != nil {
		return fmt.Errorf("invalid contradiction run: %w", err)
	}
	if err := s.ready(); err != nil {
		return err
	}
	if _, err := s.Pool.Exec(ctx, `UPDATE contradiction_runs SET status = 'running', started_at = COALESCE(started_at, now()), error = NULL, updated_at = now() WHERE id = $1`, runID); err != nil {
		return err
	}
	findings, err := s.scan(ctx)
	if err != nil {
		_, _ = s.Pool.Exec(ctx, `UPDATE contradiction_runs SET status = 'failed', error = $1, completed_at = now(), updated_at = now() WHERE id = $2`, safeError(err), runID)
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var requestedBy string
	if err := tx.QueryRow(ctx, `SELECT requested_by::text FROM contradiction_runs WHERE id = $1`, runID).Scan(&requestedBy); err != nil {
		return err
	}
	for _, finding := range findings {
		var findingID uuid.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO platform_findings (run_id, finding_type, title_ar, explanation_ar, status, severity, signals, algorithm_version, check_key, created_by)
			VALUES ($1, $2, $3, $4, 'needs_review', $5, $6, $7, $8, $9)
			ON CONFLICT DO NOTHING
			RETURNING id
		`, runID, finding.Type, finding.TitleAR, finding.ExplanationAR, finding.Severity, mustJSON(finding.Signals), AlgorithmVersion, finding.Key, requestedBy).Scan(&findingID)
		if errors.Is(err, pgx.ErrNoRows) {
			if err := tx.QueryRow(ctx, `SELECT id FROM platform_findings WHERE run_id = $1 AND check_key = $2`, runID, finding.Key).Scan(&findingID); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		for _, claimID := range finding.ClaimIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO finding_claims (finding_id, claim_id, relation) VALUES ($1, $2, 'concerns') ON CONFLICT DO NOTHING`, findingID, claimID); err != nil {
				return err
			}
		}
		for _, entityID := range finding.EntityIDs {
			if _, err := tx.Exec(ctx, `INSERT INTO finding_entities (finding_id, entity_type, entity_id) VALUES ($1, 'person', $2) ON CONFLICT DO NOTHING`, findingID, entityID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE contradiction_runs SET status = 'succeeded', finding_count = $1, completed_at = now(), updated_at = now() WHERE id = $2`, len(findings), runID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) RequeueRecovered(ctx context.Context, recovered []jobs.JobView) error {
	for _, job := range recovered {
		if job.Type != JobType {
			continue
		}
		var payload jobPayload
		if json.Unmarshal(job.Payload, &payload) != nil {
			continue
		}
		if runID, err := uuid.Parse(payload.RunID); err == nil {
			_, _ = s.Pool.Exec(ctx, `UPDATE contradiction_runs SET status = 'queued', error = 'worker lock expired', updated_at = now() WHERE id = $1 AND status = 'running'`, runID)
		}
	}
	return nil
}

func (s *Service) scan(ctx context.Context) ([]findingInput, error) {
	people, byID, err := s.loadPeople(ctx)
	if err != nil {
		return nil, err
	}
	claims, err := s.loadClaims(ctx)
	if err != nil {
		return nil, err
	}
	relations, err := s.loadRelations(ctx, claims)
	if err != nil {
		return nil, err
	}
	events, err := s.loadEvents(ctx)
	if err != nil {
		return nil, err
	}
	edges, err := s.loadTreeEdges(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]findingInput, 0)
	seen := make(map[string]struct{})
	add := func(value findingInput) {
		if _, exists := seen[value.Key]; exists {
			return
		}
		seen[value.Key] = struct{}{}
		sort.Strings(value.ClaimIDs)
		sort.Strings(value.EntityIDs)
		result = append(result, value)
	}
	fathers := make(map[uuid.UUID]map[uuid.UUID][]string)
	for _, relation := range relations {
		if fathers[relation.ChildID] == nil {
			fathers[relation.ChildID] = make(map[uuid.UUID][]string)
		}
		fathers[relation.ChildID][relation.ParentID] = append(fathers[relation.ChildID][relation.ParentID], relation.ClaimIDs...)
	}
	for childID, parentMap := range fathers {
		if len(parentMap) < 2 {
			continue
		}
		parentIDs := make([]string, 0, len(parentMap))
		claimIDs := make([]string, 0)
		for parentID, ids := range parentMap {
			parentIDs = append(parentIDs, parentID.String())
			claimIDs = append(claimIDs, ids...)
		}
		sort.Strings(parentIDs)
		add(findingInput{Key: "multiple_fathers:" + childID.String() + ":" + strings.Join(parentIDs, ","), Type: "multiple_incompatible_fathers", TitleAR: "أكثر من أب محتمل للشخص نفسه", ExplanationAR: "تظهر روابط قرابة تعطي أكثر من أب محتمل للشخص نفسه؛ يلزم فحص المصادر قبل حسم العلاقة.", Severity: "high", Signals: map[string]any{"child_id": childID.String(), "parent_ids": parentIDs}, ClaimIDs: uniqueStrings(claimIDs), EntityIDs: append([]string{childID.String()}, parentIDs...)})
	}
	for _, relation := range relations {
		child, childOK := byID[relation.ChildID]
		parent, parentOK := byID[relation.ParentID]
		if !childOK || !parentOK {
			continue
		}
		if datesAfter(parent.BirthFrom, parent.BirthTo, child.BirthFrom, child.BirthTo) {
			add(findingInput{Key: "parent_born_after_child:" + relation.ParentID.String() + ":" + relation.ChildID.String(), Type: "parent_born_after_child", TitleAR: "ميلاد الأب بعد ميلاد الابن", ExplanationAR: "النطاق الزمني المسجل للأب يبدو متأخراً عن النطاق الزمني للابن.", Severity: "high", Signals: map[string]any{"parent_id": relation.ParentID.String(), "child_id": relation.ChildID.String()}, ClaimIDs: relation.ClaimIDs, EntityIDs: []string{relation.ParentID.String(), relation.ChildID.String()}})
		}
		if deathBeforeBirth(parent.DeathFrom, parent.DeathTo, child.BirthFrom, child.BirthTo) {
			add(findingInput{Key: "parent_died_before_child_born:" + relation.ParentID.String() + ":" + relation.ChildID.String(), Type: "parent_died_too_early", TitleAR: "وفاة الأب قبل ميلاد الابن", ExplanationAR: "النطاق الزمني المسجل لوفاة الأب يبدو سابقاً لميلاد الابن.", Severity: "high", Signals: map[string]any{"parent_id": relation.ParentID.String(), "child_id": relation.ChildID.String()}, ClaimIDs: relation.ClaimIDs, EntityIDs: []string{relation.ParentID.String(), relation.ChildID.String()}})
		}
	}
	for _, event := range events {
		person, ok := byID[event.PersonID]
		if !ok {
			continue
		}
		if eventBeforeBirth(event, person) {
			add(findingInput{Key: "event_before_birth:" + event.ID, Type: "event_before_birth", TitleAR: "حدث قبل تاريخ الميلاد", ExplanationAR: "يظهر حدث مسجل قبل بداية نطاق ميلاد الشخص.", Severity: "high", Signals: map[string]any{"event_id": event.ID, "event_kind": event.Kind, "detail": event.Detail}, EntityIDs: []string{person.ID.String()}})
		}
		if eventAfterDeath(event, person) {
			add(findingInput{Key: "event_after_death:" + event.ID, Type: "event_after_death", TitleAR: "حدث بعد تاريخ الوفاة", ExplanationAR: "يظهر حدث مسجل بعد نهاية نطاق وفاة الشخص.", Severity: "high", Signals: map[string]any{"event_id": event.ID, "event_kind": event.Kind, "detail": event.Detail}, EntityIDs: []string{person.ID.String()}})
		}
	}
	addDuplicateIdentityFindings(people, add)
	addLoopFindings(edges, add)
	return result, nil
}

func (s *Service) loadPeople(ctx context.Context) (map[string]personFact, map[uuid.UUID]personFact, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, canonical_name_ar, normalized_name_ar, birth_date_from, birth_date_to, death_date_from, death_date_to FROM people WHERE merged_into_id IS NULL ORDER BY id LIMIT 5000`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	byID := make(map[uuid.UUID]personFact)
	byString := make(map[string]personFact)
	for rows.Next() {
		var id uuid.UUID
		var name, normalized string
		var birthFrom, birthTo, deathFrom, deathTo pgtype.Date
		if err := rows.Scan(&id, &name, &normalized, &birthFrom, &birthTo, &deathFrom, &deathTo); err != nil {
			return nil, nil, err
		}
		value := personFact{ID: id, Name: name, Normalized: normalized, BirthFrom: birthFrom, BirthTo: birthTo, DeathFrom: deathFrom, DeathTo: deathTo, Trees: make(map[string]struct{})}
		byID[id] = value
		byString[id.String()] = value
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	aliasRows, err := s.Pool.Query(ctx, `SELECT person_id, normalized_value_ar FROM person_aliases ORDER BY person_id`)
	if err != nil {
		return nil, nil, err
	}
	for aliasRows.Next() {
		var id uuid.UUID
		var alias string
		if err := aliasRows.Scan(&id, &alias); err != nil {
			aliasRows.Close()
			return nil, nil, err
		}
		if value, ok := byID[id]; ok {
			value.Aliases = append(value.Aliases, alias)
			byID[id] = value
		}
	}
	aliasRows.Close()
	treeRows, err := s.Pool.Query(ctx, `SELECT DISTINCT tn.person_id, tv.tree_id FROM tree_nodes tn JOIN tree_versions tv ON tv.id = tn.tree_version_id ORDER BY tn.person_id, tv.tree_id`)
	if err != nil {
		return nil, nil, err
	}
	for treeRows.Next() {
		var personID, treeID uuid.UUID
		if err := treeRows.Scan(&personID, &treeID); err != nil {
			treeRows.Close()
			return nil, nil, err
		}
		if value, ok := byID[personID]; ok {
			value.Trees[treeID.String()] = struct{}{}
			byID[personID] = value
		}
	}
	treeRows.Close()
	for id, value := range byID {
		byString[id.String()] = value
	}
	return byString, byID, nil
}

func (s *Service) loadClaims(ctx context.Context) ([]claimFact, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, subject_id, object_id, predicate, time_from, time_to FROM claims WHERE subject_type = 'person' AND object_type = 'person' ORDER BY id LIMIT 20000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]claimFact, 0)
	for rows.Next() {
		var value claimFact
		if err := rows.Scan(&value.ID, &value.SubjectID, &value.ObjectID, &value.Predicate, &value.TimeFrom, &value.TimeTo); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Service) loadRelations(ctx context.Context, claims []claimFact) ([]relationFact, error) {
	result := make([]relationFact, 0)
	for _, claim := range claims {
		predicate := strings.ToLower(strings.TrimSpace(claim.Predicate))
		var childID, parentID uuid.UUID
		switch predicate {
		case "father_of":
			childID, parentID = claim.SubjectID, claim.ObjectID
		case "parent_of":
			parentID, childID = claim.SubjectID, claim.ObjectID
		default:
			continue
		}
		result = append(result, relationFact{ChildID: childID, ParentID: parentID, ClaimIDs: []string{claim.ID.String()}})
	}
	rows, err := s.Pool.Query(ctx, `SELECT subject_id, predicate, object_id FROM entity_relationships WHERE subject_type = 'person' AND object_type = 'person' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var subjectID, objectID uuid.UUID
		var predicate string
		if err := rows.Scan(&subjectID, &predicate, &objectID); err != nil {
			rows.Close()
			return nil, err
		}
		var childID, parentID uuid.UUID
		switch strings.ToLower(strings.TrimSpace(predicate)) {
		case "father_of":
			childID, parentID = subjectID, objectID
		case "parent_of":
			parentID, childID = subjectID, objectID
		default:
			continue
		}
		result = append(result, relationFact{ChildID: childID, ParentID: parentID})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return result, nil
}

func (s *Service) loadEvents(ctx context.Context) ([]eventFact, error) {
	result := make([]eventFact, 0)
	geoRows, err := s.Pool.Query(ctx, `SELECT id::text, entity_id, time_from, time_to, relation_type FROM geographic_associations WHERE entity_type = 'person' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for geoRows.Next() {
		var id, personID string
		var from, to pgtype.Date
		var detail string
		if err := geoRows.Scan(&id, &personID, &from, &to, &detail); err != nil {
			geoRows.Close()
			return nil, err
		}
		parsed, parseErr := uuid.Parse(personID)
		if parseErr == nil {
			result = append(result, eventFact{ID: "geographic:" + id, Kind: "geographic_association", PersonID: parsed, From: from, To: to, Detail: detail})
		}
	}
	if err := geoRows.Err(); err != nil {
		geoRows.Close()
		return nil, err
	}
	geoRows.Close()
	migrationRows, err := s.Pool.Query(ctx, `SELECT id::text, subject_id, time_from, time_to, notes_ar FROM migration_events WHERE subject_type = 'person' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	for migrationRows.Next() {
		var id string
		var personID uuid.UUID
		var from, to pgtype.Date
		var detail pgtype.Text
		if err := migrationRows.Scan(&id, &personID, &from, &to, &detail); err != nil {
			migrationRows.Close()
			return nil, err
		}
		result = append(result, eventFact{ID: "migration:" + id, Kind: "migration", PersonID: personID, From: from, To: to, Detail: textValue(detail)})
	}
	if err := migrationRows.Err(); err != nil {
		migrationRows.Close()
		return nil, err
	}
	migrationRows.Close()
	return result, nil
}

func (s *Service) loadTreeEdges(ctx context.Context) ([]treeEdge, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT tr.id, tr.tree_version_id, sn.person_id, object_node.person_id
		FROM tree_relationships tr
		JOIN tree_versions tv ON tv.id = tr.tree_version_id AND tv.state = 'published'
		JOIN tree_nodes sn ON sn.id = tr.subject_node_id
		JOIN tree_nodes object_node ON object_node.id = tr.object_node_id
		ORDER BY tr.tree_version_id, tr.id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]treeEdge, 0)
	for rows.Next() {
		var value treeEdge
		if err := rows.Scan(&value.ID, &value.VersionID, &value.ParentID, &value.ChildID); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func datesAfter(parentFrom, parentTo, childFrom, childTo pgtype.Date) bool {
	if !parentFrom.Valid || !childTo.Valid {
		return false
	}
	return parentFrom.Time.After(childTo.Time)
}

func deathBeforeBirth(deathFrom, deathTo, birthFrom, birthTo pgtype.Date) bool {
	if !deathFrom.Valid || !birthFrom.Valid {
		return false
	}
	return deathFrom.Time.Before(birthFrom.Time)
}

func eventBeforeBirth(event eventFact, person personFact) bool {
	return event.To.Valid && person.BirthFrom.Valid && event.To.Time.Before(person.BirthFrom.Time)
}

func eventAfterDeath(event eventFact, person personFact) bool {
	return event.From.Valid && person.DeathTo.Valid && event.From.Time.After(person.DeathTo.Time)
}

func addDuplicateIdentityFindings(people map[string]personFact, add func(findingInput)) {
	byTree := make(map[string][]string)
	for _, person := range people {
		values := append([]string{person.Normalized}, person.Aliases...)
		for treeID := range person.Trees {
			for _, value := range values {
				value = identity.NormalizeArabicName(value)
				if value == "" {
					continue
				}
				key := treeID + "|" + value
				byTree[key] = append(byTree[key], person.ID.String())
			}
		}
	}
	for key, ids := range byTree {
		ids = uniqueStrings(ids)
		if len(ids) < 2 {
			continue
		}
		parts := strings.SplitN(key, "|", 2)
		add(findingInput{Key: "duplicate_identity_in_branch:" + parts[0] + ":" + strings.Join(ids, ","), Type: "duplicate_identity_in_branch", TitleAR: "هوية مكررة داخل فرع واحد", ExplanationAR: "تظهر أكثر من سجلات شخص بنفس الاسم أو اللقب داخل فرع واحد؛ يلزم فحص الهوية قبل الدمج.", Severity: "medium", Signals: map[string]any{"tree_id": parts[0], "name": parts[1], "person_ids": ids}, EntityIDs: ids})
	}
}

func addLoopFindings(edges []treeEdge, add func(findingInput)) {
	byVersion := make(map[uuid.UUID][]treeEdge)
	for _, edge := range edges {
		byVersion[edge.VersionID] = append(byVersion[edge.VersionID], edge)
	}
	for versionID, versionEdges := range byVersion {
		adjacency := make(map[uuid.UUID][]uuid.UUID)
		for _, edge := range versionEdges {
			adjacency[edge.ParentID] = append(adjacency[edge.ParentID], edge.ChildID)
		}
		state := make(map[uuid.UUID]int)
		stack := make([]uuid.UUID, 0)
		cycleSeen := make(map[string]struct{})
		var visit func(uuid.UUID)
		visit = func(node uuid.UUID) {
			state[node] = 1
			stack = append(stack, node)
			for _, child := range adjacency[node] {
				if state[child] == 0 {
					visit(child)
				} else if state[child] == 1 {
					start := 0
					for index, value := range stack {
						if value == child {
							start = index
							break
						}
					}
					cycleNodes := append([]uuid.UUID{}, stack[start:]...)
					cycle := make([]string, len(cycleNodes))
					for index, value := range cycleNodes {
						cycle[index] = value.String()
					}
					sort.Strings(cycle)
					key := "relationship_loop:" + versionID.String() + ":" + strings.Join(cycle, ",")
					if _, exists := cycleSeen[key]; !exists {
						cycleSeen[key] = struct{}{}
						ids := append([]string{}, cycle...)
						add(findingInput{Key: key, Type: "impossible_relationship_loop", TitleAR: "حلقة قرابة غير منطقية", ExplanationAR: "يوجد مسار علاقة دوري في الشجرة المنشورة؛ لا يجب تعديل العلاقة تلقائياً قبل المراجعة.", Severity: "high", Signals: map[string]any{"tree_version_id": versionID.String(), "cycle": ids}, EntityIDs: ids})
					}
				}
			}
			stack = stack[:len(stack)-1]
			state[node] = 2
		}
		for node := range adjacency {
			if state[node] == 0 {
				visit(node)
			}
		}
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	value := []rune(err.Error())
	if len(value) > 4000 {
		return string(value[:4000])
	}
	return string(value)
}
