package evidence

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	SourceDependencyAlgorithmVersion    = "source-dependency-v1"
	SourceDependencyMaxSourcePassages   = 100
	SourceDependencyMaxMatches          = 500
	SourceDependencyMaxSharedClaims     = 500
	SourceDependencyMinPassageLength    = 80
	SourceDependencySimilarityThreshold = 0.86
)

type SourceDependencyView struct {
	ID                     string         `json:"id"`
	SourceID               string         `json:"sourceId"`
	SourceTitleAR          string         `json:"sourceTitleAr"`
	DependsOnSourceID      string         `json:"dependsOnSourceId,omitempty"`
	DependsOnSourceTitleAR string         `json:"dependsOnSourceTitleAr,omitempty"`
	DependencyType         string         `json:"dependencyType"`
	EvidenceAR             string         `json:"evidenceAr,omitempty"`
	Status                 string         `json:"status"`
	AlgorithmVersion       string         `json:"algorithmVersion,omitempty"`
	SignalData             map[string]any `json:"signalData"`
	CreatedAt              time.Time      `json:"createdAt"`
	ReviewedBy             string         `json:"reviewedBy,omitempty"`
	ReviewedAt             *time.Time     `json:"reviewedAt,omitempty"`
	ReviewNoteAR           string         `json:"reviewNoteAr,omitempty"`
}

type SourceDependencySummary struct {
	Total                  int `json:"total"`
	NeedsReview            int `json:"needsReview"`
	Confirmed              int `json:"confirmed"`
	Rejected               int `json:"rejected"`
	IndependentSourceCount int `json:"independentSourceCount"`
}

type SourceDependencyGraph struct {
	SourceID            string                  `json:"sourceId"`
	Items               []SourceDependencyView  `json:"items"`
	Summary             SourceDependencySummary `json:"summary"`
	DetectedCount       int                     `json:"detectedCount"`
	ScannedPassageCount int                     `json:"scannedPassageCount"`
	Truncated           bool                    `json:"truncated"`
}

type CreateSourceDependencyInput struct {
	DependsOnSourceID string `json:"depends_on_source_id"`
	DependencyType    string `json:"dependency_type"`
	EvidenceAR        string `json:"evidence_ar"`
}

type ReviewSourceDependencyInput struct {
	Decision string `json:"decision"`
	NoteAR   string `json:"note_ar"`
}

type dependencyPassage struct {
	ID   uuid.UUID
	Text string
}

type dependencyCandidate struct {
	SourceID          uuid.UUID
	SourceTitleAR     string
	PhraseScore       float64
	MatchedPassageIDs []string
	SharedClaimIDs    []string
}

type sharedClaimCandidate struct {
	SourceID      uuid.UUID
	SourceTitleAR string
	ClaimIDs      []string
}

func (s *Service) ListSourceDependencies(ctx context.Context, sourceID, actorID string) (SourceDependencyGraph, error) {
	sourceUUID, actorUUID, err := s.dependencySource(ctx, sourceID, actorID, false)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	graph, err := s.loadSourceDependencyGraph(ctx, s.Pool, sourceUUID, actorUUID)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	return graph, nil
}

