package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// PageRegionEvidence identifies which independent page reading supplied a box.
type PageRegionEvidence struct {
	Method string `json:"method"`
	PageRegion
}

// PageEvidenceView is one authorized, physical-page evidence record. Coordinates
// use normalized top-left units and always refer to the original PDF page.
type PageEvidenceView struct {
	PageEvidenceID string               `json:"pageEvidenceId"`
	SourceFileID   string               `json:"sourceFileId"`
	PageNumber     int                  `json:"pageNumber"`
	Width          float64              `json:"width"`
	Height         float64              `json:"height"`
	Status         string               `json:"status"`
	SelectedMethod string               `json:"selectedMethod"`
	Text           string               `json:"text"`
	Readings       []PageReading        `json:"readings"`
	Regions        []PageRegionEvidence `json:"regions"`
	CurrentReview  *PageReviewRevision  `json:"currentReview,omitempty"`
	ReviewCount    int                  `json:"reviewCount"`
	CreatedAt      time.Time            `json:"createdAt"`
}

// PageReviewRevision is an append-only human transcription or unreadability
// decision. It stays separate from machine readings so corrections never erase
// what the OCR engines originally returned.
type PageReviewRevision struct {
	ID                   string    `json:"id"`
	SourcePageEvidenceID string    `json:"sourcePageEvidenceId"`
	RevisionNumber       int       `json:"revisionNumber"`
	Status               string    `json:"status"`
	TextAR               string    `json:"textAr"`
	NoteAR               string    `json:"noteAr,omitempty"`
	ReviewerID           string    `json:"reviewerId"`
	CreatedAt            time.Time `json:"createdAt"`
}

// PageEvidenceSummary is the reviewer-facing index for one physical page. It
// intentionally omits transcription text; the separately authorized page route
// returns the full readings when a reviewer opens the page.
type PageEvidenceSummary struct {
	PageNumber     int       `json:"pageNumber"`
	Width          float64   `json:"width"`
	Height         float64   `json:"height"`
	Status         string    `json:"status"`
	SelectedMethod string    `json:"selectedMethod"`
	ReviewCount    int       `json:"reviewCount"`
	CreatedAt      time.Time `json:"createdAt"`
}

type PageEvidenceList struct {
	SourceFileID string                `json:"sourceFileId"`
	Pages        []PageEvidenceSummary `json:"pages"`
}

