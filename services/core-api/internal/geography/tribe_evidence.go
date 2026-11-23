// Tribe evidence is the receipt folio for one tribal dossier (plan 031-A).
// The contract mirrors place_evidence verbatim — accepted-only default,
// SourcePredicate on everything quoted, one bounded window with a truncated
// flag, and the moderation lane reading the same records one level deeper —
// over the two receipt lanes a tribe owns:
//
//   - spatial receipts: spatial_evidence rows behind the tribe's
//     geographic_associations (the dira/place receipts the atlas and the
//     dossier share), sourced through their passage when one exists, otherwise
//     through the association's own source id;
//   - claim receipts: the tribe's claims' evidence rows (claim_evidence), each
//     tied to its passage or, for statement-backed evidence, through the
//     statement to its source.
//
// A receipt with no source at all is a platform record and stays (the same
// rule the person pages keep): visibility limits what may be QUOTED, and a
// row that quotes nothing quotes nothing.
package geography

import (
	"context"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// MaxTribeEvidenceRows bounds one receipt folio.
const MaxTribeEvidenceRows = 50

type TribeEvidenceRow struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	PassageID   string `json:"passageId,omitempty"`
	PassageText string `json:"passageText,omitempty"`
	PageNumber  *int   `json:"pageNumber,omitempty"`
	Locator     string `json:"locator,omitempty"`
	SourceID    string `json:"sourceId,omitempty"`
	SourceTitle string `json:"sourceTitle,omitempty"`
	// AssociationID/Relation carry the spatial lane's association pointer and
	// its relation_type; Relation doubles for the claim lane's evidence
	// relation (supports / contextualizes / contradicts / refutes).
	AssociationID string `json:"associationId,omitempty"`
	Relation      string `json:"relation,omitempty"`
	ClaimID       string `json:"claimId,omitempty"`
	ClaimStatus   string `json:"claimStatus,omitempty"`
	ClaimNotes    string `json:"claimNotes,omitempty"`
	CreatedAt     string `json:"createdAt"`
}

type TribeEvidenceResponse struct {
	TribeID   string             `json:"tribeId"`
	Status    string             `json:"status"`
	Evidence  []TribeEvidenceRow `json:"evidence"`
	Limit     int                `json:"limit"`
	Truncated bool               `json:"truncated"`
}

