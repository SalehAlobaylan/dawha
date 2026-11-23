// Package regionreview is the adjudication lane for tribal-label candidates
// the proposal lane writes against a georeferenced plate (plan 030-E). The
// rule the package exists to enforce is the one 0066's migration comment
// states: a plate-label candidate is a *proposal*, never a place. The machine
// found ink that looks like a text line and projected its centre through the
// plate's published transform; it cannot read the text, name the tribe, or
// trace a dira. Those are the reviewer's answers, and they land here — the
// reading into label_text_ar under a named reviewer's id, the verdict into the
// append-only decision history — through no other write path.
//
// No action in this package promotes a proposal into a published fact (no
// historical_regions row, no association): promotion belongs to a later
// review surface, which must carry needsReview and the review reasons with
// it. Until that surface exists, the lane's whole output is decisions about
// proposals, and nothing in it renders to readers.
package regionreview

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrDatabaseUnavailable = errors.New("region review database is unavailable")
	ErrUnauthenticated     = errors.New("region review lane requires a signed-in session")
	ErrForbidden           = errors.New("region review lane is moderator-readable")
	ErrNotFound            = errors.New("region candidate not found")
	ErrValidation          = errors.New("region review input is invalid")
	ErrAlreadyDecided      = errors.New("region candidate already decided")
	ErrCannotReopen        = errors.New("region candidate cannot be reopened")
	// ErrCropUnavailable is a crop that cannot be produced from the stored
	// scans: no source file row, a missing object, or bytes that are not an
	// image. It is an environment/deploy state, not a client mistake, and the
	// handler answers 503 — the fix is configuration or re-upload, not retrying
	// with different parameters.
	ErrCropUnavailable = errors.New("candidate crop is unavailable")
)

// Actions are the reviewer's explicit choices, the vocabulary the 0066
// decision-history table pins. The status a decision produces is the
// materialisation of the choice, not a separate decision.
const (
	ActionAccept  = "accept"
	ActionCorrect = "correct"
	ActionReject  = "reject"
	ActionDefer   = "defer"
	ActionReopen  = "reopen"
)

const (
	ListLimit = 100
	// MaxLabelTextRunes bounds one reviewer reading. The longest label these
	// lanes propose is a few Arabic words; the bound exists so a broken
	// client cannot turn label_text_ar into a page store.
	MaxLabelTextRunes = 200
	// MaxDecisionNoteRunes mirrors the pairing lane's note bound.
	MaxDecisionNoteRunes = 1000
	// MaxEraLabelRunes bounds the reviewer's era annotation, e.g. «قبل ١٩١٦».
	MaxEraLabelRunes = 60
)

func validStatus(status string) bool {
	switch status {
	case "suggested", "accepted", "corrected", "rejected", "deferred":
		return true
	}
	return false
}

// nextStatus is the transition table: from an action and the candidate's
// current status to the next status. accept/correct/reject/defer answer a
// proposal; correct also revises a decided answer in place; reopen retracts
// a decision and returns the candidate to the proposal queue.
//
// reopen is legal from EVERY decided status, including accepted and
// corrected. The pairing lane's rule (accepted is final) exists because a
// pairing's accept writes published rows the moment it lands; this lane's
// accept writes NO published row — promotion belongs to a later surface. In
// a two-pass workbench, a lane where a reading can never lose is a lane
// where reject is not first-class, and stipulation 1 makes it exactly that:
// a rejection must be able to win a disagreement. When the promotion surface
// arrives, it will own the freeze of its own promoted rows.
var nextStatus = map[string]string{
	ActionAccept + ":suggested":  "accepted",
	ActionCorrect + ":suggested": "corrected",
	ActionCorrect + ":accepted":  "corrected",
	// A further correction of a corrected answer is legitimate: the tile
	// lane's disagreement flow sends a third reviewer (or the first, rework-
	// ing) back to a reading another correct already settled.
	ActionCorrect + ":corrected": "corrected",
	ActionReject + ":suggested":  "rejected",
	// An agreeing second pass is ONE action, not a reopen-then-repeat: the
	// second reviewer's rejection of an already-rejected crop (and their
	// shared deferral) appends their own decision row and re-states the
	// status, so two-person retirement reads two names from the history
	// without either reviewer erasing the other's first.
	ActionReject + ":rejected":  "rejected",
	ActionDefer + ":suggested":  "deferred",
	ActionDefer + ":deferred":   "deferred",
	ActionReopen + ":accepted":  "suggested",
	ActionReopen + ":corrected": "suggested",
	ActionReopen + ":rejected":  "suggested",
	ActionReopen + ":deferred":  "suggested",
}

