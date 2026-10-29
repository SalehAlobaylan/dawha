package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const maxReviewRevision = 1_000_000

// SuggestionReopenInput identifies the decision revision the reviewer saw.
// A reopen is an appended event; it never deletes the decision it supersedes.
type SuggestionReopenInput struct {
	ExpectedRevision int    `json:"expectedRevision"`
	NoteAR           string `json:"noteAr,omitempty"`
}

type SuggestionDecisionRecord struct {
	ID                   string                        `json:"id"`
	ReviewerID           string                        `json:"reviewerId"`
	ReviewerLabelAR      string                        `json:"reviewerLabelAr,omitempty"`
	Decision             string                        `json:"decision"`
	SuggestedTextAR      string                        `json:"suggestedTextAr"`
	DecidedTextAR        string                        `json:"decidedTextAr,omitempty"`
	NoteAR               string                        `json:"noteAr,omitempty"`
	Revision             int                           `json:"revision"`
	GeometryRevision     *int                          `json:"geometryRevision,omitempty"`
	SupersedesDecisionID string                        `json:"supersedesDecisionId,omitempty"`
	ReviewedAt           string                        `json:"reviewedAt"`
	CandidateSnapshot    []SuggestionCandidateSnapshot `json:"candidateSnapshot,omitempty"`
	SelectedCandidate    string                        `json:"selectedCandidate,omitempty"`
	InputMethod          string                        `json:"inputMethod,omitempty"`
	NormalizerVersion    string                        `json:"normalizerVersion,omitempty"`
	CandidatePoolVersion string                        `json:"candidatePoolVersion,omitempty"`
}

type SuggestionReviewEvent struct {
	ID                 string          `json:"id"`
	Type               string          `json:"type"`
	ActorID            string          `json:"actorId"`
	ActorLabelAR       string          `json:"actorLabelAr,omitempty"`
	Revision           int             `json:"revision"`
	PreviousDecisionID string          `json:"previousDecisionId,omitempty"`
	DecisionID         string          `json:"decisionId,omitempty"`
	GeometryRevisionID string          `json:"geometryRevisionId,omitempty"`
	Metadata           json.RawMessage `json:"metadata,omitempty"`
	CreatedAt          string          `json:"createdAt"`
}

type SuggestionRegionRevision struct {
	ID              string          `json:"id"`
	Revision        int             `json:"revision"`
	PreviousRegion  Region          `json:"previousRegion"`
	Region          Region          `json:"region"`
	CoordinateSpace string          `json:"coordinateSpace"`
	AnalysisDPI     int             `json:"analysisDpi"`
	SourceChecksum  string          `json:"sourceChecksum"`
	FrameWidth      int             `json:"frameWidth"`
	FrameHeight     int             `json:"frameHeight"`
	Metadata        json.RawMessage `json:"metadata,omitempty"`
	CreatedAt       string          `json:"createdAt"`
}

type SuggestionLineageEdge struct {
	Kind      string `json:"kind"`
	ParentID  string `json:"parentId"`
	ChildID   string `json:"childId"`
	CreatedAt string `json:"createdAt"`
}

type SuggestionHistory struct {
	Suggestion      SuggestionView             `json:"suggestion"`
	OriginalRegion  Region                     `json:"originalRegion"`
	Decisions       []SuggestionDecisionRecord `json:"decisions"`
	Events          []SuggestionReviewEvent    `json:"events"`
	RegionRevisions []SuggestionRegionRevision `json:"regionRevisions"`
	Lineage         []SuggestionLineageEdge    `json:"lineage"`
	CanReopen       bool                       `json:"canReopen"`
	CanReviseRegion bool                       `json:"canReviseRegion"`
	CanSplit        bool                       `json:"canSplit"`
	CanMerge        bool                       `json:"canMerge"`
}

type NameReviewSummary struct {
	Total          int `json:"total"`
	Pending        int `json:"pending"`
	Deferred       int `json:"deferred"`
	Accepted       int `json:"accepted"`
	Corrected      int `json:"corrected"`
	Rejected       int `json:"rejected"`
	Decided        int `json:"decided"`
	Superseded     int `json:"superseded"`
	DecisionEvents int `json:"decisionEvents"`
}

