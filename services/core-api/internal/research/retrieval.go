package research

import (
	"context"
	"sort"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"
)

func (s *Service) retrieveLexicalPassages(ctx context.Context, retrieval retrievalContext) ([]Citation, error) {
	rows, err := s.Pool.Query(ctx, `
		WITH scored AS (
			SELECT sp.id, sp.source_id, s.title_ar, sp.text_ar, sp.locator_ar, sp.page_number,
			       ss.id AS statement_id, ss.statement_text_ar, ss.review_status,
			       GREATEST(
			         CASE WHEN sp.normalized_text_ar = $1 THEN 1.0 ELSE 0.0 END,
			         similarity(sp.normalized_text_ar, $1),
			         CASE WHEN sp.text_ar ILIKE '%' || $1 || '%' THEN 0.75 ELSE 0.0 END,
			         CASE WHEN COALESCE(ss.statement_text_ar, '') ILIKE '%' || $1 || '%' THEN 0.85 ELSE 0.0 END
			       ) AS lexical_score
			FROM source_passages sp
			JOIN sources s ON s.id = sp.source_id
			LEFT JOIN LATERAL (
				SELECT id, statement_text_ar, review_status
				FROM source_statements
				WHERE source_passage_id = sp.id AND review_status = 'accepted'
				ORDER BY created_at
				LIMIT 1
			) ss ON TRUE
			WHERE ss.id IS NOT NULL
			  AND (($2 = '' AND s.visibility = 'public') OR s.created_by = NULLIF($2, '')::uuid OR EXISTS (
				SELECT 1 FROM user_roles ur WHERE ur.user_id = NULLIF($2, '')::uuid AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')
			  ))
			  AND ($3 = '' OR sp.source_id = $3::uuid)
			  AND ($4 = '' OR EXISTS (
				SELECT 1 FROM claim_evidence ce JOIN claims c ON c.id = ce.claim_id
				WHERE ce.source_passage_id = sp.id AND (c.subject_id = $4::uuid OR c.object_id = $4::uuid)
			  ))
			  AND ($5 = '' OR EXISTS (
				SELECT 1 FROM geographic_associations ga WHERE ga.source_id = s.id AND ga.place_id = $5::uuid
				UNION ALL SELECT 1 FROM migration_events me WHERE me.source_id = s.id AND (me.from_place_id = $5::uuid OR me.to_place_id = $5::uuid)
			  ))
			  AND ($6 = 0 OR s.publication_date_from IS NULL OR EXTRACT(YEAR FROM s.publication_date_from) <= $7)
			  AND ($7 = 0 OR s.publication_date_to IS NULL OR EXTRACT(YEAR FROM s.publication_date_to) >= $6)
		)
		SELECT id, source_id, title_ar, text_ar, locator_ar, page_number, statement_id, statement_text_ar, review_status, lexical_score
		FROM scored WHERE lexical_score > 0 ORDER BY lexical_score DESC, id LIMIT 50
	`, retrieval.Normalized, retrieval.ActorID, retrieval.Input.SourceID, retrieval.Input.PersonID, retrieval.Input.PlaceID, retrieval.Input.FromYear, retrieval.Input.ToYear)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Citation, 0)
	for rows.Next() {
		item, score, err := scanPassage(rows)
		if err != nil {
			return nil, err
		}
		item.Score.Lexical = score
		item.Score.Combined = score
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) retrieveVectorPassages(ctx context.Context, retrieval retrievalContext) ([]Citation, error) {
	if len(retrieval.Vector) != 1536 {
		return nil, ErrValidation
	}
	rows, err := s.Pool.Query(ctx, `
		WITH scored AS (
			SELECT sp.id, sp.source_id, s.title_ar, sp.text_ar, sp.locator_ar, sp.page_number,
			       ss.id AS statement_id, ss.statement_text_ar, ss.review_status,
			       1 - (sp.embedding <=> $1::vector) AS vector_score
			FROM source_passages sp
			JOIN sources s ON s.id = sp.source_id
			LEFT JOIN LATERAL (
				SELECT id, statement_text_ar, review_status
				FROM source_statements
				WHERE source_passage_id = sp.id AND review_status = 'accepted'
				ORDER BY created_at
				LIMIT 1
			) ss ON TRUE
			WHERE sp.embedding IS NOT NULL AND ss.id IS NOT NULL
			  AND (($2 = '' AND s.visibility = 'public') OR s.created_by = NULLIF($2, '')::uuid OR EXISTS (
				SELECT 1 FROM user_roles ur WHERE ur.user_id = NULLIF($2, '')::uuid AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')
			  ))
			  AND ($3 = '' OR sp.source_id = $3::uuid)
			  AND ($4 = '' OR EXISTS (
				SELECT 1 FROM claim_evidence ce JOIN claims c ON c.id = ce.claim_id
				WHERE ce.source_passage_id = sp.id AND (c.subject_id = $4::uuid OR c.object_id = $4::uuid)
			  ))
			  AND ($5 = '' OR EXISTS (
				SELECT 1 FROM geographic_associations ga WHERE ga.source_id = s.id AND ga.place_id = $5::uuid
				UNION ALL SELECT 1 FROM migration_events me WHERE me.source_id = s.id AND (me.from_place_id = $5::uuid OR me.to_place_id = $5::uuid)
			  ))
			  AND ($6 = 0 OR s.publication_date_from IS NULL OR EXTRACT(YEAR FROM s.publication_date_from) <= $7)
			  AND ($7 = 0 OR s.publication_date_to IS NULL OR EXTRACT(YEAR FROM s.publication_date_to) >= $6)
		)
		SELECT id, source_id, title_ar, text_ar, locator_ar, page_number, statement_id, statement_text_ar, review_status, vector_score
		FROM scored WHERE vector_score > 0 ORDER BY vector_score DESC, id LIMIT 50
	`, pgvector.NewVector(retrieval.Vector), retrieval.ActorID, retrieval.Input.SourceID, retrieval.Input.PersonID, retrieval.Input.PlaceID, retrieval.Input.FromYear, retrieval.Input.ToYear)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Citation, 0)
	for rows.Next() {
		item, score, err := scanPassage(rows)
		if err != nil {
			return nil, err
		}
		item.Score.Vector = score
		item.Score.Combined = score
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanPassage(row pgx.Row) (Citation, float64, error) {
	var item Citation
	var passageID, sourceID, statementID pgtype.UUID
	var title, passageText, locator, statementText, reviewStatus pgtype.Text
	var pageNumber pgtype.Int4
	var score float64
	if err := row.Scan(&passageID, &sourceID, &title, &passageText, &locator, &pageNumber, &statementID, &statementText, &reviewStatus, &score); err != nil {
		return Citation{}, 0, err
	}
	item = Citation{
		Layer:        SourceStatement,
		Type:         "source_statement",
		ID:           uuidText(statementID),
		SourceID:     uuidText(sourceID),
		PassageID:    uuidText(passageID),
		StatementID:  uuidText(statementID),
		Title:        textValue(title),
		Excerpt:      textValue(statementText),
		LocatorAR:    textValue(locator),
		ReviewStatus: textValue(reviewStatus),
	}
	if item.Excerpt == "" {
		item.Excerpt = textValue(passageText)
	}
	if pageNumber.Valid {
		value := int(pageNumber.Int32)
		item.PageNumber = &value
	}
	return item, score, nil
}

func (s *Service) fuseAndRerank(ctx context.Context, retrieval retrievalContext, lexical, vectorResults []Citation) ([]Citation, RetrievalStats, error) {
	byPassage := make(map[string]*Citation, len(lexical)+len(vectorResults))
	add := func(candidate Citation) {
		current, found := byPassage[candidate.PassageID]
		if !found {
			copy := candidate
			byPassage[candidate.PassageID] = &copy
			return
		}
		if candidate.Score.Lexical > 0 {
			current.Score.Lexical = candidate.Score.Lexical
		}
		if candidate.Score.Vector > 0 {
			current.Score.Vector = candidate.Score.Vector
		}
	}
	for _, candidate := range lexical {
		add(candidate)
	}
	for _, candidate := range vectorResults {
		add(candidate)
	}
	items := make([]Citation, 0, len(byPassage))
	for _, candidate := range byPassage {
		if candidate.Score.Lexical > 0 && candidate.Score.Vector > 0 {
			candidate.Score.Combined = candidate.Score.Lexical*0.55 + candidate.Score.Vector*0.45
		} else if candidate.Score.Lexical > 0 {
			candidate.Score.Combined = candidate.Score.Lexical
		} else {
			candidate.Score.Combined = candidate.Score.Vector
		}
		items = append(items, *candidate)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Score.Combined > items[j].Score.Combined })
	if len(items) > 30 {
		items = items[:30]
	}
	stats := RetrievalStats{LexicalCandidates: len(lexical), VectorCandidates: len(vectorResults), FusedCandidates: len(items)}
	if len(items) == 0 {
		return items, stats, nil
	}
	documents := make([]ai.RerankDocument, 0, len(items))
	for _, item := range items {
		documents = append(documents, ai.RerankDocument{ID: item.PassageID, Text: item.Excerpt})
	}
	reranked, err := s.AI.Rerank(ctx, ai.RerankRequest{Query: retrieval.Input.Question, Documents: documents})
	if err != nil {
		return nil, stats, err
	}
	byID := make(map[string]Citation, len(items))
	for _, item := range items {
		byID[item.PassageID] = item
	}
	final := make([]Citation, 0, len(reranked.Documents))
	for _, document := range reranked.Documents {
		item, found := byID[document.ID]
		if !found {
			continue
		}
		item.Score.Rerank = document.Score
		final = append(final, item)
		delete(byID, document.ID)
	}
	for _, item := range byID {
		final = append(final, item)
	}
	sort.SliceStable(final, func(i, j int) bool {
		if final[i].Score.Rerank == final[j].Score.Rerank {
			return final[i].Score.Combined > final[j].Score.Combined
		}
		return final[i].Score.Rerank > final[j].Score.Rerank
	})
	for index := range final {
		final[index].Rank = index + 1
	}
	stats.RerankedCandidates = len(final)
	return final, stats, nil
}

