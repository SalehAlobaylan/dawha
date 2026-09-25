package researchagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type stageContext struct {
	Input       RunInput
	ActorUUID   uuid.UUID
	Terms       []string
	TreeID      string
	TreeVersion string
	SourceIDs   []uuid.UUID
}

func decomposeQuestion(input RunInput) ([]PlanStep, []string) {
	normalized := identity.NormalizeArabicName(input.Question)
	terms := make([]string, 0, MaximumTerms)
	seen := make(map[string]struct{})
	for _, term := range strings.Fields(normalized) {
		term = strings.TrimSpace(term)
		if len([]rune(term)) < 3 {
			continue
		}
		if _, found := seen[term]; found {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
		if len(terms) == MaximumTerms {
			break
		}
	}
	plan := []PlanStep{
		{Order: 1, Stage: StageDecompose, Tool: "deterministic_question_decomposition", ReadOnly: true, DescriptionAR: "تحديد المصطلحات والكيانات والنطاق قبل جمع الأدلة."},
		{Order: 2, Stage: StageSearchSources, Tool: "qualified_source_search", ReadOnly: true, DescriptionAR: "البحث في العبارات المقبولة من المصادر العامة المستقلة."},
		{Order: 3, Stage: StageSearchGraph, Tool: "bounded_graph_inspection", ReadOnly: true, DescriptionAR: "فحص العلاقات البنيوية في نسخة الشجرة المحددة."},
		{Order: 4, Stage: StageInspectGeography, Tool: "qualified_geography_inspection", ReadOnly: true, DescriptionAR: "فحص الحضور والهجرة والمواضع المرتبطة."},
		{Order: 5, Stage: StageInspectChronology, Tool: "bounded_chronology_inspection", ReadOnly: true, DescriptionAR: "مقارنة النطاقات الزمنية والتسلسل الزمني المحفوظ."},
		{Order: 6, Stage: StageCompareClaims, Tool: "claim_comparison", ReadOnly: true, DescriptionAR: "مقارنة الادعاءات والحالات المختلفة."},
		{Order: 7, Stage: StageSourceDependency, Tool: "source_dependency_inspection", ReadOnly: true, DescriptionAR: "فحص العلاقات بين المصادر قبل عدّها أدلة مستقلة."},
		{Order: 8, Stage: StageCounterEvidence, Tool: "counter_evidence_retrieval", ReadOnly: true, DescriptionAR: "استرجاع الأدلة المضادة أو السياقية بشكل صريح."},
		{Order: 9, Stage: StageEvidencePackage, Tool: "traceable_evidence_package", ReadOnly: true, DescriptionAR: "تجميع حزمة أدلة قابلة للتتبع."},
		{Order: 10, Stage: StageMissingEvidence, Tool: "gap_analysis", ReadOnly: true, DescriptionAR: "تحديد الفجوات والحدود التي تمنع حسم السؤال."},
		{Order: 11, Stage: StageRecommendation, Tool: "bounded_next_investigation", ReadOnly: true, DescriptionAR: "اقتراح خطوات البحث التالية دون تنفيذ تغييرات."},
	}
	return plan, terms
}

func searchSources(ctx context.Context, q queryer, stage stageContext) ([]EvidenceRef, error) {
	rows, err := q.Query(ctx, `
		SELECT ss.id, ss.source_id, s.title_ar, ss.source_passage_id, ss.statement_text_ar
		FROM source_statements ss
		JOIN sources s ON s.id = ss.source_id
		WHERE ss.review_status = 'accepted'
		  AND s.visibility = 'public'
		  AND s.dependency_status = 'independent'
		  AND NOT EXISTS (
			SELECT 1 FROM source_dependencies sd
			WHERE sd.status IN ('needs_review', 'confirmed')
			  AND sd.source_id = s.id
		  )
		  AND ($1 = '' OR s.id = $1::uuid)
		  AND (COALESCE(cardinality($2::uuid[]), 0) = 0 OR s.id = ANY($2::uuid[]))
		  AND (COALESCE(cardinality($3::text[]), 0) = 0 OR EXISTS (
			SELECT 1 FROM unnest($3::text[]) AS term(value)
			WHERE ss.statement_text_ar ILIKE '%' || term.value || '%' OR s.title_ar ILIKE '%' || term.value || '%'
		  ))
		  AND (
			COALESCE($4::text, '') = ''
			OR NOT EXISTS (SELECT 1 FROM question_sources qs WHERE qs.question_id = $4::uuid)
			OR EXISTS (SELECT 1 FROM question_sources qs WHERE qs.question_id = $4::uuid AND qs.source_id = s.id)
			OR EXISTS (
				SELECT 1 FROM question_claims qc
				JOIN claim_evidence ce ON ce.claim_id = qc.claim_id
				WHERE qc.question_id = $4::uuid AND ce.source_statement_id = ss.id
			)
		  )
		ORDER BY ss.created_at DESC, ss.id
		LIMIT 30
	`, nullableUUIDValue(stage.Input.SourceID), stage.SourceIDs, stage.Terms, nullableUUIDValue(stage.Input.QuestionID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EvidenceRef, 0)
	for rows.Next() {
		var id, sourceID, passageID pgtype.UUID
		var title, excerpt pgtype.Text
		if err := rows.Scan(&id, &sourceID, &title, &passageID, &excerpt); err != nil {
			return nil, err
		}
		items = append(items, EvidenceRef{Layer: "source_statement", Stance: "supports", ReferenceType: "source_statement", ReferenceID: uuidString(id), SourceID: uuidString(sourceID), StatementID: uuidString(id), Excerpt: textValue(excerpt), Metadata: map[string]any{"title": textValue(title), "passageId": uuidString(passageID), "reviewStatus": "accepted"}})
	}
	return items, rows.Err()
}

func searchGraph(ctx context.Context, q queryer, stage stageContext) ([]EvidenceRef, error) {
	if stage.TreeVersion == "" || stage.Input.EntityType != "person" {
		return []EvidenceRef{}, nil
	}
	targetID := ""
	if stage.Input.EntityType == "person" {
		targetID = stage.Input.EntityID
	}
	rows, err := q.Query(ctx, `
		SELECT tr.id, sn.person_id::text, object_node.person_id::text, sn.display_name_ar, object_node.display_name_ar,
		       tr.predicate, tr.status,
		       CASE WHEN s.id IS NULL OR (s.visibility = 'public' AND s.dependency_status = 'independent') THEN COALESCE(tr.source_id::text, '') ELSE '' END,
		       CASE WHEN s.id IS NULL OR s.visibility = 'public' THEN COALESCE(s.title_ar, '') ELSE '' END
		FROM tree_relationships tr
		JOIN tree_versions tv ON tv.id = tr.tree_version_id
		JOIN tree_nodes sn ON sn.id = tr.subject_node_id
		JOIN tree_nodes object_node ON object_node.id = tr.object_node_id
		LEFT JOIN sources s ON s.id = tr.source_id
		WHERE tr.tree_version_id = $1
		  AND tr.predicate IN ('parent_of', 'spouse_of', 'sibling_of')
		  AND ($2 = '' OR sn.person_id::text = $2 OR object_node.person_id::text = $2)
		ORDER BY tr.created_at, tr.id
		LIMIT 50
	`, stage.TreeVersion, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EvidenceRef, 0)
	for rows.Next() {
		var id pgtype.UUID
		var subjectID, objectID, sourceID, title string
		var subjectName, objectName, predicate, status pgtype.Text
		if err := rows.Scan(&id, &subjectID, &objectID, &subjectName, &objectName, &predicate, &status, &sourceID, &title); err != nil {
			return nil, err
		}
		items = append(items, EvidenceRef{Layer: "tree_interpretation", Stance: "context", ReferenceType: "tree_relationship", ReferenceID: uuidString(id), SourceID: sourceID, Excerpt: fmt.Sprintf("%s (%s) — %s", textValue(subjectName), textValue(predicate), textValue(objectName)), Metadata: map[string]any{"subjectId": subjectID, "objectId": objectID, "status": textValue(status), "title": title, "treeVersionId": stage.TreeVersion}})
	}
	return items, rows.Err()
}

func inspectGeography(ctx context.Context, q queryer, stage stageContext) ([]EvidenceRef, error) {
	rows, err := q.Query(ctx, `
		SELECT 'association', ga.id::text, COALESCE(p.canonical_name_ar, ''), ga.relation_type, ga.status,
		       COALESCE(ga.time_from::text, ''), COALESCE(ga.time_to::text, ''), COALESCE(ga.source_id::text, ''),
		       COALESCE(s.title_ar, ''), COALESCE(ga.entity_id::text, '')
		FROM geographic_associations ga
		JOIN places p ON p.id = ga.place_id
		LEFT JOIN sources s ON s.id = ga.source_id
		WHERE (($1 IN ('person', 'family', 'branch') AND ga.entity_type = $1 AND ga.entity_id = $2)
		   OR ($1 = 'source' AND ga.source_id = $2)
		   OR ($1 = 'place' AND ga.place_id = $2))
		  AND s.id IS NOT NULL AND s.visibility = 'public' AND s.dependency_status = 'independent'
		  AND NOT EXISTS (
			SELECT 1 FROM source_dependencies sd
			WHERE sd.status IN ('needs_review', 'confirmed')
			  AND sd.source_id = s.id
		  )
		UNION ALL
		SELECT 'migration', m.id::text, COALESCE(pf.canonical_name_ar, pt.canonical_name_ar, ''), 'migration', m.status,
		       COALESCE(m.time_from::text, ''), COALESCE(m.time_to::text, ''), COALESCE(m.source_id::text, ''),
		       COALESCE(s.title_ar, ''), COALESCE(m.subject_id::text, '')
		FROM migration_events m
		LEFT JOIN places pf ON pf.id = m.from_place_id
		LEFT JOIN places pt ON pt.id = m.to_place_id
		LEFT JOIN sources s ON s.id = m.source_id
		WHERE (($1 IN ('person', 'family', 'branch') AND m.subject_type = $1 AND m.subject_id = $2)
		   OR ($1 = 'source' AND m.source_id = $2)
		   OR ($1 = 'place' AND (m.from_place_id = $2 OR m.to_place_id = $2)))
		  AND s.id IS NOT NULL AND s.visibility = 'public' AND s.dependency_status = 'independent'
		  AND NOT EXISTS (
			SELECT 1 FROM source_dependencies sd
			WHERE sd.status IN ('needs_review', 'confirmed')
			  AND sd.source_id = s.id
		  )
		ORDER BY 1, 6
		LIMIT 50
	`, stage.Input.EntityType, stage.Input.EntityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EvidenceRef, 0)
	for rows.Next() {
		var kind, id, placeName, relation, status, timeFrom, timeTo, sourceID, sourceTitle, entityID string
		if err := rows.Scan(&kind, &id, &placeName, &relation, &status, &timeFrom, &timeTo, &sourceID, &sourceTitle, &entityID); err != nil {
			return nil, err
		}
		stance := "context"
		if status == "platform_inferred" {
			stance = "hypothesis"
		}
		items = append(items, EvidenceRef{Layer: "geographic_signal", Stance: stance, ReferenceType: kind, ReferenceID: id, SourceID: sourceID, Excerpt: fmt.Sprintf("%s · %s · %s", placeName, relation, status), Metadata: map[string]any{"timeFrom": timeFrom, "timeTo": timeTo, "entityId": entityID, "title": sourceTitle, "status": status}})
	}
	return items, rows.Err()
}

func inspectChronology(ctx context.Context, q queryer, stage stageContext) ([]EvidenceRef, error) {
	items := make([]EvidenceRef, 0)
	if stage.Input.EntityType == "person" {
		var id, treeVersion pgtype.UUID
		var status, reportStatus pgtype.Text
		var reportData []byte
		err := q.QueryRow(ctx, `SELECT id, tree_version_id, status, report_status, reference_population FROM temporal_analysis_runs WHERE target_person_id = $1 AND ($2 = '' OR tree_version_id = $2::uuid) ORDER BY created_at DESC LIMIT 1`, stage.Input.EntityID, stage.TreeVersion).Scan(&id, &treeVersion, &status, &reportStatus, &reportData)
		if err == nil {
			items = append(items, EvidenceRef{Layer: "temporal_signal", Stance: "hypothesis", ReferenceType: "temporal_run", ReferenceID: uuidString(id), Excerpt: "تحليل زمني محفوظ: " + textValue(reportStatus), Metadata: map[string]any{"status": textValue(status), "reportStatus": textValue(reportStatus), "treeVersionId": uuidString(treeVersion), "report": json.RawMessage(reportData)}})
		} else if !isNoRows(err) {
			return nil, err
		}
	}
	rows, err := q.Query(ctx, `
		SELECT c.id::text, c.predicate, c.status, COALESCE(c.time_from::text, ''), COALESCE(c.time_to::text, ''), COALESCE(c.notes_ar, '')
		FROM claims c
		WHERE (c.subject_id = $2 OR c.object_id = $2 OR ($1 = 'place' AND c.place_id = $2))
		ORDER BY c.updated_at DESC
		LIMIT 30
	`, stage.Input.EntityType, stage.Input.EntityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, predicate, status, timeFrom, timeTo, notes string
		if err := rows.Scan(&id, &predicate, &status, &timeFrom, &timeTo, &notes); err != nil {
			return nil, err
		}
		items = append(items, EvidenceRef{Layer: "research_claim", Stance: "context", ReferenceType: "claim", ReferenceID: id, ClaimID: id, Excerpt: fmt.Sprintf("%s · %s", predicate, status), Metadata: map[string]any{"timeFrom": timeFrom, "timeTo": timeTo, "notesAr": notes, "status": status}})
	}
	return items, rows.Err()
}

func compareClaims(ctx context.Context, q queryer, stage stageContext) ([]EvidenceRef, error) {
	rows, err := q.Query(ctx, `
		SELECT c.id::text, c.predicate, c.status, COALESCE(c.notes_ar, ''),
		       COUNT(DISTINCT CASE WHEN ss.id IS NOT NULL AND ss.review_status = 'accepted'
		                              AND src.visibility = 'public' AND src.dependency_status = 'independent'
		                              AND NOT EXISTS (
		                                SELECT 1 FROM source_dependencies sd
		                                WHERE sd.status IN ('needs_review', 'confirmed') AND sd.source_id = src.id
		                              )
		                           THEN ce.source_statement_id END) AS evidence_count
		FROM claims c
		LEFT JOIN claim_evidence ce ON ce.claim_id = c.id
		LEFT JOIN source_statements ss ON ss.id = ce.source_statement_id
		LEFT JOIN sources src ON src.id = ss.source_id
		WHERE (c.subject_id = $2 OR c.object_id = $2 OR ($1 = 'place' AND c.place_id = $2))
		GROUP BY c.id
		ORDER BY CASE WHEN c.status IN ('disputed', 'contested', 'contradicted') THEN 0 ELSE 1 END, c.updated_at DESC
		LIMIT 50
	`, stage.Input.EntityType, stage.Input.EntityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EvidenceRef, 0)
	for rows.Next() {
		var id, predicate, status, notes string
		var count int
		if err := rows.Scan(&id, &predicate, &status, &notes, &count); err != nil {
			return nil, err
		}
		stance := "context"
		if count > 0 && (status == "supported" || status == "documented") {
			stance = "supports"
		}
		items = append(items, EvidenceRef{Layer: "research_claim", Stance: stance, ReferenceType: "claim", ReferenceID: id, ClaimID: id, Excerpt: fmt.Sprintf("%s · %s", predicate, status), Metadata: map[string]any{"status": status, "notesAr": notes, "evidenceCount": count}})
	}
	return items, rows.Err()
}

func inspectSourceDependency(ctx context.Context, q queryer, stage stageContext) ([]EvidenceRef, error) {
	if len(stage.SourceIDs) == 0 {
		return []EvidenceRef{}, nil
	}
	rows, err := q.Query(ctx, `
		SELECT sd.id::text, sd.source_id::text, source.title_ar, sd.depends_on_source_id::text, dependency.title_ar,
		       sd.dependency_type, sd.status, COALESCE(sd.evidence_ar, '')
		FROM source_dependencies sd
		JOIN sources source ON source.id = sd.source_id
		JOIN sources dependency ON dependency.id = sd.depends_on_source_id
		WHERE (sd.source_id = ANY($1::uuid[]) OR sd.depends_on_source_id = ANY($1::uuid[]))
		  AND source.visibility = 'public'
		  AND dependency.visibility = 'public'
		ORDER BY sd.created_at DESC
		LIMIT 50
	`, stage.SourceIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EvidenceRef, 0)
	for rows.Next() {
		var id, sourceID, sourceTitle, dependsOnID, dependsOnTitle, dependencyType, status, evidence string
		if err := rows.Scan(&id, &sourceID, &sourceTitle, &dependsOnID, &dependsOnTitle, &dependencyType, &status, &evidence); err != nil {
			return nil, err
		}
		items = append(items, EvidenceRef{Layer: "source_dependency", Stance: "context", ReferenceType: "source_dependency", ReferenceID: id, SourceID: sourceID, Excerpt: fmt.Sprintf("%s ← %s · %s", sourceTitle, dependsOnTitle, status), Metadata: map[string]any{"dependsOnSourceId": dependsOnID, "dependencyType": dependencyType, "status": status, "evidenceAr": evidence}})
	}
	return items, rows.Err()
}

func retrieveCounterEvidence(ctx context.Context, q queryer, stage stageContext) ([]EvidenceRef, error) {
	rows, err := q.Query(ctx, `
		SELECT ce.id::text, ce.claim_id::text, ce.source_statement_id::text, COALESCE(ss.source_id::text, ''),
		       COALESCE(ss.statement_text_ar, ''), ce.relation, c.status
		FROM claim_evidence ce
		JOIN claims c ON c.id = ce.claim_id
		JOIN source_statements ss ON ss.id = ce.source_statement_id
		JOIN sources s ON s.id = ss.source_id
		WHERE ce.relation IN ('contradicts', 'refutes')
		  AND s.visibility = 'public'
		  AND s.dependency_status = 'independent'
		  AND NOT EXISTS (
			SELECT 1 FROM source_dependencies sd
			WHERE sd.status IN ('needs_review', 'confirmed')
			  AND sd.source_id = s.id
		  )
		  AND (c.subject_id = $2 OR c.object_id = $2 OR ($1 = 'place' AND c.place_id = $2))
		ORDER BY ce.created_at DESC
		LIMIT 50
	`, stage.Input.EntityType, stage.Input.EntityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]EvidenceRef, 0)
	for rows.Next() {
		var id, claimID, statementID, sourceID, excerpt, relation, claimStatus string
		if err := rows.Scan(&id, &claimID, &statementID, &sourceID, &excerpt, &relation, &claimStatus); err != nil {
			return nil, err
		}
		items = append(items, EvidenceRef{Layer: "source_statement", Stance: "counter_evidence", ReferenceType: "claim_evidence", ReferenceID: id, SourceID: sourceID, StatementID: statementID, ClaimID: claimID, Excerpt: excerpt, Metadata: map[string]any{"relation": relation, "claimStatus": claimStatus}})
	}
	return items, rows.Err()
}

func mergeEvidence(values ...[]EvidenceRef) []EvidenceRef {
	seen := make(map[string]bool)
	items := make([]EvidenceRef, 0)
	for _, group := range values {
		for _, item := range group {
			key := item.ReferenceType + ":" + item.ReferenceID + ":" + item.Stance
			if seen[key] {
				continue
			}
			seen[key] = true
			items = append(items, item)
			if len(items) == MaximumEvidence {
				return items
			}
		}
	}
	return items
}

func buildEvidencePackage(evidence []EvidenceRef) EvidencePackage {
	packageData := EvidencePackage{Evidence: evidence, Total: len(evidence)}
	sourceIDs := make(map[string]struct{})
	for _, item := range evidence {
		switch item.Stance {
		case "supports":
			packageData.SupportCount++
		case "counter_evidence":
			packageData.CounterCount++
		case "hypothesis":
			packageData.HypothesisCount++
		default:
			packageData.ContextCount++
		}
		if item.SourceID != "" {
			sourceIDs[item.SourceID] = struct{}{}
		}
	}
	packageData.SourceCount = len(sourceIDs)
	return packageData
}

func buildGaps(runID string, input RunInput, evidence []EvidenceRef, sourceCount, claimCount, dependencyCount, graphCount, geographyCount, chronologyCount int) ([]Gap, []Recommendation) {
	gaps := make([]Gap, 0)
	recommendations := make([]Recommendation, 0)
	addGap := func(kind, description, severity string, metadata map[string]any) {
		gaps = append(gaps, Gap{ID: stableID(runID, "gap", kind, description), Kind: kind, DescriptionAR: description, Severity: severity, Status: "open", Metadata: metadata})
	}
	addRecommendation := func(action, rationale, priority string, metadata map[string]any) {
		recommendations = append(recommendations, Recommendation{ID: stableID(runID, "recommendation", action, rationale), Action: action, RationaleAR: rationale, Priority: priority, Status: "suggested", Metadata: metadata})
	}
	supportCount := 0
	counterCount := 0
	for _, item := range evidence {
		if item.Stance == "supports" {
			supportCount++
		}
		if item.Stance == "counter_evidence" {
			counterCount++
		}
	}
	if supportCount == 0 {
		addGap("missing_source_evidence", "لم تُجمع عبارات مصدرية مؤهلة تدعم السؤال.", "high", map[string]any{"sourceCount": sourceCount})
		addRecommendation("توسيع البحث المصدري", "ابحث عن مصادر مستقلة تحتوي على عبارات صريحة.", "high", nil)
	}
	if claimCount > 0 && counterCount == 0 {
		addGap("missing_counter_evidence", "لا توجد أدلة مضادة أو متضاربة موثقة في النطاق.", "medium", nil)
		addRecommendation("فحص الروايات المقابلة", "قارن المصادر التي تختلف في العلاقة أو المكان قبل الحسم.", "normal", nil)
	}
	if dependencyCount > 0 {
		addGap("source_dependency", "توجد علاقة اعتماد مصدرية تحتاج إلى مراجعة قبل عدّ المصادر مستقلة.", "high", map[string]any{"dependencyCount": dependencyCount})
		addRecommendation("مراجعة الاعتماد بين المصادر", "راجع المصدر التبعي وسلسلة النشر قبل زيادة وزن الأدلة.", "high", nil)
	}
	if graphCount == 0 {
		addGap("missing_graph_path", "لم يظهر مسار بياني قابل للتتبع ضمن النسخة المختارة.", "medium", nil)
		addRecommendation("توسيع نطاق الشجرة", "افحص نسخة شجرة منشورة تحتوي الكيان والعلاقات المطلوبة.", "normal", nil)
	}
	if geographyCount == 0 {
		addGap("missing_geography", "لا توجد إشارات جغرافية مؤهلة ضمن النطاق.", "low", nil)
	}
	if chronologyCount == 0 {
		addGap("missing_chronology", "لا توجد إشارة زمنية أو ادعاء تاريخي كافٍ.", "medium", nil)
	}
	if len(gaps) == 0 {
		addRecommendation("مراجعة يدوية للحزمة", "راجع الاقتباسات والتسلسل قبل تحويل الفرضية إلى سؤال مفتوح.", "normal", nil)
	}
	return gaps, recommendations
}

func stableID(parts ...string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(strings.Join(parts, "|"))).String()
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func nullableUUIDValue(value string) any {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	return parsed
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