// lockReviewRow serialises all state changes for one occurrence. It does not
// require the row to be pending: reopen and history inspection must be able to
// read a decided row under the same lock.
func (s *Service) lockReviewRow(ctx context.Context, tx pgx.Tx, id string) (uuid.UUID, string, int, bool, error) {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(253025)`); err != nil {
		return uuid.Nil, "", 0, false, err
	}
	var file uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT source_file_id FROM source_tree_suggestions WHERE id=$1`, id).Scan(&file); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, "", 0, false, ErrSuggestionNotFound
		}
		return uuid.Nil, "", 0, false, err
	}
	if _, err := tx.Exec(ctx, `SELECT id FROM source_files WHERE id=$1 FOR UPDATE`, file); err != nil {
		return uuid.Nil, "", 0, false, err
	}
	var status string
	var revision int
	var superseded bool
	var staged bool
	if err := tx.QueryRow(ctx, `SELECT status,review_revision,superseded_at IS NOT NULL,localisation_pending FROM source_tree_suggestions WHERE id=$1 FOR UPDATE`, id).Scan(&status, &revision, &superseded, &staged); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, "", 0, false, ErrSuggestionNotFound
		}
		return uuid.Nil, "", 0, false, err
	}
	if staged {
		return uuid.Nil, "", 0, false, ErrConflict
	}
	return file, status, revision, superseded, nil
}