func (s *Service) CreateSourceDependency(ctx context.Context, sourceID, actorID string, input CreateSourceDependencyInput) (SourceDependencyGraph, error) {
	sourceUUID, actorUUID, err := s.dependencySource(ctx, sourceID, actorID, true)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	input, err = validateSourceDependencyInput(input)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	targetUUID, err := uuid.Parse(input.DependsOnSourceID)
	if err != nil {
		return SourceDependencyGraph{}, ErrValidation
	}
	if sourceUUID == targetUUID {
		return SourceDependencyGraph{}, ErrValidation
	}
	target, err := s.sourceByIDAny(ctx, s.Pool, targetUUID)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	if target.Visibility != "public" {
		allowed, accessErr := canViewSource(ctx, s.Pool, targetUUID, *actorUUID)
		if accessErr != nil {
			return SourceDependencyGraph{}, accessErr
		}
		if !allowed {
			return SourceDependencyGraph{}, ErrForbidden
		}
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	defer tx.Rollback(ctx)
	dependencyID := uuid.New()
	var evidence any
	if input.EvidenceAR != "" {
		evidence = input.EvidenceAR
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO source_dependencies
			(id, source_id, depends_on_source_id, dependency_type, evidence_ar, status, algorithm_version, signal_data)
		VALUES ($1, $2, $3, $4, $5, 'needs_review', 'manual-v1', $6)
	`, dependencyID, sourceUUID, targetUUID, input.DependencyType, evidence, mustJSON(map[string]any{"signal": "manual"})); err != nil {
		return SourceDependencyGraph{}, mapConflict(err)
	}
	if err := refreshSourceDependencyStatus(ctx, tx, sourceUUID); err != nil {
		return SourceDependencyGraph{}, err
	}
	if err := writeAudit(ctx, tx, *actorUUID, "source_dependency_created", "source_dependency", dependencyID, nil, map[string]any{
		"sourceId":          sourceUUID.String(),
		"dependsOnSourceId": targetUUID.String(),
		"dependencyType":    input.DependencyType,
	}); err != nil {
		return SourceDependencyGraph{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceDependencyGraph{}, err
	}
	return s.loadSourceDependencyGraph(ctx, s.Pool, sourceUUID, actorUUID)
}

func (s *Service) ReviewSourceDependency(ctx context.Context, dependencyID, actorID string, input ReviewSourceDependencyInput) (SourceDependencyGraph, error) {
	if err := s.ready(); err != nil {
		return SourceDependencyGraph{}, err
	}
	dependencyUUID, err := uuid.Parse(strings.TrimSpace(dependencyID))
	if err != nil {
		return SourceDependencyGraph{}, ErrNotFound
	}
	reviewerUUID, err := uuid.Parse(strings.TrimSpace(actorID))
	if err != nil {
		return SourceDependencyGraph{}, ErrForbidden
	}
	input, err = validateSourceDependencyReviewInput(input)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	defer tx.Rollback(ctx)
	var sourceID, targetID pgtype.UUID
	var currentStatus string
	if err := tx.QueryRow(ctx, `
		SELECT source_id, depends_on_source_id, status
		FROM source_dependencies
		WHERE id = $1
		FOR UPDATE
	`, dependencyUUID).Scan(&sourceID, &targetID, &currentStatus); err != nil {
		if err == pgx.ErrNoRows {
			return SourceDependencyGraph{}, ErrNotFound
		}
		return SourceDependencyGraph{}, err
	}
	if currentStatus != "needs_review" {
		return SourceDependencyGraph{}, ErrConflict
	}
	sourceUUID := uuid.UUID(sourceID.Bytes)
	allowed, err := canManageSource(ctx, tx, sourceUUID, reviewerUUID)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	if !allowed {
		return SourceDependencyGraph{}, ErrForbidden
	}
	if targetID.Valid {
		allowed, err = canViewSource(ctx, tx, uuid.UUID(targetID.Bytes), reviewerUUID)
		if err != nil {
			return SourceDependencyGraph{}, err
		}
		if !allowed {
			return SourceDependencyGraph{}, ErrForbidden
		}
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO source_dependency_reviews (id, dependency_id, reviewer_id, decision, note_ar)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
	`, uuid.New(), dependencyUUID, reviewerUUID, input.Decision, input.NoteAR); err != nil {
		return SourceDependencyGraph{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE source_dependencies
		SET status = $1, reviewed_by = $2, reviewed_at = now(), review_note_ar = NULLIF($3, '')
		WHERE id = $4
	`, input.Decision, reviewerUUID, input.NoteAR, dependencyUUID); err != nil {
		return SourceDependencyGraph{}, err
	}
	if err := refreshSourceDependencyStatus(ctx, tx, sourceUUID); err != nil {
		return SourceDependencyGraph{}, err
	}
	if err := writeAudit(ctx, tx, reviewerUUID, "source_dependency_reviewed", "source_dependency", dependencyUUID, map[string]any{"status": currentStatus}, map[string]any{
		"decision": input.Decision,
		"noteAr":   input.NoteAR,
	}); err != nil {
		return SourceDependencyGraph{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SourceDependencyGraph{}, err
	}
	return s.loadSourceDependencyGraph(ctx, s.Pool, sourceUUID, &reviewerUUID)
}

func (s *Service) DetectSourceDependencies(ctx context.Context, sourceID, actorID string) (SourceDependencyGraph, error) {
	sourceUUID, actorUUID, err := s.dependencySource(ctx, sourceID, actorID, true)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	detectionContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.Begin(detectionContext)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	defer tx.Rollback(detectionContext)
	passages, truncated, err := dependencyPassages(detectionContext, tx, sourceUUID)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	candidates, candidatesTruncated, err := detectDependencyCandidates(detectionContext, tx, sourceUUID, actorUUID, passages)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	detectedCount := 0
	for _, candidate := range candidates {
		dependencyType := "likely_paraphrase"
		evidenceAR := "تشابه الصياغة بين مقاطع المصدرين يحتاج إلى مراجعة بشرية."
		if candidate.PhraseScore <= 0 {
			dependencyType = "shared_origin"
			evidenceAR = "تسلسل ادعاءات مشترك بين المصدرين يحتاج إلى مراجعة بشرية."
		}
		signals := make([]map[string]any, 0, 2)
		if candidate.PhraseScore > 0 {
			signals = append(signals, map[string]any{
				"type":              "repeated_wording",
				"similarity":        candidate.PhraseScore,
				"matchedPassageIds": candidate.MatchedPassageIDs,
			})
		}
		if len(candidate.SharedClaimIDs) > 0 {
			signals = append(signals, map[string]any{
				"type":     "shared_claim_sequence",
				"claimIds": candidate.SharedClaimIDs,
			})
		}
		signalData := map[string]any{
			"algorithmVersion": SourceDependencyAlgorithmVersion,
			"signals":          signals,
		}
		dependencyID := uuid.New()
		result, err := tx.Exec(detectionContext, `
			INSERT INTO source_dependencies
				(id, source_id, depends_on_source_id, dependency_type, evidence_ar, status, algorithm_version, signal_data)
			VALUES ($1, $2, $3, $4, $5, 'needs_review', $6, $7)
			ON CONFLICT (source_id, depends_on_source_id, dependency_type) DO NOTHING
		`, dependencyID, sourceUUID, candidate.SourceID, dependencyType, evidenceAR, SourceDependencyAlgorithmVersion, mustJSON(signalData))
		if err != nil {
			return SourceDependencyGraph{}, err
		}
		if result.RowsAffected() > 0 {
			detectedCount++
		}
	}
	if err := refreshSourceDependencyStatus(detectionContext, tx, sourceUUID); err != nil {
		return SourceDependencyGraph{}, err
	}
	if err := writeAudit(detectionContext, tx, *actorUUID, "source_dependency_scan_completed", "source", sourceUUID, nil, map[string]any{
		"algorithmVersion":    SourceDependencyAlgorithmVersion,
		"detectedCount":       detectedCount,
		"scannedPassageCount": len(passages),
		"truncated":           truncated || candidatesTruncated,
	}); err != nil {
		return SourceDependencyGraph{}, err
	}
	if err := tx.Commit(detectionContext); err != nil {
		return SourceDependencyGraph{}, err
	}
	graph, err := s.loadSourceDependencyGraph(ctx, s.Pool, sourceUUID, actorUUID)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	graph.DetectedCount = detectedCount
	graph.ScannedPassageCount = len(passages)
	graph.Truncated = truncated || candidatesTruncated
	return graph, nil
}

func (s *Service) dependencySource(ctx context.Context, sourceID, actorID string, manage bool) (uuid.UUID, *uuid.UUID, error) {
	if err := s.ready(); err != nil {
		return uuid.Nil, nil, err
	}
	sourceUUID, err := uuid.Parse(strings.TrimSpace(sourceID))
	if err != nil {
		return uuid.Nil, nil, ErrNotFound
	}
	actorUUID, err := parseDependencyActor(actorID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if manage && actorUUID == nil {
		return uuid.Nil, nil, ErrForbidden
	}
	if actorUUID == nil {
		if _, err := s.sourceByID(ctx, s.Pool, sourceUUID); err != nil {
			return uuid.Nil, nil, err
		}
		return sourceUUID, nil, nil
	}
	source, err := s.sourceByIDAny(ctx, s.Pool, sourceUUID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	if source.Visibility != "public" {
		allowed, accessErr := canViewSource(ctx, s.Pool, sourceUUID, *actorUUID)
		if accessErr != nil {
			return uuid.Nil, nil, accessErr
		}
		if !allowed {
			return uuid.Nil, nil, ErrForbidden
		}
	}
	if manage {
		allowed, accessErr := canManageSource(ctx, s.Pool, sourceUUID, *actorUUID)
		if accessErr != nil {
			return uuid.Nil, nil, accessErr
		}
		if !allowed {
			return uuid.Nil, nil, ErrForbidden
		}
	}
	return sourceUUID, actorUUID, nil
}

func parseDependencyActor(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, ErrForbidden
	}
	return &parsed, nil
}

func dependencyActorArg(actorID *uuid.UUID) any {
	if actorID == nil {
		return nil
	}
	return *actorID
}

func (s *Service) loadSourceDependencyGraph(ctx context.Context, q dbExecutor, sourceID uuid.UUID, actorID *uuid.UUID) (SourceDependencyGraph, error) {
	rows, err := q.Query(ctx, `
		SELECT d.id, d.source_id, source.title_ar, d.depends_on_source_id, target.title_ar,
		       d.dependency_type, d.evidence_ar, d.status, d.algorithm_version, d.signal_data,
		       d.created_at, d.reviewed_by, d.reviewed_at, d.review_note_ar
		FROM source_dependencies d
		JOIN sources source ON source.id = d.source_id
		LEFT JOIN sources target ON target.id = d.depends_on_source_id
		WHERE d.source_id = $1
		  AND (target.id IS NULL OR target.visibility = 'public' OR ($2::uuid IS NOT NULL AND (target.created_by = $2 OR EXISTS (
		    SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')
		  ))))
		ORDER BY d.created_at DESC, d.id
	`, sourceID, dependencyActorArg(actorID))
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	defer rows.Close()
	items := make([]SourceDependencyView, 0)
	for rows.Next() {
		item, scanErr := scanSourceDependency(rows)
		if scanErr != nil {
			return SourceDependencyGraph{}, scanErr
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return SourceDependencyGraph{}, err
	}
	summary := SourceDependencySummary{}
	for _, item := range items {
		summary.Total++
		switch item.Status {
		case "needs_review":
			summary.NeedsReview++
		case "confirmed":
			summary.Confirmed++
		case "rejected":
			summary.Rejected++
		}
	}
	summary.IndependentSourceCount, err = countIndependentSources(ctx, q, sourceID, actorID)
	if err != nil {
		return SourceDependencyGraph{}, err
	}
	return SourceDependencyGraph{SourceID: sourceID.String(), Items: items, Summary: summary}, nil
}

func scanSourceDependency(row pgx.Row) (SourceDependencyView, error) {
	var item SourceDependencyView
	var id, sourceID, targetID, reviewedBy pgtype.UUID
	var targetTitle, evidence, algorithmVersion, reviewNote pgtype.Text
	var signalData []byte
	var createdAt time.Time
	var reviewedAt pgtype.Timestamptz
	if err := row.Scan(&id, &sourceID, &item.SourceTitleAR, &targetID, &targetTitle, &item.DependencyType, &evidence, &item.Status, &algorithmVersion, &signalData, &createdAt, &reviewedBy, &reviewedAt, &reviewNote); err != nil {
		return SourceDependencyView{}, err
	}
	item.ID = uuidString(id)
	item.SourceID = uuidString(sourceID)
	item.DependsOnSourceID = uuidString(targetID)
	item.DependsOnSourceTitleAR = textValue(targetTitle)
	item.EvidenceAR = textValue(evidence)
	item.AlgorithmVersion = textValue(algorithmVersion)
	item.CreatedAt = createdAt
	item.ReviewedBy = uuidString(reviewedBy)
	item.ReviewNoteAR = textValue(reviewNote)
	if reviewedAt.Valid {
		value := reviewedAt.Time
		item.ReviewedAt = &value
	}
	item.SignalData = map[string]any{}
	if len(signalData) > 0 {
		_ = json.Unmarshal(signalData, &item.SignalData)
		if item.SignalData == nil {
			item.SignalData = map[string]any{}
		}
	}
	return item, nil
}

func refreshSourceDependencyStatus(ctx context.Context, q dbExecutor, sourceID uuid.UUID) error {
	_, err := q.Exec(ctx, `
		UPDATE sources s
		SET dependency_status = CASE
		      WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'confirmed') THEN 'derived'
		      WHEN EXISTS (SELECT 1 FROM source_dependencies sd WHERE sd.source_id = s.id AND sd.status = 'needs_review') THEN 'likely_dependent'
		      WHEN s.dependency_status IN ('derived', 'likely_dependent') THEN 'unknown'
		      ELSE s.dependency_status
		    END,
		    updated_at = now()
		WHERE s.id = $1
	`, sourceID)
	return err
}

func countIndependentSources(ctx context.Context, q dbExecutor, sourceID uuid.UUID, actorID *uuid.UUID) (int, error) {
	var count int
	if err := q.QueryRow(ctx, `
		SELECT count(*)
		FROM sources candidate
		WHERE candidate.id <> $1
		  AND (candidate.visibility = 'public' OR ($2::uuid IS NOT NULL AND (candidate.created_by = $2 OR EXISTS (
		    SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')
		  ))))
		  AND NOT EXISTS (
		    SELECT 1 FROM source_dependencies d
		    WHERE d.status IN ('needs_review', 'confirmed')
		      AND ((d.source_id = $1 AND d.depends_on_source_id = candidate.id) OR (d.source_id = candidate.id AND d.depends_on_source_id = $1))
		  )
	`, sourceID, dependencyActorArg(actorID)).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func dependencyPassages(ctx context.Context, q dbExecutor, sourceID uuid.UUID) ([]dependencyPassage, bool, error) {
	rows, err := q.Query(ctx, `
		SELECT id, normalized_text_ar
		FROM source_passages
		WHERE source_id = $1 AND char_length(normalized_text_ar) >= $2
		ORDER BY sequence_number, id
		LIMIT $3
	`, sourceID, SourceDependencyMinPassageLength, SourceDependencyMaxSourcePassages+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]dependencyPassage, 0)
	for rows.Next() {
		var id pgtype.UUID
		var text string
		if err := rows.Scan(&id, &text); err != nil {
			return nil, false, err
		}
		items = append(items, dependencyPassage{ID: uuid.UUID(id.Bytes), Text: text})
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(items) > SourceDependencyMaxSourcePassages
	if truncated {
		items = items[:SourceDependencyMaxSourcePassages]
	}
	return items, truncated, nil
}

func detectDependencyCandidates(ctx context.Context, q dbExecutor, sourceID uuid.UUID, actorID *uuid.UUID, passages []dependencyPassage) ([]dependencyCandidate, bool, error) {
	candidates := make(map[uuid.UUID]*dependencyCandidate)
	candidatesTruncated := false
	if len(passages) > 0 {
		entries := make([]map[string]string, 0, len(passages))
		for _, passage := range passages {
			entries = append(entries, map[string]string{"id": passage.ID.String(), "text": passage.Text})
		}
		payload, err := json.Marshal(entries)
		if err != nil {
			return nil, false, err
		}
		rows, err := q.Query(ctx, `
			WITH selected AS (
				SELECT (entry->>'id')::uuid AS id, entry->>'text' AS normalized_text_ar
				FROM jsonb_array_elements($1::jsonb) AS entry
			)
			SELECT selected.id, candidate.id, candidate.source_id, s.title_ar,
			       similarity(candidate.normalized_text_ar, selected.normalized_text_ar) AS score
			FROM selected
			JOIN source_passages candidate ON candidate.source_id <> $2
			JOIN sources s ON s.id = candidate.source_id
			WHERE char_length(candidate.normalized_text_ar) >= $3
			  AND (s.visibility = 'public' OR ($4::uuid IS NOT NULL AND (s.created_by = $4 OR EXISTS (
			    SELECT 1 FROM user_roles ur WHERE ur.user_id = $4 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')
			  ))))
			  AND similarity(candidate.normalized_text_ar, selected.normalized_text_ar) >= $5
			ORDER BY score DESC, candidate.id
			LIMIT $6
		`, payload, sourceID, SourceDependencyMinPassageLength, dependencyActorArg(actorID), SourceDependencySimilarityThreshold, SourceDependencyMaxMatches)
		if err != nil {
			return nil, false, err
		}
		phraseMatchCount := 0
		for rows.Next() {
			var selectedID, candidateID, candidateSourceID pgtype.UUID
			var title string
			var score float64
			if err := rows.Scan(&selectedID, &candidateID, &candidateSourceID, &title, &score); err != nil {
				rows.Close()
				return nil, false, err
			}
			phraseMatchCount++
			candidateKey := uuid.UUID(candidateSourceID.Bytes)
			candidate := candidates[candidateKey]
			if candidate == nil {
				candidate = &dependencyCandidate{SourceID: candidateKey, SourceTitleAR: title}
				candidates[candidateKey] = candidate
			}
			if score > candidate.PhraseScore+0.000001 {
				candidate.PhraseScore = score
				candidate.MatchedPassageIDs = []string{uuid.UUID(selectedID.Bytes).String(), uuid.UUID(candidateID.Bytes).String()}
			} else if math.Abs(score-candidate.PhraseScore) <= 0.000001 && len(candidate.MatchedPassageIDs) < 10 {
				candidate.MatchedPassageIDs = append(candidate.MatchedPassageIDs, uuid.UUID(selectedID.Bytes).String(), uuid.UUID(candidateID.Bytes).String())
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, false, err
		}
		rows.Close()
		if phraseMatchCount >= SourceDependencyMaxMatches {
			candidatesTruncated = true
		}
	}
	sharedRows, err := q.Query(ctx, `
		WITH source_claims AS (
			SELECT DISTINCT ce.claim_id
			FROM claim_evidence ce
			LEFT JOIN source_statements ss ON ss.id = ce.source_statement_id
			LEFT JOIN source_passages sp ON sp.id = ce.source_passage_id
			WHERE COALESCE(ss.source_id, sp.source_id) = $1
			  AND (ss.id IS NULL OR ss.review_status = 'accepted')
		)
		SELECT other_source.source_id, other_source.title_ar, other_source.claim_id
		FROM (
			SELECT DISTINCT ce_other.claim_id, COALESCE(ss_other.source_id, sp_other.source_id) AS source_id, s.title_ar
			FROM source_claims sc
			JOIN claim_evidence ce_other ON ce_other.claim_id = sc.claim_id
			LEFT JOIN source_statements ss_other ON ss_other.id = ce_other.source_statement_id
			LEFT JOIN source_passages sp_other ON sp_other.id = ce_other.source_passage_id
			JOIN sources s ON s.id = COALESCE(ss_other.source_id, sp_other.source_id)
			WHERE COALESCE(ss_other.source_id, sp_other.source_id) <> $1
			  AND (ss_other.id IS NULL OR ss_other.review_status = 'accepted')
			  AND (s.visibility = 'public' OR ($2::uuid IS NOT NULL AND (s.created_by = $2 OR EXISTS (
			    SELECT 1 FROM user_roles ur WHERE ur.user_id = $2 AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')
			  ))))
		) other_source
		ORDER BY other_source.source_id, other_source.claim_id
		LIMIT $3
	`, sourceID, dependencyActorArg(actorID), SourceDependencyMaxSharedClaims)
	if err != nil {
		return nil, false, err
	}
	shared := make(map[uuid.UUID]*sharedClaimCandidate)
	sharedClaimCount := 0
	for sharedRows.Next() {
		var sourceIDValue pgtype.UUID
		var title, claimID string
		if err := sharedRows.Scan(&sourceIDValue, &title, &claimID); err != nil {
			sharedRows.Close()
			return nil, false, err
		}
		sharedClaimCount++
		candidateKey := uuid.UUID(sourceIDValue.Bytes)
		candidate := shared[candidateKey]
		if candidate == nil {
			candidate = &sharedClaimCandidate{SourceID: candidateKey, SourceTitleAR: title}
			shared[candidateKey] = candidate
		}
		if len(candidate.ClaimIDs) < 20 {
			candidate.ClaimIDs = append(candidate.ClaimIDs, claimID)
		}
	}
	if err := sharedRows.Err(); err != nil {
		sharedRows.Close()
		return nil, false, err
	}
	sharedRows.Close()
	if sharedClaimCount >= SourceDependencyMaxSharedClaims {
		candidatesTruncated = true
	}
	for sourceIDValue, sharedCandidate := range shared {
		if len(sharedCandidate.ClaimIDs) < 2 {
			continue
		}
		candidate := candidates[sourceIDValue]
		if candidate == nil {
			candidate = &dependencyCandidate{SourceID: sourceIDValue, SourceTitleAR: sharedCandidate.SourceTitleAR}
			candidates[sourceIDValue] = candidate
		}
		candidate.SharedClaimIDs = sharedCandidate.ClaimIDs
	}
	items := make([]dependencyCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		items = append(items, *candidate)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SourceID.String() < items[j].SourceID.String() })
	return items, candidatesTruncated, nil
}

func validateSourceDependencyInput(input CreateSourceDependencyInput) (CreateSourceDependencyInput, error) {
	input.DependsOnSourceID = strings.TrimSpace(input.DependsOnSourceID)
	input.DependencyType = strings.ToLower(strings.TrimSpace(input.DependencyType))
	input.EvidenceAR = strings.TrimSpace(input.EvidenceAR)
	if input.DependencyType == "" {
		input.DependencyType = "unknown"
	}
	if input.DependsOnSourceID == "" || len([]rune(input.EvidenceAR)) > 5000 || !validSourceDependencyType(input.DependencyType) {
		return CreateSourceDependencyInput{}, ErrValidation
	}
	if _, err := uuid.Parse(input.DependsOnSourceID); err != nil {
		return CreateSourceDependencyInput{}, ErrValidation
	}
	return input, nil
}

func validateSourceDependencyReviewInput(input ReviewSourceDependencyInput) (ReviewSourceDependencyInput, error) {
	input.Decision = strings.ToLower(strings.TrimSpace(input.Decision))
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	if (input.Decision != "confirmed" && input.Decision != "rejected") || len([]rune(input.NoteAR)) > 2000 {
		return ReviewSourceDependencyInput{}, ErrValidation
	}
	return input, nil
}

func validSourceDependencyType(value string) bool {
	switch value {
	case "cites", "derived_from", "likely_paraphrase", "shared_origin", "unknown":
		return true
	default:
		return false
	}
}
