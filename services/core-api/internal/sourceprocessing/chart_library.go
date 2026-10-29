package sourceprocessing

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ChartLibraryItem keeps an uploaded chart discoverable before it becomes a
// digital tree. Public source titles are visible to visitors; file identifiers,
// review counts and drafts follow the existing canReviewSource policy.
type ChartLibraryItem struct {
	SourceID         string    `json:"sourceId"`
	TitleAR          string    `json:"titleAr"`
	Visibility       string    `json:"visibility"`
	CanReview        bool      `json:"canReview"`
	UpdatedAt        time.Time `json:"updatedAt"`
	FileID           string    `json:"fileId,omitempty"`
	Filename         string    `json:"filename,omitempty"`
	MimeType         string    `json:"mimeType,omitempty"`
	ProcessingStatus string    `json:"processingStatus,omitempty"`
	TotalNames       int       `json:"totalNames"`
	PendingNames     int       `json:"pendingNames"`
	DeferredNames    int       `json:"deferredNames"`
	ReviewedNames    int       `json:"reviewedNames"`
	ReadingStatus    string    `json:"readingStatus,omitempty"`
	DraftID          string    `json:"draftId,omitempty"`
	DraftState       string    `json:"draftState,omitempty"`
	TreeID           string    `json:"treeId,omitempty"`
}

func (s *Service) ListChartLibrary(ctx context.Context, actorID string) ([]ChartLibraryItem, error) {
	if err := s.databaseReady(); err != nil {
		return nil, err
	}
	var actor any
	if strings.TrimSpace(actorID) != "" {
		parsed, err := uuid.Parse(actorID)
		if err != nil {
			return nil, ErrForbidden
		}
		actor = parsed
	}
	rows, err := s.Pool.Query(ctx, `
		WITH chart_sources AS (
			SELECT s.*, COALESCE(s.created_by = $1::uuid OR EXISTS (
				SELECT 1 FROM user_roles ur WHERE ur.user_id = $1::uuid
				AND ur.role IN ('collaborator', 'researcher', 'moderator', 'admin')
			), false) AS can_review
			FROM sources s WHERE s.source_type = 'family_document'
		)
		SELECT s.id::text, s.title_ar, s.visibility, s.can_review, s.updated_at,
		       COALESCE(f.id::text, ''), COALESCE(f.original_filename_ar, ''),
		       COALESCE(f.mime_type, ''), COALESCE(f.processing_status, ''),
		       n.total, n.pending, n.deferred, n.reviewed,
		       COALESCE(reading.status, ''), COALESCE(d.id::text, ''),
		       COALESCE(d.state, ''), COALESCE(d.tree_id::text, '')
		FROM chart_sources s
		LEFT JOIN source_files f ON f.source_id = s.id AND s.can_review
		LEFT JOIN LATERAL (
			SELECT count(*) FILTER (WHERE superseded_at IS NULL) AS total,
			       count(*) FILTER (WHERE superseded_at IS NULL AND status = 'suggested') AS pending,
			       count(*) FILTER (WHERE superseded_at IS NULL AND status = 'deferred') AS deferred,
			       count(*) FILTER (WHERE superseded_at IS NULL AND status IN ('accepted', 'corrected', 'rejected')) AS reviewed
			FROM source_tree_suggestions WHERE source_file_id = f.id AND NOT localisation_pending
		) n ON true
		LEFT JOIN LATERAL (
			SELECT status FROM source_name_reading_runs WHERE source_file_id = f.id
			ORDER BY created_at DESC, id DESC LIMIT 1
		) reading ON true
		LEFT JOIN LATERAL (
			SELECT id, state, tree_id FROM source_tree_drafts WHERE source_file_id = f.id
			ORDER BY created_at DESC, id DESC LIMIT 1
		) d ON true
		WHERE s.visibility = 'public' OR s.can_review
		ORDER BY s.updated_at DESC, s.id, f.created_at DESC, f.id
	`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]ChartLibraryItem, 0)
	for rows.Next() {
		var item ChartLibraryItem
		if err := rows.Scan(&item.SourceID, &item.TitleAR, &item.Visibility, &item.CanReview, &item.UpdatedAt,
			&item.FileID, &item.Filename, &item.MimeType, &item.ProcessingStatus,
			&item.TotalNames, &item.PendingNames, &item.DeferredNames, &item.ReviewedNames,
			&item.ReadingStatus, &item.DraftID, &item.DraftState, &item.TreeID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