// CropBox is the box a proposal came from, in the plate's own full-resolution
// pixel coordinates. The detector wrote it; only the crop renderer clamps it.
type CropBox struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Coordinates is the label box centre projected through the plate's published
// transform. It locates the *proposal*: a printed label is typeset beside its
// feature, so this point can sit up to half a label-width from the feature it
// names (the 030-E artifact records this limit; it is not corrected here).
type Coordinates struct {
	Longitude float64 `json:"longitude"`
	Latitude  float64 `json:"latitude"`
}

type Candidate struct {
	ID          string `json:"id"`
	SourceID    string `json:"sourceId"`
	SourceTitle string `json:"sourceTitle,omitempty"`
	// FileSHA256 is the plate file's own checksum from the source record, so
	// a crop can be tied to the exact scan the lane georeferenced.
	FileSHA256 string `json:"fileSha256,omitempty"`
	PlateKey   string `json:"plateKey"`
	// OccurrenceID is the detector's stable id for this ink occurrence, unique
	// per source, derived from the box itself.
	OccurrenceID string  `json:"occurrenceId"`
	CropPx       CropBox `json:"cropPx"`
	// Coordinates is the projected centre; nil for a plate without a
	// transform (such proposals cannot exist through the first lane, but the
	// schema allows them and an absent answer must not impersonate 0,0).
	Coordinates *Coordinates `json:"coordinates,omitempty"`
	// HasRegionGeometry reports a traced ring's existence without rendering
	// it; RegionGeometryWKT ships the shape itself for the workbench overlay.
	// Every row the first lane wrote is label-only (NULL geometry).
	HasRegionGeometry bool   `json:"hasRegionGeometry"`
	RegionGeometryWKT string `json:"regionGeometryWkt,omitempty"`
	// The reviewer's reading. Empty until a human writes it through Decide;
	// this package is its only writer.
	LabelTextAR string `json:"labelTextAr"`
	LabelReadBy string `json:"labelReadBy,omitempty"`
	LabelReadAt string `json:"labelReadAt,omitempty"`
	EraLabel    string `json:"eraLabel"`
	// Score components, kept apart on purpose (migration 0066): a single
	// confidence number would hide which measurement drove the ranking, and
	// label_confidence answers "does this box look like printed text", not
	// "is this a tribe". Each component is optional in the schema and stays a
	// pointer here: an absent measurement must not impersonate a zero.
	InkDensity      *float64 `json:"inkDensity,omitempty"`
	ComponentCount  *int     `json:"componentCount,omitempty"`
	LabelConfidence *float64 `json:"labelConfidence,omitempty"`
	// TransformRMSKm is the floor below any position derived through the
	// plate's transform — the 030-D held-out RMS, not this candidate's error.
	TransformRMSKm *float64 `json:"transformRmsKm,omitempty"`
	Rank           *int     `json:"rank,omitempty"`
	Status         string   `json:"status"`
	DecidedBy      string   `json:"decidedBy,omitempty"`
	DecidedAt      string   `json:"decidedAt,omitempty"`
	DecisionNoteAR string   `json:"decisionNoteAr,omitempty"`
	CreatedAt      string   `json:"createdAt"`
}

// PlateSummary is the honesty block a reviewer decides beside: the plate's
// georeference posture, read from sources.metadata.georef. It travels with
// the detail and crop responses so the certainty grammar is on screen while a
// reviewer works — approximate, needsReview, and why — not one click away.
type PlateSummary struct {
	SourceID      string   `json:"sourceId"`
	PlateKey      string   `json:"plateKey"`
	TitleAR       string   `json:"titleAr,omitempty"`
	License       string   `json:"license,omitempty"`
	GeorefMethod  string   `json:"georefMethod,omitempty"`
	Certainty     string   `json:"certainty,omitempty"`
	NeedsReview   bool     `json:"needsReview"`
	ReviewReasons []string `json:"reviewReasons"`
}