func (s *Service) ReopenSuggestion(ctx context.Context, suggestionID, actorID string, input SuggestionReopenInput) (SuggestionView, error) {
	if s == nil || s.Pool == nil {
		return SuggestionView{}, ErrDatabaseUnavailable
	}
	parsed, err := uuid.Parse(suggestionID)
	if err != nil {
		return SuggestionView{}, ErrSuggestionNotFound
	}
	if input.ExpectedRevision < 0 || input.ExpectedRevision > maxReviewRevision || len(input.NoteAR) > 2000 {
		return SuggestionView{}, ErrValidation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SuggestionView{}, err
	}
	defer tx.Rollback(ctx)
	file, status, revision, superseded, err := s.lockReviewRow(ctx, tx, suggestionID)
	if err != nil {
		return SuggestionView{}, err
	}
	if superseded {
		return SuggestionView{}, ErrSuggestionSuperseded
	}
	allowed, err := s.canReview(ctx, file, actorID)
	if err != nil {
		return SuggestionView{}, err
	}
	if !allowed {
		return SuggestionView{}, ErrForbidden
	}
	if status == "suggested" {
		return SuggestionView{}, ErrConflict
	}
	if input.ExpectedRevision != revision {
		return SuggestionView{}, ErrConflict
	}
	var previous string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM source_tree_suggestion_decisions WHERE suggestion_id=$1 AND review_revision=$2`, suggestionID, revision).Scan(&previous); errors.Is(err, pgx.ErrNoRows) {
		return SuggestionView{}, ErrConflict
	} else if err != nil {
		return SuggestionView{}, err
	}
	// Revision zero belongs to a never-reviewed form. Retire that token when
	// reopening a pre-migration decision without modifying its ledger row.
	openRevision := revision
	if openRevision == 0 {
		openRevision = 1
	}
	tag, err := tx.Exec(ctx, `
		UPDATE source_tree_suggestions
		SET status='suggested', decided_text_ar=NULL, decided_by=NULL, decided_at=NULL,
		    decision_note_ar=NULL, review_revision=$3, updated_at=now()
		WHERE id=$1 AND review_revision=$2 AND superseded_at IS NULL
	`, parsed, revision, openRevision)
	if err != nil {
		return SuggestionView{}, err
	}
	if tag.RowsAffected() != 1 {
		return SuggestionView{}, ErrConflict
	}
	if _, err = tx.Exec(ctx, `INSERT INTO source_name_review_events(suggestion_id,actor_id,event_type,revision,previous_decision_id,metadata) VALUES($1,$2,'reopened',$3,$4,$5::jsonb)`, parsed, actorID, openRevision, previous, mustJSON(map[string]string{"note": strings.TrimSpace(input.NoteAR)})); err != nil {
		return SuggestionView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return SuggestionView{}, err
	}
	return s.getSuggestion(ctx, parsed)
}

func (s *Service) SuggestionHistory(ctx context.Context, suggestionID, actorID string) (SuggestionHistory, error) {
	result := SuggestionHistory{Decisions: []SuggestionDecisionRecord{}, Events: []SuggestionReviewEvent{}, RegionRevisions: []SuggestionRegionRevision{}, Lineage: []SuggestionLineageEdge{}}
	view, err := s.getSuggestionForActor(ctx, suggestionID, actorID)
	if err != nil {
		return result, err
	}
	result.Suggestion = view
	result.OriginalRegion = view.Region
	result.CanReopen = !view.Superseded && view.Status != "suggested"
	result.CanReviseRegion = !view.Superseded && view.Status == "suggested"
	result.CanSplit = !view.Superseded && view.Status == "suggested"
	result.CanMerge = !view.Superseded

	rows, err := s.Pool.Query(ctx, `
		SELECT d.id::text, d.reviewer_id::text, COALESCE(u.display_name_ar,''), d.decision,
		       d.suggested_text_ar, COALESCE(d.decided_text_ar,''), COALESCE(d.note_ar,''),
		       d.review_revision, d.geometry_revision, COALESCE(d.supersedes_decision_id::text,''),
		       d.created_at::text, d.candidate_snapshot, COALESCE(d.selected_candidate,''),
		       d.input_method, d.normalizer_version, d.candidate_pool_version
		FROM source_tree_suggestion_decisions d
		LEFT JOIN users u ON u.id=d.reviewer_id
		WHERE d.suggestion_id=$1
		ORDER BY d.created_at, d.id
	`, suggestionID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item SuggestionDecisionRecord
		var snapshot []byte
		if err = rows.Scan(&item.ID, &item.ReviewerID, &item.ReviewerLabelAR, &item.Decision,
			&item.SuggestedTextAR, &item.DecidedTextAR, &item.NoteAR, &item.Revision,
			&item.GeometryRevision, &item.SupersedesDecisionID, &item.ReviewedAt, &snapshot, &item.SelectedCandidate,
			&item.InputMethod, &item.NormalizerVersion, &item.CandidatePoolVersion); err != nil {
			rows.Close()
			return result, err
		}
		if len(snapshot) > 0 {
			if json.Unmarshal(snapshot, &item.CandidateSnapshot) != nil {
				rows.Close()
				return result, ErrValidation
			}
		}
		result.Decisions = append(result.Decisions, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}

	rows, err = s.Pool.Query(ctx, `
		SELECT e.id::text, e.event_type, e.actor_id::text, COALESCE(u.display_name_ar,''),
		       e.revision, COALESCE(e.previous_decision_id::text,''), COALESCE(e.decision_id::text,''),
		       COALESCE(e.geometry_revision_id::text,''), e.metadata, e.created_at::text
		FROM source_name_review_events e
		LEFT JOIN users u ON u.id=e.actor_id
		WHERE e.suggestion_id=$1
		ORDER BY e.created_at, e.id
	`, suggestionID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item SuggestionReviewEvent
		var metadata []byte
		if err = rows.Scan(&item.ID, &item.Type, &item.ActorID, &item.ActorLabelAR, &item.Revision,
			&item.PreviousDecisionID, &item.DecisionID, &item.GeometryRevisionID, &metadata, &item.CreatedAt); err != nil {
			rows.Close()
			return result, err
		}
		if len(metadata) > 0 {
			item.Metadata = json.RawMessage(metadata)
		}
		result.Events = append(result.Events, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}

	rows, err = s.Pool.Query(ctx, `
		SELECT r.id::text, r.revision, r.previous_region, r.region, r.coordinate_space,
		       r.analysis_dpi, r.source_checksum, r.frame_width, r.frame_height, r.created_at::text
		FROM source_name_region_revisions r
		WHERE r.suggestion_id=$1
		ORDER BY r.revision
	`, suggestionID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item SuggestionRegionRevision
		var previous, region []byte
		if err = rows.Scan(&item.ID, &item.Revision, &previous, &region, &item.CoordinateSpace,
			&item.AnalysisDPI, &item.SourceChecksum, &item.FrameWidth, &item.FrameHeight, &item.CreatedAt); err != nil {
			rows.Close()
			return result, err
		}
		if json.Unmarshal(previous, &item.PreviousRegion) != nil || json.Unmarshal(region, &item.Region) != nil {
			rows.Close()
			return result, ErrValidation
		}
		result.RegionRevisions = append(result.RegionRevisions, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return result, err
	}
	if len(result.RegionRevisions) > 0 {
		result.OriginalRegion = result.RegionRevisions[0].PreviousRegion
	}

	rows, err = s.Pool.Query(ctx, `
		SELECT kind, parent_id::text, child_id::text, created_at::text
		FROM source_name_occurrence_lineage
		WHERE parent_id=$1 OR child_id=$1
		ORDER BY created_at, id
	`, suggestionID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item SuggestionLineageEdge
		if err = rows.Scan(&item.Kind, &item.ParentID, &item.ChildID, &item.CreatedAt); err != nil {
			rows.Close()
			return result, err
		}
		result.Lineage = append(result.Lineage, item)
	}
	rows.Close()
	return result, rows.Err()
}

func (s *Service) getSuggestionForActor(ctx context.Context, suggestionID, actorID string) (SuggestionView, error) {
	if _, err := s.loadSuggestionContext(ctx, suggestionID, actorID, false); err != nil {
		return SuggestionView{}, err
	}
	parsed, err := uuid.Parse(suggestionID)
	if err != nil {
		return SuggestionView{}, ErrSuggestionNotFound
	}
	return s.getSuggestion(ctx, parsed)
}

func (s *Service) NameReviewSummary(ctx context.Context, fileID, actorID string) (NameReviewSummary, error) {
	if s == nil || s.Pool == nil {
		return NameReviewSummary{}, ErrDatabaseUnavailable
	}
	file, err := uuid.Parse(fileID)
	if err != nil {
		return NameReviewSummary{}, ErrNotFound
	}
	allowed, err := s.canReview(ctx, file, actorID)
	if err != nil {
		return NameReviewSummary{}, err
	}
	if !allowed {
		return NameReviewSummary{}, ErrForbidden
	}
	var result NameReviewSummary
	err = s.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status='suggested' AND superseded_at IS NULL),
			COUNT(*) FILTER (WHERE status='deferred' AND superseded_at IS NULL),
			COUNT(*) FILTER (WHERE status='accepted' AND superseded_at IS NULL),
			COUNT(*) FILTER (WHERE status='corrected' AND superseded_at IS NULL),
			COUNT(*) FILTER (WHERE status='rejected' AND superseded_at IS NULL),
			COUNT(*) FILTER (WHERE status IN ('accepted','corrected','rejected') AND superseded_at IS NULL),
			COUNT(*) FILTER (WHERE superseded_at IS NOT NULL),
			(SELECT COUNT(*) FROM source_tree_suggestion_decisions d JOIN source_tree_suggestions s2 ON s2.id=d.suggestion_id WHERE s2.source_file_id=$1 AND (d.review_revision > 1 OR (d.review_revision = 1 AND EXISTS (SELECT 1 FROM source_tree_suggestion_decisions prev WHERE prev.suggestion_id=d.suggestion_id AND prev.review_revision=0))))
		FROM source_tree_suggestions WHERE source_file_id=$1 AND NOT localisation_pending
	`, file).Scan(&result.Total, &result.Pending, &result.Deferred, &result.Accepted, &result.Corrected, &result.Rejected, &result.Decided, &result.Superseded, &result.DecisionEvents)
	return result, err
}
