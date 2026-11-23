package regionreview

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type CaptionRevision struct {
	ID         string `json:"id"`
	NameAR     string `json:"nameAr"`
	ProposedBy string `json:"proposedBy"`
	Published  bool   `json:"published"`
}

func (s *Service) captionModerator(ctx context.Context, actorID, captionID string) (uuid.UUID, error) {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return reviewer, err
	}
	if !allowed {
		return reviewer, ErrForbidden
	}
	if _, err := uuid.Parse(captionID); err != nil {
		return reviewer, ErrValidation
	}
	var sourceID string
	err = s.Pool.QueryRow(ctx, `
		SELECT c.source_id::text FROM plate_captions c JOIN sources s ON s.id = c.source_id
		WHERE c.id = $1 AND c.scan_sha256 = s.metadata->>'file_sha256'`, captionID).Scan(&sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return reviewer, ErrNotFound
	}
	if err != nil {
		return reviewer, err
	}
	return reviewer, s.requirePlateReadable(ctx, sourceID, actorID)
}

func (s *Service) CaptionHistory(ctx context.Context, actorID, captionID string) ([]CaptionRevision, error) {
	if _, err := s.captionModerator(ctx, actorID, captionID); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT r.id::text, r.name_ar, COALESCE(u.display_name_ar, ''), r.published_at IS NOT NULL
		FROM plate_caption_revisions r JOIN users u ON u.id = r.proposed_by
		WHERE r.caption_id = $1 ORDER BY r.revision_order DESC LIMIT 50`, captionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowToStructByPos[CaptionRevision])
}

func (s *Service) ProposeCaption(ctx context.Context, actorID, captionID, name string) error {
	reviewer, err := s.captionModerator(ctx, actorID, captionID)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	if len([]rune(name)) > 200 || strings.IndexFunc(name, func(r rune) bool { return unicode.IsLetter(r) && unicode.In(r, unicode.Arabic) }) < 0 {
		return ErrValidation
	}
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO plate_caption_revisions(caption_id, name_ar, proposed_by) VALUES ($1, $2, $3)`, captionID, name, reviewer)
	return err
}

func (s *Service) PublishCaption(ctx context.Context, actorID, captionID, revisionID string) error {
	reviewer, err := s.captionModerator(ctx, actorID, captionID)
	if err != nil {
		return err
	}
	id, err := uuid.Parse(revisionID)
	if err != nil {
		return ErrValidation
	}
	tag, err := s.Pool.Exec(ctx, `
		UPDATE plate_caption_revisions SET published_at = now(), published_by = $3
		WHERE caption_id = $1 AND id = $2 AND published_at IS NULL`, captionID, id, reviewer)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