type ListParams struct {
	Status   string
	PlateKey string
	Query    string
}

type ListResponse struct {
	Candidates []Candidate `json:"candidates"`
	Status     string      `json:"status"`
	Limit      int         `json:"limit"`
	Truncated  bool        `json:"truncated"`
}

type CandidateResponse struct {
	Candidate Candidate     `json:"candidate"`
	Plate     *PlateSummary `json:"plate,omitempty"`
}

type DecideInput struct {
	Action      string
	NoteAR      string
	LabelTextAR *string
	EraLabel    *string
}

type DecisionHistoryEvent struct {
	ID           string `json:"id"`
	CandidateID  string `json:"candidateId"`
	ReviewerID   string `json:"reviewerId"`
	ReviewerName string `json:"reviewerName,omitempty"`
	Action       string `json:"action"`
	FromStatus   string `json:"fromStatus"`
	ToStatus     string `json:"toStatus"`
	NoteAR       string `json:"noteAr,omitempty"`
	// Diff carries what a correction changed beside the proposal — the
	// previous reading against the new one — so the original machine-era
	// answer is never overwritten silently. JSON-encoded object.
	Diff      string `json:"diff,omitempty"`
	CreatedAt string `json:"createdAt"`
}

type HistoryResponse struct {
	CandidateID string                 `json:"candidateId"`
	History     []DecisionHistoryEvent `json:"history"`
	Limit       int                    `json:"limit"`
	Truncated   bool                   `json:"truncated"`
}

type DecideResponse struct {
	Candidate Candidate `json:"candidate"`
	Action    string    `json:"action"`
}

type Service struct {
	Pool *pgxpool.Pool
	// plates is the object store the plate scans live in. It is nil only in
	// states where a crop must be refused rather than faked (tests, or a
	// deployment that did not configure storage).
	plates storage.Store
	cache  plateCache
}

func NewService(pool *pgxpool.Pool, plates storage.Store) *Service {
	return &Service{Pool: pool, plates: plates}
}