func claimConflicts(claims []Citation) []Conflict {
	conflicts := make([]Conflict, 0)
	for _, claim := range claims {
		if claim.Status == "" {
			continue
		}
		if claim.Status == "contested" || claim.Status == "disputed" || claim.Status == "contradicted" || claim.Status == "rejected" {
			conflicts = append(conflicts, Conflict{Type: "research_claim", LeftID: claim.ID, RightID: claim.ID, Status: claim.Status, Explanation: "ادعاء بحالة تنافسية أو غير محسوم.", SourceIDs: sourceIDs(claim)})
		}
	}
	return conflicts
}

func sourceIDs(citation Citation) []string {
	if citation.SourceID == "" {
		return nil
	}
	return []string{citation.SourceID}
}

func uuidText(value pgtype.UUID) string {
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

func (s *Service) retrieveClaims(ctx context.Context, retrieval retrievalContext) ([]Citation, error) {
	rows, err := s.Pool.Query(ctx, `
		WITH visible_evidence AS (
			SELECT ce.claim_id, ce.source_statement_id, ce.source_passage_id, ss.source_id,
			       s.title_ar, ss.statement_text_ar, sp.text_ar, ss.review_status
			FROM claim_evidence ce
			LEFT JOIN source_statements ss ON ss.id = ce.source_statement_id
			LEFT JOIN source_passages sp ON sp.id = ce.source_passage_id
			LEFT JOIN sources s ON s.id = COALESCE(ss.source_id, sp.source_id)
			WHERE (($2 = '' AND s.visibility = 'public') OR s.created_by = NULLIF($2, '')::uuid OR EXISTS (
				SELECT 1 FROM user_roles ur WHERE ur.user_id = NULLIF($2, '')::uuid AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')
			))
		)
		SELECT c.id, c.subject_type, c.predicate, c.object_type, c.status, COALESCE(c.notes_ar, ''),
		       ve.source_id, ve.source_statement_id, ve.source_passage_id, COALESCE(ve.title_ar, ''),
		       COALESCE(ve.statement_text_ar, ve.text_ar, ''), COALESCE(ve.review_status, ''),
			       GREATEST(similarity(c.predicate, $1), CASE WHEN COALESCE(c.notes_ar, '') ILIKE '%' || $1 || '%' THEN 0.7 ELSE 0.0 END, word_similarity($1, COALESCE(ve.statement_text_ar, '')) * 0.8) AS score
		FROM claims c
		LEFT JOIN LATERAL (
			SELECT * FROM visible_evidence ve0 WHERE ve0.claim_id = c.id ORDER BY ve0.source_statement_id NULLS LAST LIMIT 1
		) ve ON TRUE
		WHERE ve.claim_id IS NOT NULL
		  AND ve.review_status = 'accepted'
		  AND (c.predicate ILIKE '%' || $1 || '%' OR COALESCE(c.notes_ar, '') ILIKE '%' || $1 || '%' OR similarity(c.predicate, $1) > 0.1 OR word_similarity($1, COALESCE(ve.statement_text_ar, '')) > 0.15)
		  AND ($3 = '' OR ve.source_id = $3::uuid)
		  AND ($4 = '' OR c.subject_id = $4::uuid OR c.object_id = $4::uuid)
		  AND ($5 = '' OR c.place_id = $5::uuid)
		  AND ($6 = 0 OR c.time_from IS NULL OR EXTRACT(YEAR FROM c.time_from) <= $7)
		  AND ($7 = 0 OR c.time_to IS NULL OR EXTRACT(YEAR FROM c.time_to) >= $6)
		ORDER BY score DESC, c.updated_at DESC LIMIT 20
	`, retrieval.Normalized, retrieval.ActorID, retrieval.Input.SourceID, retrieval.Input.PersonID, retrieval.Input.PlaceID, retrieval.Input.FromYear, retrieval.Input.ToYear)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Citation, 0)
	for rows.Next() {
		var item Citation
		var claimID, sourceID, statementID, passageID pgtype.UUID
		var subjectType, predicate, objectType, status, notes, title, excerpt, reviewStatus pgtype.Text
		var score float64
		if err := rows.Scan(&claimID, &subjectType, &predicate, &objectType, &status, &notes, &sourceID, &statementID, &passageID, &title, &excerpt, &reviewStatus, &score); err != nil {
			return nil, err
		}
		item = Citation{Layer: ResearchClaim, Type: "research_claim", ID: uuidText(claimID), SourceID: uuidText(sourceID), StatementID: uuidText(statementID), PassageID: uuidText(passageID), Title: textValue(title), Excerpt: textValue(excerpt), ReviewStatus: textValue(reviewStatus), Status: textValue(status), Score: Score{Combined: score, Rerank: score}}
		if item.Excerpt == "" {
			item.Excerpt = textValue(notes)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Service) retrieveTreeInterpretations(ctx context.Context, retrieval retrievalContext) ([]Citation, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT tr.id, t.name_ar, tv.version_number, sn.display_name_ar, object_node.display_name_ar,
		       tr.predicate, tr.status, tr.source_id, COALESCE(s.title_ar, ''),
			       GREATEST(CASE WHEN sn.display_name_ar ILIKE '%' || $1 || '%' THEN 0.9 ELSE 0.0 END,
			                CASE WHEN object_node.display_name_ar ILIKE '%' || $1 || '%' THEN 0.9 ELSE 0.0 END,
			                CASE WHEN tr.predicate ILIKE '%' || $1 || '%' THEN 0.8 ELSE 0.0 END,
			                word_similarity($1, sn.display_name_ar),
			                word_similarity($1, object_node.display_name_ar)) AS score
		FROM tree_relationships tr
		JOIN tree_versions tv ON tv.id = tr.tree_version_id AND tv.state = 'published'
		JOIN trees t ON t.id = tv.tree_id
		JOIN tree_nodes sn ON sn.id = tr.subject_node_id
		JOIN tree_nodes object_node ON object_node.id = tr.object_node_id
		LEFT JOIN sources s ON s.id = tr.source_id
		WHERE (sn.display_name_ar ILIKE '%' || $1 || '%' OR object_node.display_name_ar ILIKE '%' || $1 || '%' OR tr.predicate ILIKE '%' || $1 || '%' OR word_similarity($1, sn.display_name_ar) > 0.15 OR word_similarity($1, object_node.display_name_ar) > 0.15)
		  AND (t.visibility = 'public' OR ($2 = '' AND t.visibility = 'public') OR t.owner_id = NULLIF($2, '')::uuid OR EXISTS (
			SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = NULLIF($2, '')::uuid
		  ) OR EXISTS (
			SELECT 1 FROM user_roles ur WHERE ur.user_id = NULLIF($2, '')::uuid AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')
		  ))
		  AND (s.id IS NULL OR ($2 = '' AND s.visibility = 'public') OR s.created_by = NULLIF($2, '')::uuid OR EXISTS (
			SELECT 1 FROM user_roles ur WHERE ur.user_id = NULLIF($2, '')::uuid AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')
		  ) OR EXISTS (
			SELECT 1 FROM trees tree_owner WHERE tree_owner.id = t.id AND tree_owner.owner_id = NULLIF($2, '')::uuid
		  ) OR EXISTS (
			SELECT 1 FROM tree_collaborators tc WHERE tc.tree_id = t.id AND tc.user_id = NULLIF($2, '')::uuid
		  ))
		  AND ($3 = '' OR tr.source_id = NULLIF($3, '')::uuid)
		  AND ($4 = '' OR sn.person_id = NULLIF($4, '')::uuid OR object_node.person_id = NULLIF($4, '')::uuid)
		  AND ($5 = '' OR t.id = NULLIF($5, '')::uuid)
		  AND ($6 = '' OR tv.id = NULLIF($6, '')::uuid)
		ORDER BY score DESC, tr.created_at DESC LIMIT 20
	`, retrieval.Normalized, retrieval.ActorID, retrieval.Input.SourceID, retrieval.Input.PersonID, retrieval.Input.TreeID, retrieval.Input.TreeVersionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Citation, 0)
	for rows.Next() {
		var id, sourceID pgtype.UUID
		var title, subject, object, predicate, status, sourceTitle pgtype.Text
		var version int32
		var score float64
		if err := rows.Scan(&id, &title, &version, &subject, &object, &predicate, &status, &sourceID, &sourceTitle, &score); err != nil {
			return nil, err
		}
		excerpt := textValue(subject) + " " + textValue(predicate) + " " + textValue(object)
		items = append(items, Citation{Layer: TreeInterpretation, Type: "tree_interpretation", ID: uuidText(id), SourceID: uuidText(sourceID), Title: textValue(title), Excerpt: excerpt, LocatorAR: "نسخة منشورة " + itoa32(version), Status: textValue(status), Score: Score{Combined: score, Rerank: score}})
	}
	return items, rows.Err()
}

func (s *Service) retrieveFindings(ctx context.Context, retrieval retrievalContext) ([]Citation, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT pf.id, pf.title_ar, pf.explanation_ar, pf.status, COALESCE(pf.algorithm_version, '')
		FROM platform_findings pf
		WHERE (pf.title_ar ILIKE '%' || $1 || '%' OR pf.explanation_ar ILIKE '%' || $1 || '%' OR similarity(pf.title_ar, $1) > 0.1 OR word_similarity($1, pf.title_ar) > 0.15 OR word_similarity($1, pf.explanation_ar) > 0.15)
		  AND pf.status <> 'dismissed'
		ORDER BY pf.updated_at DESC LIMIT 20
	`, retrieval.Normalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Citation, 0)
	for rows.Next() {
		var id pgtype.UUID
		var title, explanation, status, algorithm pgtype.Text
		if err := rows.Scan(&id, &title, &explanation, &status, &algorithm); err != nil {
			return nil, err
		}
		items = append(items, Citation{Layer: PlatformFinding, Type: "platform_finding", ID: uuidText(id), FindingID: uuidText(id), Title: textValue(title), Excerpt: textValue(explanation), Status: textValue(status), LocatorAR: textValue(algorithm)})
	}
	return items, rows.Err()
}

func (s *Service) retrieveQuestions(ctx context.Context, retrieval retrievalContext) ([]Citation, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT q.id, q.title_ar, COALESCE(q.description_ar, ''), q.status, q.priority
		FROM open_questions q
		WHERE (q.title_ar ILIKE '%' || $1 || '%' OR COALESCE(q.description_ar, '') ILIKE '%' || $1 || '%' OR similarity(q.title_ar, $1) > 0.1 OR word_similarity($1, q.title_ar) > 0.15 OR word_similarity($1, COALESCE(q.description_ar, '')) > 0.15)
		  AND q.status NOT IN ('resolved', 'archived')
		ORDER BY q.updated_at DESC LIMIT 20
	`, retrieval.Normalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Citation, 0)
	for rows.Next() {
		var id pgtype.UUID
		var title, description, status, priority pgtype.Text
		if err := rows.Scan(&id, &title, &description, &status, &priority); err != nil {
			return nil, err
		}
		items = append(items, Citation{Layer: OpenQuestion, Type: "open_question", ID: uuidText(id), QuestionID: uuidText(id), Title: textValue(title), Excerpt: textValue(description), Status: textValue(status), LocatorAR: textValue(priority)})
	}
	return items, rows.Err()
}

func itoa32(value int32) string {
	if value == 0 {
		return "0"
	}
	result := ""
	for value > 0 {
		result = string(rune('0'+value%10)) + result
		value /= 10
	}
	return result
}
