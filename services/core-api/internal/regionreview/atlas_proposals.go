package regionreview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type AtlasStage struct {
	ItemKey   string            `json:"itemKey"`
	SHA256    string            `json:"sha256"`
	Method    string            `json:"method"`
	Engine    string            `json:"engine"`
	Frame     json.RawMessage   `json:"frame"`
	Proposals []json.RawMessage `json:"proposals"`
}

type stagedReading struct {
	Original string          `json:"original"`
	Selector json.RawMessage `json:"selector"`
}

type AtlasProposal struct {
	ID      string          `json:"id"`
	Title   string          `json:"titleAr"`
	Locator string          `json:"locatorAr"`
	URL     string          `json:"url"`
	SHA256  string          `json:"sha256"`
	Method  string          `json:"method"`
	Engine  string          `json:"engine"`
	Raw     json.RawMessage `json:"raw"`
}

type AtlasProposalPage struct {
	Proposals []AtlasProposal `json:"proposals"`
	Next      string          `json:"next,omitempty"`
}

type AtlasProposalDecision struct {
	Action    string `json:"action"`
	Original  string `json:"original"`
	DisplayAR string `json:"displayAr"`
	Language  string `json:"language"`
	NoteAR    string `json:"noteAr"`
}

func jsonObject(data json.RawMessage) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal(data, &object) == nil && object != nil
}

func (s *Service) ImportAtlasStage(ctx context.Context, actorID, sourceID string, stage AtlasStage) (int64, error) {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return 0, err
	}
	if !allowed {
		return 0, ErrForbidden
	}
	source, err := uuid.Parse(sourceID)
	if err != nil {
		return 0, ErrValidation
	}
	digest, err := hex.DecodeString(stage.SHA256)
	if err != nil || len(digest) != sha256.Size || len(stage.Proposals) == 0 || len(stage.Proposals) > 500 ||
		stage.ItemKey == "" || len(stage.ItemKey) > 200 || stage.Engine == "" || len(stage.Engine) > 200 || !jsonObject(stage.Frame) {
		return 0, ErrValidation
	}
	switch stage.Method {
	case "tesseract", "pdftotext", "svg-text", "webvtt":
	default:
		return 0, ErrValidation
	}
	var frame map[string]json.RawMessage
	if err := json.Unmarshal(stage.Frame, &frame); err != nil {
		return 0, ErrValidation
	}
	canonicalFrame, err := json.Marshal(frame)
	if err != nil {
		return 0, err
	}
	keys := make([]string, len(stage.Proposals))
	for index, raw := range stage.Proposals {
		var reading stagedReading
		if json.Unmarshal(raw, &reading) != nil || strings.TrimSpace(reading.Original) == "" || !jsonObject(reading.Selector) {
			return 0, ErrValidation
		}
		var selector map[string]json.RawMessage
		if err := json.Unmarshal(reading.Selector, &selector); err != nil {
			return 0, ErrValidation
		}
		canonical, err := json.Marshal(selector)
		if err != nil {
			return 0, err
		}
		key := sha256.Sum256([]byte(string(canonicalFrame) + "\n" + string(canonical) + "\n" + reading.Original))
		keys[index] = hex.EncodeToString(key[:])
	}

	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return 0, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	access, err := policy.Source(ctx, tx, source)
	if err != nil {
		return 0, err
	}
	if !access.Allowed() {
		return 0, ErrNotFound
	}

	digestHex := hex.EncodeToString(digest)
	var revision uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO atlas_source_item_revisions AS revision
		    (item_id, revision_key, locator_ar, external_url, rights_ar, credit_ar, period_ar,
		     depicted_from_year, depicted_to_year, scan_sha256)
		SELECT i.id, 'scan:' || $3::text, base.locator_ar, base.external_url, base.rights_ar, base.credit_ar,
		       base.period_ar, base.depicted_from_year, base.depicted_to_year, $3
		FROM atlas_source_items i
		JOIN atlas_source_item_revisions base ON base.item_id = i.id AND base.revision_key NOT LIKE 'scan:%'
		WHERE i.source_id = $1 AND i.item_key = $2
		ORDER BY base.revision_key LIMIT 1
		ON CONFLICT (item_id, revision_key) DO UPDATE SET revision_key = EXCLUDED.revision_key
		RETURNING revision.id`, source, stage.ItemKey, digestHex).Scan(&revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	payload, err := json.Marshal(stage.Proposals)
	if err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO atlas_reading_proposals (item_revision_id, proposal_key, raw, method, engine, frame, imported_by)
		SELECT $1, ($2::text[])[ordinality::integer], value, $3, $4, $5, $6
		FROM jsonb_array_elements($7::jsonb) WITH ORDINALITY AS raw(value, ordinality)
		ON CONFLICT (item_revision_id, proposal_key) DO NOTHING`,
		revision, keys, stage.Method, stage.Engine, stage.Frame, reviewer, payload)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), tx.Commit(ctx)
}