func (s *Service) actorModerator(ctx context.Context, actorID string) (uuid.UUID, bool, error) {
	if s.Pool == nil {
		return uuid.Nil, false, ErrDatabaseUnavailable
	}
	actorUUID, err := uuid.Parse(actorID)
	if err != nil || actorUUID == uuid.Nil {
		return uuid.Nil, false, ErrUnauthenticated
	}
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, actorUUID).Scan(&exists); err != nil {
		return uuid.Nil, false, err
	}
	if !exists {
		return uuid.Nil, false, ErrUnauthenticated
	}
	// The review-role gate is the workspace's moderator set, the same two
	// roles the pairing and data-hygiene lanes answer to; there is no third
	// "reviewer" role to invent.
	var allowed bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_roles WHERE user_id = $1 AND role IN ('moderator', 'admin'))`, actorUUID).Scan(&allowed); err != nil {
		return uuid.Nil, false, err
	}
	return actorUUID, allowed, nil
}

const candidateColumns = `
	c.id, c.source_id, COALESCE(src.title_ar, ''),
	COALESCE(src.metadata->>'file_sha256', ''), c.plate_key, c.occurrence_id, c.crop_px,
	ST_X(c.coordinates), ST_Y(c.coordinates), c.region_geometry IS NOT NULL,
	COALESCE(ST_AsText(c.region_geometry), ''),
	c.label_text_ar, COALESCE(c.label_read_by::text, ''), c.label_read_at,
	COALESCE(c.era_label, ''), c.ink_density, c.component_count, c.label_confidence,
	c.transform_rms_km, c.rank, c.status,
	COALESCE(c.decided_by::text, ''), c.decided_at,
	COALESCE(c.decision_note_ar, ''), c.created_at`

// scanCandidate reads one row of candidateColumns. Fields the schema may
// legitimately leave empty land in pointers, never in zeros that would
// impersonate a measurement.
func scanCandidate(scan func(dest ...any) error) (Candidate, error) {
	var candidate Candidate
	var cropBytes []byte
	var longitude, latitude, inkDensity, labelConfidence, transformRMS pgtype.Float8
	var componentCount, rank pgtype.Int4
	var labelReadAt, decidedAt, createdAt pgtype.Timestamptz
	if err := scan(&candidate.ID, &candidate.SourceID, &candidate.SourceTitle,
		&candidate.FileSHA256, &candidate.PlateKey, &candidate.OccurrenceID, &cropBytes,
		&longitude, &latitude, &candidate.HasRegionGeometry, &candidate.RegionGeometryWKT,
		&candidate.LabelTextAR, &candidate.LabelReadBy, &labelReadAt,
		&candidate.EraLabel, &inkDensity, &componentCount, &labelConfidence,
		&transformRMS, &rank, &candidate.Status,
		&candidate.DecidedBy, &decidedAt,
		&candidate.DecisionNoteAR, &createdAt); err != nil {
		return Candidate{}, err
	}
	parsed, err := parseCropBox(cropBytes)
	if err != nil {
		return Candidate{}, ErrValidation
	}
	candidate.CropPx = parsed
	if longitude.Valid && latitude.Valid {
		candidate.Coordinates = &Coordinates{Longitude: longitude.Float64, Latitude: latitude.Float64}
	}
	if inkDensity.Valid {
		v := inkDensity.Float64
		candidate.InkDensity = &v
	}
	if componentCount.Valid {
		v := int(componentCount.Int32)
		candidate.ComponentCount = &v
	}
	if labelConfidence.Valid {
		v := labelConfidence.Float64
		candidate.LabelConfidence = &v
	}
	if transformRMS.Valid {
		v := transformRMS.Float64
		candidate.TransformRMSKm = &v
	}
	if rank.Valid {
		v := int(rank.Int32)
		candidate.Rank = &v
	}
	if labelReadAt.Valid {
		candidate.LabelReadAt = labelReadAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	if decidedAt.Valid {
		candidate.DecidedAt = decidedAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	if createdAt.Valid {
		candidate.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
	}
	return candidate, nil
}

// List returns one status lane of proposals, bounded. Moderator-only: the lane
// is the same adjudication authority the pairing lane is, and its rows are
// unread proposals — exactly what must not reach a reader.
func (s *Service) List(ctx context.Context, actorID string, params ListParams) (ListResponse, error) {
	var err error
	var allowed bool
	_, allowed, err = s.actorModerator(ctx, actorID)
	if err != nil {
		return ListResponse{}, err
	}
	if !allowed {
		return ListResponse{}, ErrForbidden
	}
	if params.Status == "" {
		params.Status = "suggested"
	}
	if !validStatus(params.Status) {
		return ListResponse{}, ErrValidation
	}
	queryText := strings.TrimSpace(params.Query)
	plateKey := strings.TrimSpace(params.PlateKey)

	rows, err := s.Pool.Query(ctx, `
		SELECT `+candidateColumns+`
		FROM external_region_candidates c
		LEFT JOIN sources src ON src.id = c.source_id
		WHERE c.status = $1
		  AND ($2::text = '' OR c.plate_key = $2)
		  AND ($3::text = '' OR c.occurrence_id ILIKE '%' || $3 || '%'
		    OR c.label_text_ar ILIKE '%' || $3 || '%'
		    OR c.plate_key ILIKE '%' || $3 || '%'
		    OR src.title_ar ILIKE '%' || $3 || '%')
		ORDER BY c.rank NULLS LAST, c.created_at, c.id
		LIMIT $4`, params.Status, plateKey, queryText, ListLimit+1)
	if err != nil {
		return ListResponse{}, err
	}
	defer rows.Close()

	response := ListResponse{Candidates: []Candidate{}, Status: params.Status, Limit: ListLimit}
	for rows.Next() {
		candidate, err := scanCandidate(rows.Scan)
		if err != nil {
			return ListResponse{}, err
		}
		response.Candidates = append(response.Candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return ListResponse{}, err
	}
	if len(response.Candidates) > ListLimit {
		response.Candidates = response.Candidates[:ListLimit]
		response.Truncated = true
	}
	return response, nil
}

// Get returns one proposal with its plate's honesty block. Moderator-only.
func (s *Service) Get(ctx context.Context, actorID, candidateID string) (CandidateResponse, error) {
	var allowed bool
	var err error
	_, allowed, err = s.actorModerator(ctx, actorID)
	if err != nil {
		return CandidateResponse{}, err
	}
	if !allowed {
		return CandidateResponse{}, ErrForbidden
	}
	candidateUUID, err := uuid.Parse(candidateID)
	if err != nil {
		return CandidateResponse{}, ErrValidation
	}
	candidate, err := s.getCandidate(ctx, candidateUUID)
	if err != nil {
		return CandidateResponse{}, err
	}
	plate, err := s.getPlateSummary(ctx, candidate.SourceID)
	if err != nil {
		return CandidateResponse{}, err
	}
	plate.PlateKey = candidate.PlateKey
	return CandidateResponse{Candidate: candidate, Plate: &plate}, nil
}

func (s *Service) getCandidate(ctx context.Context, id uuid.UUID) (Candidate, error) {
	row := s.Pool.QueryRow(ctx, `
		SELECT `+candidateColumns+`
		FROM external_region_candidates c
		LEFT JOIN sources src ON src.id = c.source_id
		WHERE c.id = $1`, id)
	candidate, err := scanCandidate(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Candidate{}, ErrNotFound
		}
		return Candidate{}, err
	}
	return candidate, nil
}

func (s *Service) getPlateSummary(ctx context.Context, sourceID string) (PlateSummary, error) {
	var metadataBytes []byte
	var sourceUUID, titleAR, license string
	err := s.Pool.QueryRow(ctx, `
		SELECT id::text, COALESCE(title_ar, ''), COALESCE(metadata->>'license', ''),
		       COALESCE(metadata, '{}'::jsonb)
		FROM sources WHERE id = $1::uuid`, sourceID).Scan(
		&sourceUUID, &titleAR, &license, &metadataBytes)
	if err != nil {
		return PlateSummary{}, err
	}
	return plateSummaryFromMetadata(sourceUUID, "", titleAR, license, metadataBytes), nil
}

// plateSummaryFromMetadata reads a source row's honesty block (the georef
// posture 030-D published) into a PlateSummary. A source record without a
// georef block — every source that is not a plate — yields an empty block,
// not an invented one.
func plateSummaryFromMetadata(sourceID, plateKey, titleAR, license string, metadata []byte) PlateSummary {
	summary := PlateSummary{
		SourceID:      sourceID,
		PlateKey:      plateKey,
		TitleAR:       titleAR,
		License:       license,
		ReviewReasons: []string{},
	}
	var parsed struct {
		Georef *struct {
			Method        string   `json:"method"`
			Certainty     string   `json:"certainty"`
			NeedsReview   bool     `json:"needsReview"`
			ReviewReasons []string `json:"reviewReasons"`
		} `json:"georef"`
	}
	if err := json.Unmarshal(metadata, &parsed); err == nil && parsed.Georef != nil {
		summary.GeorefMethod = parsed.Georef.Method
		summary.Certainty = parsed.Georef.Certainty
		summary.NeedsReview = parsed.Georef.NeedsReview
		if len(parsed.Georef.ReviewReasons) > 0 {
			summary.ReviewReasons = parsed.Georef.ReviewReasons
		}
	}
	return summary
}

func validateRuneBound(value string, limit int) bool {
	return len([]rune(value)) <= limit
}

// Decide applies one explicit action to a candidate. The reading rides the
// decision: accept and correct carry the reviewer's reading of the crop into
// label_text_ar under their own id, and a correction records what changed in
// the history diff so a previous answer is never overwritten silently.
// Reject and defer write nothing but the decision (a reject may still record
// what the crop actually reads — «شعار اللوحة» — under the reviewer's id).
//
// The reading is the contract's honesty gate: an accept without one would
// later promote into a named region nobody read. A reviewer who cannot read
// the crop defers it — that is what defer is for.
func (s *Service) Decide(ctx context.Context, actorID, candidateID string, input DecideInput) (DecideResponse, error) {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return DecideResponse{}, err
	}
	if !allowed {
		return DecideResponse{}, ErrForbidden
	}
	candidateUUID, err := uuid.Parse(candidateID)
	if err != nil {
		return DecideResponse{}, ErrValidation
	}
	// The action vocabulary is the decision-history table's own CHECK.
	switch input.Action {
	case ActionAccept, ActionCorrect, ActionReject, ActionDefer, ActionReopen:
	default:
		return DecideResponse{}, ErrValidation
	}
	note := strings.TrimSpace(input.NoteAR)
	if !validateRuneBound(note, MaxDecisionNoteRunes) {
		return DecideResponse{}, ErrValidation
	}
	reading := ""
	if input.LabelTextAR != nil {
		reading = strings.TrimSpace(*input.LabelTextAR)
	}
	if !validateRuneBound(reading, MaxLabelTextRunes) {
		return DecideResponse{}, ErrValidation
	}
	era := ""
	if input.EraLabel != nil {
		era = strings.TrimSpace(*input.EraLabel)
	}
	if !validateRuneBound(era, MaxEraLabelRunes) {
		return DecideResponse{}, ErrValidation
	}

	var status, existingReading, existingEra string
	if err := s.Pool.QueryRow(ctx, `
		SELECT status, label_text_ar, era_label
		FROM external_region_candidates WHERE id = $1::uuid`, candidateUUID.String()).Scan(&status, &existingReading, &existingEra); err != nil {
		return DecideResponse{}, ErrNotFound
	}
	toStatus, ok := nextStatus[input.Action+":"+status]
	if !ok {
		return DecideResponse{}, ErrAlreadyDecided
	}

	// Which decisions carry a reading, and which refuse one: accept and
	// correct write the reading (falling back to a reading already on the
	// row — a reviewer may confirm a colleague's reading); reject may record
	// one; defer and reopen carry none — a reading supplied there is a client
	// error rather than words to drop silently, and the row's existing
	// reading is left exactly as the human who wrote it left it.
	finalReading := strings.TrimSpace(existingReading)
	switch input.Action {
	case ActionAccept, ActionCorrect:
		if reading != "" {
			finalReading = reading
		}
		if finalReading == "" {
			return DecideResponse{}, ErrValidation
		}
	case ActionReject:
		if reading != "" {
			finalReading = reading
		}
	}
	if input.LabelTextAR != nil && strings.TrimSpace(*input.LabelTextAR) != "" &&
		!(input.Action == ActionAccept || input.Action == ActionCorrect || input.Action == ActionReject) {
		return DecideResponse{}, ErrValidation
	}
	finalEra := strings.TrimSpace(existingEra)
	switch input.Action {
	case ActionAccept, ActionCorrect:
		if era != "" {
			finalEra = era
		}
	}
	if input.EraLabel != nil && strings.TrimSpace(*input.EraLabel) != "" &&
		!(input.Action == ActionAccept || input.Action == ActionCorrect) {
		return DecideResponse{}, ErrValidation
	}
	readingChanged := finalReading != existingReading
	eraChanged := finalEra != existingEra
	diff := map[string]any{}
	// A decision that stands behind a reading records that reading — the
	// reading the verdict was made on, from what the row carried to what it
	// now carries — even when unchanged. Without it a confirming second pass
	// (correct with the same reading) would leave no trace of WHICH reading
	// was confirmed, and the tile lane's verdict derivation would read an
	// empty answer.
	switch input.Action {
	case ActionAccept, ActionCorrect:
		diff["label_text_ar"] = map[string]string{"from": existingReading, "to": finalReading}
		diff["era_label"] = map[string]string{"from": existingEra, "to": finalEra}
	case ActionReject:
		if finalReading != "" {
			diff["label_text_ar"] = map[string]string{"from": existingReading, "to": finalReading}
		}
	}
	diffBytes, err := json.Marshal(diff)
	if err != nil {
		return DecideResponse{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return DecideResponse{}, err
	}
	defer tx.Rollback(ctx)

	// The guard clause (WHERE status = from-status) makes the transition
	// atomic: two moderators deciding at once produce one decision and one
	// conflict, never two rows of history. A reopen also clears the decision
	// attribution: the candidate is suggested again, undecided — the reading
	// stays (a named human wrote it, and the history preserves it), while the
	// verdict columns stop pretending a decision stands.
	tag, err := tx.Exec(ctx, `UPDATE external_region_candidates SET
		status = $2,
		decided_by = CASE WHEN $5::text = $6 THEN NULL ELSE $3::uuid END,
		decided_at = CASE WHEN $5::text = $6 THEN NULL ELSE now() END,
		decision_note_ar = $4, updated_at = now()
		WHERE id = $1 AND status = $7`,
		candidateUUID, toStatus, reviewer, note, input.Action, ActionReopen, status)
	if err != nil {
		return DecideResponse{}, err
	}
	if tag.RowsAffected() != 1 {
		return DecideResponse{}, ErrAlreadyDecided
	}
	if readingChanged {
		tag, err = tx.Exec(ctx, `UPDATE external_region_candidates SET
			label_text_ar = $2, label_read_by = $3::uuid, label_read_at = now()
			WHERE id = $1`, candidateUUID, finalReading, reviewer)
		if err != nil {
			return DecideResponse{}, err
		}
		if tag.RowsAffected() != 1 {
			return DecideResponse{}, ErrAlreadyDecided
		}
	}
	if eraChanged {
		tag, err = tx.Exec(ctx, `UPDATE external_region_candidates SET
			era_label = $2 WHERE id = $1`, candidateUUID, era)
		if err != nil {
			return DecideResponse{}, err
		}
		if tag.RowsAffected() != 1 {
			return DecideResponse{}, ErrAlreadyDecided
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO external_region_candidate_decisions
		(candidate_id, reviewer_id, action, from_status, to_status, note_ar, diff)
		VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb)`,
		candidateUUID, reviewer, input.Action, status, toStatus, note, string(diffBytes)); err != nil {
		return DecideResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DecideResponse{}, err
	}

	reloaded, err := s.getCandidate(ctx, candidateUUID)
	if err != nil {
		return DecideResponse{}, err
	}
	return DecideResponse{Candidate: reloaded, Action: input.Action}, nil
}