// TribeEvidence reads one tribe's receipts under the reader's policy. The
// accepted lane is the default and quotes only accepted review states on
// readable sources; a moderator's include=needs_review lane is the same
// records one level deeper.
func (s *Service) TribeEvidence(ctx context.Context, tribeID string, actorID string, includeNeedsReview bool) (TribeEvidenceResponse, error) {
	if s.Pool == nil {
		return TribeEvidenceResponse{}, ErrDatabaseUnavailable
	}
	tribeUUID, err := uuid.Parse(tribeID)
	if err != nil {
		return TribeEvidenceResponse{}, ErrValidation
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil {
		actorUUID = uuid.Nil
	}
	statusFilter := "accepted"
	if includeNeedsReview {
		if actorUUID == uuid.Nil {
			return TribeEvidenceResponse{}, ErrPlaceEvidenceForbidden
		}
		var allowed bool
		if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('moderator', 'admin'))`, actorUUID).Scan(&allowed); err != nil {
			return TribeEvidenceResponse{}, err
		}
		if !allowed {
			return TribeEvidenceResponse{}, ErrPlaceEvidenceForbidden
		}
		// An empty filter keeps both lanes; the moderation band separates them
		// in the folio by the row's own status.
		statusFilter = ""
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return TribeEvidenceResponse{}, err
	}
	params := visibility.NewParams()
	tribeReference := params.Add(tribeUUID.String())
	// The two lanes' acceptance states differ by vocabulary: a spatial
	// receipt's acceptance is its review_status, a claim's is its own status
	// cycle (documented / supported are its reviewed-positive states). One
	// shared filter word, two lane readings of it.
	spacialStatusReference := params.Add(statusFilter)
	claimStatusFilterReference := params.Add("")
	claimStatusesReference := params.Add("{documented,supported}")
	// The spatial lane's two visibility walls, the place folio's own: the
	// place the association points at must be readable, and the receipt's
	// owning source (its passage's, else the association's) must be too — a
	// private source is not laundered through a public tribe.
	placeVisibleReference := policy.ReferencePredicate(params, "place", "p.id")
	spatialSourceReference := policy.SourcePredicate(params, "src.id")
	claimSourceReference := policy.SourcePredicate(params, "src.id")
	limitReference := params.Add(MaxTribeEvidenceRows + 1)
	claimPredicate := policy.ClaimPredicate(params, "c.id")

	rows, err := s.Pool.Query(ctx, `
		SELECT * FROM (
			SELECT se.id::text AS id, 'spatial_evidence' AS kind, se.review_status AS status,
			       COALESCE(sp.text_ar, se.evidence_text_ar, '') AS passage_text,
			       COALESCE(sp.id::text, '') AS passage_id,
			       sp.page_number,
			       COALESCE(sp.locator_ar, '') AS locator,
			       COALESCE(src.id::text, '') AS source_id,
			       COALESCE(src.title_ar, '') AS source_title,
			       COALESCE(ga.id::text, '') AS association_id,
			       ga.relation_type AS relation,
			       ''::text AS claim_id, ''::text AS claim_status, ''::text AS claim_notes,
			       se.created_at
			FROM spatial_evidence se
			JOIN geographic_associations ga ON ga.id = se.geographic_association_id
			JOIN places p ON p.id = ga.place_id
			LEFT JOIN source_passages sp ON sp.id = se.source_passage_id
			LEFT JOIN sources src ON src.id = COALESCE(sp.source_id, ga.source_id)
			WHERE ga.entity_type = 'tribe' AND ga.entity_id = `+tribeReference+`
			   AND `+placeVisibleReference+`
			   AND (src.id IS NULL OR `+spatialSourceReference+`)
			  AND (`+spacialStatusReference+` = '' OR se.review_status = `+spacialStatusReference+`)

			UNION ALL

			SELECT c.id::text, 'claim_evidence', c.status,
			       COALESCE(sp.text_ar, ce.evidence_note_ar, ''),
			       COALESCE(sp.id::text, ''),
			       sp.page_number,
			       COALESCE(sp.locator_ar, ''),
			       COALESCE(src.id::text, ''),
			       COALESCE(src.title_ar, ''),
			       ''::text,
			       ce.relation,
			       c.id::text, c.status, COALESCE(c.notes_ar, ''),
			       c.created_at
			FROM claims c
			JOIN claim_evidence ce ON ce.claim_id = c.id
			LEFT JOIN source_passages sp ON sp.id = ce.source_passage_id
			LEFT JOIN source_statements ss ON ss.id = ce.source_statement_id
			LEFT JOIN sources src ON src.id = COALESCE(sp.source_id, ss.source_id)
			WHERE ((c.subject_type = 'tribe' AND c.subject_id = `+tribeReference+`)
			    OR (c.object_type = 'tribe' AND c.object_id = `+tribeReference+`))
			  AND (`+claimStatusFilterReference+` = '' OR c.status = ANY(`+claimStatusesReference+`::text[]))
			  AND (src.id IS NULL OR `+claimSourceReference+`)
			  AND `+claimPredicate+`
		) receipts
		ORDER BY receipts.created_at, receipts.id
		LIMIT `+limitReference, params.Args()...)
	if err != nil {
		return TribeEvidenceResponse{}, err
	}
	defer rows.Close()

	response := TribeEvidenceResponse{
		TribeID:  tribeUUID.String(),
		Status:   "accepted",
		Evidence: []TribeEvidenceRow{},
		Limit:    MaxTribeEvidenceRows,
	}
	if includeNeedsReview {
		response.Status = "accepted+needs_review"
	}
	for rows.Next() {
		var row TribeEvidenceRow
		var pageNumber pgtype.Int4
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&row.ID, &row.Kind, &row.Status, &row.PassageText, &row.PassageID,
			&pageNumber, &row.Locator, &row.SourceID, &row.SourceTitle,
			&row.AssociationID, &row.Relation, &row.ClaimID, &row.ClaimStatus, &row.ClaimNotes,
			&createdAt); err != nil {
			return TribeEvidenceResponse{}, err
		}
		if pageNumber.Valid {
			page := int(pageNumber.Int32)
			row.PageNumber = &page
		}
		if createdAt.Valid {
			row.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		response.Evidence = append(response.Evidence, row)
	}
	if err := rows.Err(); err != nil {
		return TribeEvidenceResponse{}, err
	}
	if len(response.Evidence) > MaxTribeEvidenceRows {
		response.Evidence = response.Evidence[:MaxTribeEvidenceRows]
		response.Truncated = true
	}
	return response, nil
}