func (s *Service) AtlasProposals(ctx context.Context, actorID, after string) (AtlasProposalPage, error) {
	const limit = 20
	result := AtlasProposalPage{Proposals: []AtlasProposal{}}
	_, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return result, err
	}
	if !allowed {
		return result, ErrForbidden
	}
	if after != "" {
		if _, err := uuid.Parse(after); err != nil {
			return result, ErrValidation
		}
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return result, err
	}
	params := visibility.NewParams()
	cursor := params.Add(after)
	rows, err := s.Pool.Query(ctx, `
		SELECT p.id::text, s.title_ar, r.locator_ar, r.external_url, r.scan_sha256, p.method, p.engine, p.raw
		FROM atlas_reading_proposals p
		JOIN atlas_source_item_revisions r ON r.id = p.item_revision_id
		JOIN atlas_source_items i ON i.id = r.item_id JOIN sources s ON s.id = i.source_id
		WHERE p.decision IS NULL AND (NULLIF(`+cursor+`::text, '') IS NULL OR p.id > NULLIF(`+cursor+`::text, '')::uuid)
		  AND `+policy.SourcePredicate(params, "s.id")+` ORDER BY p.id LIMIT 21`, params.Args()...)
	if err != nil {
		return result, err
	}
	result.Proposals, err = pgx.CollectRows(rows, pgx.RowToStructByPos[AtlasProposal])
	if err != nil {
		return result, err
	}
	if len(result.Proposals) > limit {
		result.Proposals = result.Proposals[:limit]
		result.Next = result.Proposals[limit-1].ID
	}
	return result, nil
}

func (s *Service) DecideAtlasProposal(ctx context.Context, actorID, proposalID string, input AtlasProposalDecision) error {
	reviewer, allowed, err := s.actorModerator(ctx, actorID)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	proposal, err := uuid.Parse(proposalID)
	if err != nil {
		return ErrValidation
	}
	input.Original = strings.TrimSpace(input.Original)
	input.DisplayAR = strings.TrimSpace(input.DisplayAR)
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	if input.NoteAR == "" || utf8.RuneCountInString(input.NoteAR) > 1000 {
		return ErrValidation
	}
	if input.Action == "publish" {
		if input.Original == "" || utf8.RuneCountInString(input.Original) > 500 ||
			input.DisplayAR == "" || utf8.RuneCountInString(input.DisplayAR) > 200 ||
			(input.Language != "ar" && input.Language != "en" && input.Language != "und") {
			return ErrValidation
		}
	} else if input.Action != "reject" {
		return ErrValidation
	}

	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var revision, source uuid.UUID
	var key, engineMethod string
	var decision *string
	var selector json.RawMessage
	err = tx.QueryRow(ctx, `
		SELECT r.id, i.source_id, p.proposal_key, p.method, p.decision, p.raw->'selector'
		FROM atlas_reading_proposals p JOIN atlas_source_item_revisions r ON r.id = p.item_revision_id
		JOIN atlas_source_items i ON i.id = r.item_id WHERE p.id = $1 FOR UPDATE OF p`, proposal).
		Scan(&revision, &source, &key, &engineMethod, &decision, &selector)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	access, err := policy.Source(ctx, tx, source)
	if err != nil {
		return err
	}
	if !access.Allowed() {
		return ErrNotFound
	}
	if decision != nil {
		return ErrAlreadyDecided
	}

	var occurrence *uuid.UUID
	if input.Action == "publish" {
		method := "original"
		switch engineMethod {
		case "tesseract":
			method = "ocr"
		case "webvtt":
			method = "transcript"
		}
		var id uuid.UUID
		err = tx.QueryRow(ctx, `
			INSERT INTO atlas_source_occurrences (item_revision_id, occurrence_key, original_text, display_ar,
			    language, reading_method, note_ar, published, source_selector)
			VALUES ($1,$2,$3,$4,$5,$6,$7,true,$8) RETURNING id`, revision, "review:"+key,
			input.Original, input.DisplayAR, input.Language, method, input.NoteAR, selector).Scan(&id)
		if err != nil {
			return err
		}
		occurrence = &id
	}
	_, err = tx.Exec(ctx, `
		UPDATE atlas_reading_proposals SET reviewed_by = $2, reviewed_at = now(), decision = $3,
		    note_ar = $4, occurrence_id = $5 WHERE id = $1`, proposal, reviewer, input.Action, input.NoteAR, occurrence)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