// PageEvidenceIndex lists every physical page captured by the latest run. This
// makes unreadable pages visible even when they produced no candidate.
func (s *Service) PageEvidenceIndex(ctx context.Context, fileID, actorID string) (PageEvidenceList, error) {
	if err := s.databaseReady(); err != nil {
		return PageEvidenceList{}, err
	}
	fileUUID, actorUUID, err := parseIDs(fileID, actorID)
	if err != nil {
		return PageEvidenceList{}, err
	}
	var sourceID pgtype.UUID
	if err := s.Pool.QueryRow(ctx, `SELECT source_id FROM source_files WHERE id = $1`, fileUUID).Scan(&sourceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PageEvidenceList{}, ErrNotFound
		}
		return PageEvidenceList{}, err
	}
	allowed, err := canReviewSource(ctx, s.Pool, uuidValue(sourceID), actorUUID)
	if err != nil {
		return PageEvidenceList{}, err
	}
	if !allowed {
		return PageEvidenceList{}, ErrForbidden
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT page_number, page_width, page_height, extraction_status,
		       selected_method, review_count, created_at
		FROM (
			SELECT DISTINCT ON (p.page_number) p.id, p.page_number, p.page_width, p.page_height,
			       p.extraction_status, p.selected_method,
			       (SELECT count(*)::integer
			        FROM source_page_review_revisions r
			        JOIN source_page_evidence reviewed_page ON reviewed_page.id = r.source_page_evidence_id
			        WHERE reviewed_page.source_file_id = p.source_file_id AND reviewed_page.page_number = p.page_number) AS review_count,
			       p.created_at
			FROM source_page_evidence p
			WHERE p.source_file_id = $1
			ORDER BY p.page_number, p.created_at DESC, p.id DESC
		) latest
		ORDER BY page_number
	`, fileUUID)
	if err != nil {
		return PageEvidenceList{}, err
	}
	defer rows.Close()
	result := PageEvidenceList{SourceFileID: fileUUID.String(), Pages: []PageEvidenceSummary{}}
	for rows.Next() {
		var page PageEvidenceSummary
		if err := rows.Scan(&page.PageNumber, &page.Width, &page.Height, &page.Status, &page.SelectedMethod, &page.ReviewCount, &page.CreatedAt); err != nil {
			return PageEvidenceList{}, err
		}
		result.Pages = append(result.Pages, page)
	}
	return result, rows.Err()
}

// PageEvidence returns OCR/native text only after the same source review check
// that protects original-file downloads.
func (s *Service) PageEvidence(ctx context.Context, fileID, actorID string, pageNumber int) (PageEvidenceView, error) {
	if err := s.databaseReady(); err != nil {
		return PageEvidenceView{}, err
	}
	if pageNumber < 1 || pageNumber > MaxPages {
		return PageEvidenceView{}, ErrValidation
	}
	fileUUID, actorUUID, err := parseIDs(fileID, actorID)
	if err != nil {
		return PageEvidenceView{}, err
	}
	var sourceID, pageEvidenceID pgtype.UUID
	var item PageEvidenceView
	var readingsJSON, regionsJSON []byte
	if err := s.Pool.QueryRow(ctx, `
		SELECT p.id, p.source_id, p.source_file_id, p.page_number, p.page_width, p.page_height,
		       p.extraction_status, p.selected_method, p.text_ar, p.readings, p.regions, p.created_at
		FROM source_page_evidence p
		WHERE p.source_file_id = $1 AND p.page_number = $2
		ORDER BY p.created_at DESC, p.id DESC
		LIMIT 1
	`, fileUUID, pageNumber).Scan(&pageEvidenceID, &sourceID, &item.SourceFileID, &item.PageNumber, &item.Width, &item.Height, &item.Status, &item.SelectedMethod, &item.Text, &readingsJSON, &regionsJSON, &item.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PageEvidenceView{}, ErrNotFound
		}
		return PageEvidenceView{}, err
	}
	allowed, err := canReviewSource(ctx, s.Pool, uuidValue(sourceID), actorUUID)
	if err != nil {
		return PageEvidenceView{}, err
	}
	if !allowed {
		return PageEvidenceView{}, ErrForbidden
	}
	item.PageEvidenceID = uuidValue(pageEvidenceID).String()
	var review PageReviewRevision
	var reviewerID, reviewedEvidenceID pgtype.UUID
	var noteAR pgtype.Text
	err = s.Pool.QueryRow(ctx, `
		SELECT r.id, r.source_page_evidence_id, r.revision_number, r.status, r.transcription_text_ar, r.note_ar, r.reviewer_id, r.created_at
		FROM source_page_review_revisions r
		JOIN source_page_evidence reviewed_page ON reviewed_page.id = r.source_page_evidence_id
		WHERE reviewed_page.source_file_id = $1 AND reviewed_page.page_number = $2
		ORDER BY r.created_at DESC, r.id DESC
		LIMIT 1
	`, fileUUID, pageNumber).Scan(&review.ID, &reviewedEvidenceID, &review.RevisionNumber, &review.Status, &review.TextAR, &noteAR, &reviewerID, &review.CreatedAt)
	if err == nil {
		review.SourcePageEvidenceID = uuidValue(reviewedEvidenceID).String()
		review.ReviewerID = uuidString(reviewerID)
		review.NoteAR = textValue(noteAR)
		item.CurrentReview = &review
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return PageEvidenceView{}, err
	}
	if err := s.Pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM source_page_review_revisions WHERE source_page_evidence_id = $1
	`, uuidValue(pageEvidenceID)).Scan(&item.ReviewCount); err != nil {
		return PageEvidenceView{}, err
	}
	if err := json.Unmarshal(readingsJSON, &item.Readings); err != nil {
		return PageEvidenceView{}, err
	}
	if err := json.Unmarshal(regionsJSON, &item.Regions); err != nil {
		return PageEvidenceView{}, err
	}
	if item.Readings == nil {
		item.Readings = []PageReading{}
	}
	if item.Regions == nil {
		item.Regions = []PageRegionEvidence{}
	}
	return item, nil
}