// History returns the append-only audit trail for one candidate. Moderator-only:
// the lane is the same authority that made the decisions.
func (s *Service) History(ctx context.Context, actorID, candidateID string) (HistoryResponse, error) {
	var err error
	var allowed bool
	_, allowed, err = s.actorModerator(ctx, actorID)
	if err != nil {
		return HistoryResponse{}, err
	}
	if !allowed {
		return HistoryResponse{}, ErrForbidden
	}
	candidateUUID, err := uuid.Parse(candidateID)
	if err != nil {
		return HistoryResponse{}, ErrValidation
	}
	var exists bool
	if err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM external_region_candidates WHERE id = $1::uuid)`, candidateUUID.String()).Scan(&exists); err != nil {
		return HistoryResponse{}, err
	}
	if !exists {
		return HistoryResponse{}, ErrNotFound
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT h.id, h.candidate_id, h.reviewer_id, COALESCE(u.display_name_ar, ''), h.action,
		       h.from_status, h.to_status, COALESCE(h.note_ar, ''), COALESCE(h.diff::text, ''), h.created_at
		FROM external_region_candidate_decisions h
		LEFT JOIN users u ON u.id = h.reviewer_id
		WHERE h.candidate_id = $1::uuid
		ORDER BY h.sequence_number
		LIMIT $2`, candidateUUID.String(), ListLimit+1)
	if err != nil {
		return HistoryResponse{}, err
	}
	defer rows.Close()
	response := HistoryResponse{CandidateID: candidateUUID.String(), History: []DecisionHistoryEvent{}, Limit: ListLimit}
	for rows.Next() {
		event := DecisionHistoryEvent{}
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&event.ID, &event.CandidateID, &event.ReviewerID, &event.ReviewerName, &event.Action,
			&event.FromStatus, &event.ToStatus, &event.NoteAR, &event.Diff, &createdAt); err != nil {
			return HistoryResponse{}, err
		}
		if createdAt.Valid {
			event.CreatedAt = createdAt.Time.UTC().Format("2006-01-02T15:04:05Z")
		}
		response.History = append(response.History, event)
	}
	if err := rows.Err(); err != nil {
		return HistoryResponse{}, err
	}
	if len(response.History) > ListLimit {
		response.History = response.History[:ListLimit]
		response.Truncated = true
	}
	return response, nil
}
