package geography

import (
	"context"
	"errors"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxPlaceEvidenceRows bounds one dossier. The dossier of a place used to be
// read as "every evidence row the place owns", so its size grew with the
// corpus; a bounded window is what a folio can lay out, and the response says
// when it was cut.
const MaxPlaceEvidenceRows = 50

var (
	// ErrPlaceEvidenceForbidden is the answer to a caller who asked for the
	// moderation lane without holding its roles, anonymous included. The lane
	// is the same dossier read one level deeper, so its refusal says the role
	// and nothing else.
	ErrPlaceEvidenceForbidden = errors.New("place evidence moderation lane is moderator-readable")
)

// PlaceEvidenceRow is one dossier line: the passage that was read, where it
// sits in the source, the source itself, and the association it evidences. A
// review state is part of the row, because a pending row and an accepted row
// are different epistemic objects and the folio never blurs them.
type PlaceEvidenceRow struct {
	ID                  string `json:"id"`
	Status              string `json:"status"`
	PassageID           string `json:"passageId,omitempty"`
	PassageText         string `json:"passageText,omitempty"`
	PageNumber          *int   `json:"pageNumber,omitempty"`
	Locator             string `json:"locator,omitempty"`
	SourceID            string `json:"sourceId,omitempty"`
	SourceTitle         string `json:"sourceTitle,omitempty"`
	SourceEdition       string `json:"sourceEdition,omitempty"`
	SourceCitation      string `json:"sourceCitation,omitempty"`
	AssociationID       string `json:"associationId,omitempty"`
	AssociationEntity   string `json:"associationEntity,omitempty"`
	AssociationRelation string `json:"associationRelation,omitempty"`
	AssociationStatus   string `json:"associationStatus,omitempty"`
}

// PlaceEvidenceResponse is the bounded dossier for one place.
type PlaceEvidenceResponse struct {
	PlaceID   string             `json:"placeId"`
	Status    string             `json:"status"`
	Evidence  []PlaceEvidenceRow `json:"evidence"`
	Limit     int                `json:"limit"`
	Truncated bool               `json:"truncated"`
}

// PlaceEvidence reads one place's dossier under the reader's policy: the place
// itself must be readable, every row must sit in the accepted review state (or,
// for a moderator who asked, also in the pending lane), and the passage's
// owning source must be readable by the same instance — a public place does
// not launder a private source, because the folio would otherwise quote a page
// the reader is not allowed to open.
//
// A moderator's pending rows are a deeper read of the same records, not a
// second system: they render behind the moderation band in the same folio.
func (s *Service) PlaceEvidence(ctx context.Context, placeID string, actorID string, includeNeedsReview bool) (PlaceEvidenceResponse, error) {
	if s.Pool == nil {
		return PlaceEvidenceResponse{}, ErrDatabaseUnavailable
	}
	placeUUID, err := uuid.Parse(placeID)
	if err != nil {
		return PlaceEvidenceResponse{}, ErrValidation
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		actorUUID = uuid.Nil
	}
	statusFilter := "accepted"
	if includeNeedsReview {
		if actorUUID == uuid.Nil {
			return PlaceEvidenceResponse{}, ErrPlaceEvidenceForbidden
		}
		var allowed bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('moderator', 'admin'))`, actorUUID).Scan(&allowed); err != nil {
			return PlaceEvidenceResponse{}, err
		}
		if !allowed {
			return PlaceEvidenceResponse{}, ErrPlaceEvidenceForbidden
		}
		// An empty filter keeps both lanes: the moderation band separates them
		// in the folio by the row's own review_status.
		statusFilter = ""
	}

	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return PlaceEvidenceResponse{}, err
	}
	params := visibility.NewParams()
	placeReference := params.Add(placeUUID.String())
	limitReference := params.Add(MaxPlaceEvidenceRows + 1)
	statusReference := params.Add(statusFilter)

	rows, err := s.Pool.Query(ctx, `
		SELECT se.id, se.review_status,
		       COALESCE(sp.text_ar, se.evidence_text_ar, ''),
		       COALESCE(sp.id::text, ''), sp.page_number, COALESCE(sp.locator_ar, ''),
		       COALESCE(src.id::text, ''), COALESCE(src.title_ar, ''),
		       COALESCE(src.edition_ar, ''), COALESCE(src.citation_ar, ''),
		       COALESCE(ga.id::text, ''), ga.relation_type, ga.status,
		       COALESCE(ep.canonical_name_ar, ef.canonical_name_ar, et.canonical_name_ar, eb.canonical_name_ar, '')
		FROM spatial_evidence se
		JOIN geographic_associations ga ON ga.id = se.geographic_association_id
		JOIN places p ON p.id = ga.place_id
		JOIN source_passages sp ON sp.id = se.source_passage_id
		JOIN sources src ON src.id = sp.source_id
		LEFT JOIN people ep ON ga.entity_type = 'person' AND ep.id = ga.entity_id
		LEFT JOIN families ef ON ga.entity_type = 'family' AND ef.id = ga.entity_id
		LEFT JOIN tribes et ON ga.entity_type = 'tribe' AND et.id = ga.entity_id
		LEFT JOIN branches eb ON ga.entity_type = 'branch' AND eb.id = ga.entity_id
		WHERE ga.place_id = `+placeReference+`
		  AND `+policy.ReferencePredicate(params, "place", "p.id")+`
		  AND `+policy.SourcePredicate(params, "src.id")+`
		  AND (`+statusReference+` = '' OR se.review_status = `+statusReference+`)
		ORDER BY sp.sequence_number NULLS LAST, se.created_at, se.id
		LIMIT `+limitReference,
		params.Args()...)
	if err != nil {
		return PlaceEvidenceResponse{}, err
	}
	defer rows.Close()

	response := PlaceEvidenceResponse{
		PlaceID:  placeUUID.String(),
		Status:   "accepted",
		Evidence: []PlaceEvidenceRow{},
		Limit:    MaxPlaceEvidenceRows,
	}
	if statusFilter == "" {
		response.Status = "accepted+needs_review"
	}
	for rows.Next() {
		row := PlaceEvidenceRow{}
		var pageNumber pgtype.Int4
		var entityName pgtype.Text
		if err := rows.Scan(&row.ID, &row.Status, &row.PassageText,
			&row.PassageID, &pageNumber, &row.Locator,
			&row.SourceID, &row.SourceTitle, &row.SourceEdition, &row.SourceCitation,
			&row.AssociationID, &row.AssociationRelation, &row.AssociationStatus, &entityName); err != nil {
			return PlaceEvidenceResponse{}, err
		}
		if pageNumber.Valid {
			page := int(pageNumber.Int32)
			row.PageNumber = &page
		}
		row.AssociationEntity = textValue(entityName)
		response.Evidence = append(response.Evidence, row)
	}
	if err := rows.Err(); err != nil {
		return PlaceEvidenceResponse{}, err
	}
	// The row over the bound is the fact the response reports, not a fact it
	// ships: a folio that fits fifty lines gets fifty, and the flag.
	if len(response.Evidence) > MaxPlaceEvidenceRows {
		response.Evidence = response.Evidence[:MaxPlaceEvidenceRows]
		response.Truncated = true
	}
	return response, nil
}
