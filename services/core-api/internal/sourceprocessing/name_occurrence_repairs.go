package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Splitting and merging change the geometry of a physical occurrence. They are
// appended operations: the parent rows keep their readings and decisions, are
// marked superseded for queue/evaluation purposes, and every child points back
// through source_name_occurrence_lineage. A child never inherits a human answer,
// so no reviewer text is fabricated into a new observation.

type SuggestionSplitInput struct {
	ExpectedReviewRevision   int      `json:"expectedReviewRevision"`
	ExpectedGeometryRevision int      `json:"expectedGeometryRevision"`
	Regions                  []Region `json:"regions"`
	NoteAR                   string   `json:"noteAr,omitempty"`
}

type SuggestionSplitResult struct {
	Parent   SuggestionView   `json:"parent"`
	Children []SuggestionView `json:"children"`
}

type SuggestionMergeMember struct {
	ID                       string `json:"id"`
	ExpectedReviewRevision   int    `json:"expectedReviewRevision"`
	ExpectedGeometryRevision int    `json:"expectedGeometryRevision"`
}

type SuggestionMergeInput struct {
	Members []SuggestionMergeMember `json:"members"`
	Region  Region                  `json:"region"`
	NoteAR  string                  `json:"noteAr,omitempty"`
}

type SuggestionMergeResult struct {
	Merged     SuggestionView   `json:"merged"`
	Superseded []SuggestionView `json:"superseded"`
}

func newDerivedOccurrenceID(prefix string) string {
	return prefix + "-" + uuid.NewString()
}

func (s *Service) insertDerivedOccurrence(ctx context.Context, tx pgx.Tx, parentID, occurrenceID string, region Region) (uuid.UUID, error) {
	regionJSON, err := json.Marshal(region)
	if err != nil {
		return uuid.Nil, ErrValidation
	}
	var child uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO source_tree_suggestions (
			source_id, source_file_id, source_page_evidence_id, processing_run_id,
			occurrence_id, page_number, kind, suggested_text_ar, region, alternatives,
			confidence, view_count, reading_method, reading_model, status, updated_at
		)
		SELECT source_id, source_file_id, source_page_evidence_id, processing_run_id,
		       $2, page_number, 'name', '', $3::jsonb, '[]'::jsonb,
		       0, 1, reading_method, reading_model, 'suggested', now()
		FROM source_tree_suggestions WHERE id=$1
		RETURNING id
	`, parentID, occurrenceID, regionJSON).Scan(&child)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrSuggestionNotFound
	}
	return child, err
}

// SplitSuggestion retires one pending detection and appends two or more child
// occurrences. The child rectangles must lie inside the parent and must not
// overlap each other: overlapping children would count the same ink twice.
func (s *Service) SplitSuggestion(ctx context.Context, suggestionID, actorID string, input SuggestionSplitInput) (SuggestionSplitResult, error) {
	result := SuggestionSplitResult{Children: []SuggestionView{}}
	if len(input.Regions) < 2 || len(input.Regions) > 16 || len(input.NoteAR) > 2000 {
		return result, ErrValidation
	}
	for index, region := range input.Regions {
		if err := validateNormalizedRegion(region); err != nil {
			return result, err
		}
		for previous := 0; previous < index; previous++ {
			if regionsOverlap(region, input.Regions[previous]) {
				return result, ErrValidation
			}
		}
	}
	parsed, err := uuid.Parse(suggestionID)
	if err != nil {
		return result, ErrSuggestionNotFound
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	file, status, revision, superseded, err := s.lockReviewRow(ctx, tx, suggestionID)
	if err != nil {
		return result, err
	}
	if superseded {
		return result, ErrSuggestionSuperseded
	}
	allowed, err := s.canReview(ctx, file, actorID)
	if err != nil {
		return result, err
	}
	if !allowed {
		return result, ErrForbidden
	}
	if status != "suggested" {
		return result, ErrSuggestionAlreadyDecided
	}
	if revision != input.ExpectedReviewRevision {
		return result, ErrConflict
	}
	var parentRegionJSON []byte
	var geometryRevision int
	if err = tx.QueryRow(ctx, `SELECT region, geometry_revision FROM source_tree_suggestions WHERE id=$1 FOR UPDATE`, parsed).Scan(&parentRegionJSON, &geometryRevision); err != nil {
		return result, ErrSuggestionNotFound
	}
	if geometryRevision != input.ExpectedGeometryRevision {
		return result, ErrConflict
	}
	var parentRegion Region
	if err = json.Unmarshal(parentRegionJSON, &parentRegion); err != nil {
		return result, ErrValidation
	}
	for _, region := range input.Regions {
		if !regionContains(parentRegion, region) {
			return result, ErrValidation
		}
	}

	children := make([]uuid.UUID, 0, len(input.Regions))
	for _, region := range input.Regions {
		child, insertErr := s.insertDerivedOccurrence(ctx, tx, suggestionID, newDerivedOccurrenceID("split"), region)
		if insertErr != nil {
			return result, insertErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO source_name_occurrence_lineage(parent_id, child_id, kind) VALUES($1,$2,'split')`, parsed, child); err != nil {
			return result, err
		}
		children = append(children, child)
	}
	metadata, err := json.Marshal(map[string]any{
		"children": children,
		"regions":  input.Regions,
		"note":     strings.TrimSpace(input.NoteAR),
	})
	if err != nil {
		return result, ErrValidation
	}
	var eventID string
	if err = tx.QueryRow(ctx, `
		INSERT INTO source_name_review_events (suggestion_id, actor_id, event_type, revision, metadata)
		VALUES ($1,$2,'split',$3,$4::jsonb) RETURNING id::text
	`, parsed, actorID, revision, metadata).Scan(&eventID); err != nil {
		return result, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE source_tree_suggestions
		SET superseded_at=clock_timestamp(), superseded_by_event_id=$2, updated_at=now()
		WHERE id=$1 AND superseded_at IS NULL AND review_revision=$3
	`, parsed, eventID, revision)
	if err != nil {
		return result, err
	}
	if tag.RowsAffected() != 1 {
		return result, ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	result.Parent, err = s.getSuggestion(ctx, parsed)
	if err != nil {
		return result, err
	}
	for _, child := range children {
		view, viewErr := s.getSuggestion(ctx, child)
		if viewErr != nil {
			return result, viewErr
		}
		result.Children = append(result.Children, view)
	}
	return result, nil
}

// MergeSuggestions retires two or more occurrences and appends one merged child.
// The merged region must contain every member rectangle; the callers cannot make
// a child that silently drops ink from one of its parents.
func (s *Service) MergeSuggestions(ctx context.Context, suggestionID, actorID string, input SuggestionMergeInput) (SuggestionMergeResult, error) {
	result := SuggestionMergeResult{Superseded: []SuggestionView{}}
	if len(input.Members) < 2 || len(input.Members) > 16 || len(input.NoteAR) > 2000 {
		return result, ErrValidation
	}
	if err := validateNormalizedRegion(input.Region); err != nil {
		return result, err
	}
	seen := make(map[string]struct{}, len(input.Members))
	members := make([]SuggestionMergeMember, 0, len(input.Members))
	for _, member := range input.Members {
		if _, err := uuid.Parse(member.ID); err != nil || member.ExpectedReviewRevision < 0 || member.ExpectedGeometryRevision < 0 {
			return result, ErrValidation
		}
		if _, exists := seen[member.ID]; exists {
			return result, ErrValidation
		}
		seen[member.ID] = struct{}{}
		members = append(members, member)
	}
	if _, ok := seen[suggestionID]; !ok {
		return result, ErrValidation
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)

	var file uuid.UUID
	var pageNumber int
	var leadID uuid.UUID
	for index, member := range members {
		memberFile, status, revision, superseded, lockErr := s.lockReviewRow(ctx, tx, member.ID)
		if lockErr != nil {
			return result, lockErr
		}
		if superseded {
			return result, ErrSuggestionSuperseded
		}
		if memberFile != file && index > 0 {
			return result, ErrValidation
		}
		file = memberFile
		if revision != member.ExpectedReviewRevision {
			return result, ErrConflict
		}
		var memberRegionJSON []byte
		var geometryRevision, memberPage int
		if err = tx.QueryRow(ctx, `SELECT region, geometry_revision, page_number FROM source_tree_suggestions WHERE id=$1 FOR UPDATE`, member.ID).Scan(&memberRegionJSON, &geometryRevision, &memberPage); err != nil {
			return result, ErrSuggestionNotFound
		}
		if status != "suggested" && status != SuggestionAccepted && status != SuggestionCorrected && status != SuggestionRejected && status != SuggestionDeferred {
			return result, ErrValidation
		}
		if geometryRevision != member.ExpectedGeometryRevision {
			return result, ErrConflict
		}
		if index == 0 {
			pageNumber = memberPage
			leadID = uuid.MustParse(member.ID)
		} else if memberPage != pageNumber {
			return result, ErrValidation
		}
		var memberRegion Region
		if err = json.Unmarshal(memberRegionJSON, &memberRegion); err != nil {
			return result, ErrValidation
		}
		if !regionContains(input.Region, memberRegion) {
			return result, ErrValidation
		}
	}
	allowed, err := s.canReview(ctx, file, actorID)
	if err != nil {
		return result, err
	}
	if !allowed {
		return result, ErrForbidden
	}
	child, err := s.insertDerivedOccurrence(ctx, tx, leadID.String(), newDerivedOccurrenceID("merge"), input.Region)
	if err != nil {
		return result, err
	}
	memberIDs := make([]uuid.UUID, 0, len(members))
	for _, member := range members {
		parsed, parseErr := uuid.Parse(member.ID)
		if parseErr != nil {
			return result, ErrValidation
		}
		memberIDs = append(memberIDs, parsed)
		if _, err = tx.Exec(ctx, `INSERT INTO source_name_occurrence_lineage(parent_id, child_id, kind) VALUES($1,$2,'merge')`, parsed, child); err != nil {
			return result, err
		}
	}
	for _, member := range members {
		parsed := uuid.MustParse(member.ID)
		metadata, marshalErr := json.Marshal(map[string]any{
			"childId": child,
			"members": memberIDs,
			"region":  input.Region,
			"note":    strings.TrimSpace(input.NoteAR),
		})
		if marshalErr != nil {
			return result, ErrValidation
		}
		var eventID string
		if err = tx.QueryRow(ctx, `
			INSERT INTO source_name_review_events (suggestion_id, actor_id, event_type, revision, metadata)
			VALUES ($1,$2,'merge',$3,$4::jsonb) RETURNING id::text
		`, parsed, actorID, member.ExpectedReviewRevision, metadata).Scan(&eventID); err != nil {
			return result, err
		}
		tag, updateErr := tx.Exec(ctx, `
			UPDATE source_tree_suggestions
			SET superseded_at=clock_timestamp(), superseded_by_event_id=$2, updated_at=now()
			WHERE id=$1 AND superseded_at IS NULL AND review_revision=$3
		`, parsed, eventID, member.ExpectedReviewRevision)
		if updateErr != nil {
			return result, updateErr
		}
		if tag.RowsAffected() != 1 {
			return result, ErrConflict
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	result.Merged, err = s.getSuggestion(ctx, child)
	if err != nil {
		return result, err
	}
	for _, member := range members {
		view, viewErr := s.getSuggestion(ctx, uuid.MustParse(member.ID))
		if viewErr != nil {
			return result, viewErr
		}
		result.Superseded = append(result.Superseded, view)
	}
	return result, nil
}
