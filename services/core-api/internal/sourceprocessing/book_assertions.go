package sourceprocessing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const MaxBookAssertionQuoteRunes = 12000

type BookAssertionInput struct {
	PageEvidenceID         string `json:"pageEvidenceId"`
	PageReviewRevisionID   string `json:"pageReviewRevisionId"`
	ExpectedReviewRevision int    `json:"expectedReviewRevision"`
	QuoteStartCodepoint    int    `json:"quoteStartCodepoint"`
	QuoteEndCodepoint      int    `json:"quoteEndCodepoint"`
	QuoteTextAR            string `json:"quoteTextAr"`
	SubjectMentionAR       string `json:"subjectMentionAr"`
	Predicate              string `json:"predicate"`
	ObjectMentionAR        string `json:"objectMentionAr"`
	PrintedPageLabel       string `json:"printedPageLabel"`
	AttributionAR          string `json:"attributionAr"`
	Polarity               string `json:"polarity"`
	Certainty              string `json:"certainty"`
	NoteAR                 string `json:"noteAr"`
}

type ReviewBookAssertionInput struct {
	ExpectedRevision int    `json:"expectedRevision"`
	Decision         string `json:"decision"`
	NoteAR           string `json:"noteAr"`
}

type UpdateBookAssertionInput struct {
	ExpectedRevision int `json:"expectedRevision"`
	BookAssertionInput
}

type BookAssertionView struct {
	ID                   string    `json:"id"`
	SourceID             string    `json:"sourceId"`
	SourceFileID         string    `json:"sourceFileId"`
	PageEvidenceID       string    `json:"pageEvidenceId"`
	PageReviewRevisionID string    `json:"pageReviewRevisionId"`
	State                string    `json:"state"`
	Revision             int       `json:"revision"`
	SourceStatementID    string    `json:"sourceStatementId,omitempty"`
	QuoteStartCodepoint  int       `json:"quoteStartCodepoint"`
	QuoteEndCodepoint    int       `json:"quoteEndCodepoint"`
	OffsetUnit           string    `json:"offsetUnit"`
	QuoteTextAR          string    `json:"quoteTextAr"`
	SubjectMentionAR     string    `json:"subjectMentionAr"`
	Predicate            string    `json:"predicate"`
	ObjectMentionAR      string    `json:"objectMentionAr"`
	PrintedPageLabel     string    `json:"printedPageLabel,omitempty"`
	AttributionAR        string    `json:"attributionAr,omitempty"`
	Polarity             string    `json:"polarity"`
	Certainty            string    `json:"certainty"`
	NoteAR               string    `json:"noteAr,omitempty"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type bookAssertionRow struct {
	ID, SourceID, SourceFileID, PageEvidenceID, PageReviewRevisionID uuid.UUID
	State                                                            string
	Revision                                                         int
	SourceStatementID                                                pgtype.UUID
	CreatedAt, UpdatedAt                                             time.Time
}

type bookAssertionPayload struct {
	QuoteStartCodepoint int    `json:"quoteStartCodepoint"`
	QuoteEndCodepoint   int    `json:"quoteEndCodepoint"`
	OffsetUnit          string `json:"offsetUnit"`
	QuoteTextAR         string `json:"quoteTextAr"`
	SubjectMentionAR    string `json:"subjectMentionAr"`
	Predicate           string `json:"predicate"`
	ObjectMentionAR     string `json:"objectMentionAr"`
	PrintedPageLabel    string `json:"printedPageLabel,omitempty"`
	AttributionAR       string `json:"attributionAr,omitempty"`
	Polarity            string `json:"polarity"`
	Certainty           string `json:"certainty"`
	NoteAR              string `json:"noteAr,omitempty"`
}

// CreateBookAssertion stages one human-structured assertion against a precise
// span of a reviewed Arabic transcription. Names remain mentions; nothing here
// creates or merges people, claims, or tree relationships.
func (s *Service) CreateBookAssertion(ctx context.Context, fileID, actorID string, input BookAssertionInput) (BookAssertionView, error) {
	if err := s.databaseReady(); err != nil {
		return BookAssertionView{}, err
	}
	fileUUID, actorUUID, err := parseIDs(fileID, actorID)
	if err != nil {
		return BookAssertionView{}, err
	}
	input, err = validateBookAssertionInput(input)
	if err != nil {
		return BookAssertionView{}, err
	}
	pageID, err := uuid.Parse(input.PageEvidenceID)
	if err != nil {
		return BookAssertionView{}, ErrValidation
	}
	reviewID, err := uuid.Parse(input.PageReviewRevisionID)
	if err != nil {
		return BookAssertionView{}, ErrValidation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return BookAssertionView{}, err
	}
	defer tx.Rollback(ctx)
	var sourceID pgtype.UUID
	var sourceType string
	if err := tx.QueryRow(ctx, `SELECT s.id, s.source_type FROM source_files f JOIN sources s ON s.id = f.source_id WHERE f.id = $1 FOR SHARE OF f, s`, fileUUID).Scan(&sourceID, &sourceType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookAssertionView{}, ErrNotFound
		}
		return BookAssertionView{}, err
	}
	if sourceType != "book" && sourceType != "manuscript" {
		return BookAssertionView{}, ErrValidation
	}
	allowed, err := canReviewSource(ctx, tx, uuidValue(sourceID), actorUUID)
	if err != nil {
		return BookAssertionView{}, err
	}
	if !allowed {
		return BookAssertionView{}, ErrForbidden
	}
	var pageNumber int
	if err := tx.QueryRow(ctx, `SELECT page_number FROM source_page_evidence WHERE id = $1 AND source_file_id = $2 FOR SHARE`, pageID, fileUUID).Scan(&pageNumber); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookAssertionView{}, ErrNotFound
		}
		return BookAssertionView{}, err
	}
	var transcription string
	var reviewNumber int
	var reviewStatus string
	if err := tx.QueryRow(ctx, `SELECT revision_number, status, transcription_text_ar FROM source_page_review_revisions WHERE id = $1 AND source_page_evidence_id = $2 FOR SHARE`, reviewID, pageID).Scan(&reviewNumber, &reviewStatus, &transcription); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookAssertionView{}, ErrConflict
		}
		return BookAssertionView{}, err
	}
	var latestReview int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(revision_number), 0) FROM source_page_review_revisions WHERE source_page_evidence_id = $1`, pageID).Scan(&latestReview); err != nil {
		return BookAssertionView{}, err
	}
	if reviewNumber != input.ExpectedReviewRevision || reviewNumber != latestReview || reviewStatus == "unreadable" {
		return BookAssertionView{}, ErrConflict
	}
	runes := []rune(transcription)
	if input.QuoteEndCodepoint > len(runes) || string(runes[input.QuoteStartCodepoint:input.QuoteEndCodepoint]) != input.QuoteTextAR {
		return BookAssertionView{}, ErrValidation
	}
	payload := bookAssertionPayloadFromInput(input)
	assertionID := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO source_book_assertions (id, source_id, source_file_id, page_evidence_id, page_review_revision_id, created_by) VALUES ($1, $2, $3, $4, $5, $6)`, assertionID, uuidValue(sourceID), fileUUID, pageID, reviewID, actorUUID); err != nil {
		return BookAssertionView{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_book_assertion_revisions (assertion_id, revision_number, reviewer_id, payload) VALUES ($1, 1, $2, $3)`, assertionID, actorUUID, mustJSON(payload)); err != nil {
		return BookAssertionView{}, err
	}
	quoteHash := sha256.Sum256([]byte(payload.QuoteTextAR))
	if err := writeAudit(ctx, tx, &actorUUID, "source_book_assertion_created", "source_book_assertion", assertionID, nil, map[string]any{"sourceFileId": fileUUID.String(), "pageNumber": pageNumber, "quoteStartCodepoint": payload.QuoteStartCodepoint, "quoteEndCodepoint": payload.QuoteEndCodepoint, "quoteSha256": hex.EncodeToString(quoteHash[:]), "predicate": payload.Predicate}); err != nil {
		return BookAssertionView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BookAssertionView{}, err
	}
	return s.GetBookAssertion(ctx, assertionID.String(), actorUUID.String())
}

func (s *Service) GetBookAssertion(ctx context.Context, assertionID, actorID string) (BookAssertionView, error) {
	if err := s.databaseReady(); err != nil {
		return BookAssertionView{}, err
	}
	assertionUUID, actorUUID, err := parseIDs(assertionID, actorID)
	if err != nil {
		return BookAssertionView{}, err
	}
	row, err := readBookAssertion(ctx, s.Pool, assertionUUID)
	if err != nil {
		return BookAssertionView{}, err
	}
	allowed, err := canReviewSource(ctx, s.Pool, row.SourceID, actorUUID)
	if err != nil {
		return BookAssertionView{}, err
	}
	if !allowed {
		return BookAssertionView{}, ErrForbidden
	}
	var body []byte
	if err := s.Pool.QueryRow(ctx, `SELECT payload FROM source_book_assertion_revisions WHERE assertion_id = $1 AND revision_number = $2`, row.ID, row.Revision).Scan(&body); err != nil {
		return BookAssertionView{}, err
	}
	return bookAssertionView(row, body)
}

// UpdateBookAssertion adds an immutable extraction revision while the assertion
// is still a draft. It must remain anchored to the exact transcription revision
// and exact quote span; changing the human transcription requires a new draft.
func (s *Service) UpdateBookAssertion(ctx context.Context, assertionID, actorID string, input UpdateBookAssertionInput) (BookAssertionView, error) {
	if err := s.databaseReady(); err != nil {
		return BookAssertionView{}, err
	}
	assertionUUID, actorUUID, err := parseIDs(assertionID, actorID)
	if err != nil {
		return BookAssertionView{}, err
	}
	if input.ExpectedRevision < 1 {
		return BookAssertionView{}, ErrValidation
	}
	bookInput, err := validateBookAssertionInput(input.BookAssertionInput)
	if err != nil {
		return BookAssertionView{}, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return BookAssertionView{}, err
	}
	defer tx.Rollback(ctx)
	row, err := readBookAssertionForUpdate(ctx, tx, assertionUUID)
	if err != nil {
		return BookAssertionView{}, err
	}
	allowed, err := canReviewSource(ctx, tx, row.SourceID, actorUUID)
	if err != nil {
		return BookAssertionView{}, err
	}
	if !allowed {
		return BookAssertionView{}, ErrForbidden
	}
	if row.State != "draft" || row.Revision != input.ExpectedRevision || bookInput.PageEvidenceID != row.PageEvidenceID.String() || bookInput.PageReviewRevisionID != row.PageReviewRevisionID.String() {
		return BookAssertionView{}, ErrConflict
	}
	var reviewNumber int
	var reviewStatus, transcription string
	if err := tx.QueryRow(ctx, `SELECT revision_number, status, transcription_text_ar FROM source_page_review_revisions WHERE id = $1 AND source_page_evidence_id = $2 FOR SHARE`, row.PageReviewRevisionID, row.PageEvidenceID).Scan(&reviewNumber, &reviewStatus, &transcription); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookAssertionView{}, ErrConflict
		}
		return BookAssertionView{}, err
	}
	var latestReview int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(revision_number), 0) FROM source_page_review_revisions WHERE source_page_evidence_id = $1`, row.PageEvidenceID).Scan(&latestReview); err != nil {
		return BookAssertionView{}, err
	}
	if reviewNumber != bookInput.ExpectedReviewRevision || reviewNumber != latestReview || reviewStatus == "unreadable" {
		return BookAssertionView{}, ErrConflict
	}
	runes := []rune(transcription)
	if bookInput.QuoteEndCodepoint > len(runes) || string(runes[bookInput.QuoteStartCodepoint:bookInput.QuoteEndCodepoint]) != bookInput.QuoteTextAR {
		return BookAssertionView{}, ErrValidation
	}
	payload := bookAssertionPayloadFromInput(bookInput)
	nextRevision := row.Revision + 1
	if _, err := tx.Exec(ctx, `INSERT INTO source_book_assertion_revisions (assertion_id, revision_number, reviewer_id, payload) VALUES ($1, $2, $3, $4)`, row.ID, nextRevision, actorUUID, mustJSON(payload)); err != nil {
		return BookAssertionView{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_book_assertions SET current_revision = $1, updated_at = now() WHERE id = $2`, nextRevision, row.ID); err != nil {
		return BookAssertionView{}, err
	}
	quoteHash := sha256.Sum256([]byte(payload.QuoteTextAR))
	if err := writeAudit(ctx, tx, &actorUUID, "source_book_assertion_updated", "source_book_assertion", row.ID, map[string]any{"revision": row.Revision}, map[string]any{"revision": nextRevision, "quoteSha256": hex.EncodeToString(quoteHash[:]), "predicate": payload.Predicate}); err != nil {
		return BookAssertionView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BookAssertionView{}, err
	}
	return s.GetBookAssertion(ctx, row.ID.String(), actorUUID.String())
}

func (s *Service) ListBookAssertions(ctx context.Context, fileID, actorID string) ([]BookAssertionView, error) {
	if err := s.databaseReady(); err != nil {
		return nil, err
	}
	fileUUID, actorUUID, err := parseIDs(fileID, actorID)
	if err != nil {
		return nil, err
	}
	var sourceID pgtype.UUID
	if err := s.Pool.QueryRow(ctx, `SELECT source_id FROM source_files WHERE id = $1`, fileUUID).Scan(&sourceID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	allowed, err := canReviewSource(ctx, s.Pool, uuidValue(sourceID), actorUUID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	rows, err := s.Pool.Query(ctx, `SELECT id FROM source_book_assertions WHERE source_file_id = $1 ORDER BY created_at, id LIMIT 500`, fileUUID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	items := make([]BookAssertionView, 0, len(ids))
	for _, id := range ids {
		item, err := s.GetBookAssertion(ctx, id.String(), actorUUID.String())
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

// ReviewBookAssertion turns a reviewed extraction into a source statement while
// keeping the assertion unresolved against canonical people and claims.
func (s *Service) ReviewBookAssertion(ctx context.Context, assertionID, actorID string, input ReviewBookAssertionInput) (BookAssertionView, error) {
	if err := s.databaseReady(); err != nil {
		return BookAssertionView{}, err
	}
	assertionUUID, actorUUID, err := parseIDs(assertionID, actorID)
	if err != nil {
		return BookAssertionView{}, err
	}
	input.Decision = strings.TrimSpace(strings.ToLower(input.Decision))
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	if input.ExpectedRevision < 1 || (input.Decision != "reviewed" && input.Decision != "rejected") || len([]rune(input.NoteAR)) > 2000 {
		return BookAssertionView{}, ErrValidation
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return BookAssertionView{}, err
	}
	defer tx.Rollback(ctx)
	row, err := readBookAssertionForUpdate(ctx, tx, assertionUUID)
	if err != nil {
		return BookAssertionView{}, err
	}
	allowed, err := canReviewSource(ctx, tx, row.SourceID, actorUUID)
	if err != nil {
		return BookAssertionView{}, err
	}
	if !allowed {
		return BookAssertionView{}, ErrForbidden
	}
	if row.State != "draft" || row.Revision != input.ExpectedRevision {
		return BookAssertionView{}, ErrConflict
	}
	var body []byte
	if err := tx.QueryRow(ctx, `SELECT payload FROM source_book_assertion_revisions WHERE assertion_id = $1 AND revision_number = $2`, assertionUUID, row.Revision).Scan(&body); err != nil {
		return BookAssertionView{}, err
	}
	var payload bookAssertionPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return BookAssertionView{}, ErrValidation
	}
	var latestReview int
	var usedReview int
	var reviewStatus, transcription string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(revision_number), 0) FROM source_page_review_revisions WHERE source_page_evidence_id = $1`, row.PageEvidenceID).Scan(&latestReview); err != nil {
		return BookAssertionView{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT revision_number, status, transcription_text_ar FROM source_page_review_revisions WHERE id = $1 AND source_page_evidence_id = $2 FOR SHARE`, row.PageReviewRevisionID, row.PageEvidenceID).Scan(&usedReview, &reviewStatus, &transcription); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BookAssertionView{}, ErrConflict
		}
		return BookAssertionView{}, err
	}
	if usedReview != latestReview || reviewStatus == "unreadable" {
		return BookAssertionView{}, ErrConflict
	}
	runes := []rune(transcription)
	if payload.QuoteStartCodepoint < 0 || payload.QuoteEndCodepoint > len(runes) || payload.QuoteEndCodepoint <= payload.QuoteStartCodepoint || string(runes[payload.QuoteStartCodepoint:payload.QuoteEndCodepoint]) != payload.QuoteTextAR {
		return BookAssertionView{}, ErrConflict
	}
	var statementID pgtype.UUID
	if input.Decision == "reviewed" {
		var pageNumber int
		if err := tx.QueryRow(ctx, `SELECT page_number FROM source_page_evidence WHERE id = $1`, row.PageEvidenceID).Scan(&pageNumber); err != nil {
			return BookAssertionView{}, err
		}
		locator := "صفحة PDF " + fmt.Sprint(pageNumber)
		if payload.PrintedPageLabel != "" {
			locator += " · صفحة الكتاب " + payload.PrintedPageLabel
		}
		newStatementID := uuid.New()
		if _, err := tx.Exec(ctx, `INSERT INTO source_statements (id, source_id, source_file_id, source_page_evidence_id, source_page_review_revision_id, statement_text_ar, locator_ar, extraction_method, review_status, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7, 'manual', 'accepted', $8)`, newStatementID, row.SourceID, row.SourceFileID, row.PageEvidenceID, row.PageReviewRevisionID, payload.QuoteTextAR, locator, actorUUID); err != nil {
			return BookAssertionView{}, err
		}
		statementID = pgtype.UUID{Bytes: newStatementID, Valid: true}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO source_book_assertion_reviews (assertion_id, revision_number, reviewer_id, decision, note_ar) VALUES ($1, $2, $3, $4, NULLIF($5, ''))`, assertionUUID, row.Revision, actorUUID, input.Decision, input.NoteAR); err != nil {
		return BookAssertionView{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE source_book_assertions SET state = $1, source_statement_id = $2, updated_at = now() WHERE id = $3`, input.Decision, nullableUUID(uuidValue(statementID), statementID.Valid), assertionUUID); err != nil {
		return BookAssertionView{}, err
	}
	quoteHash := sha256.Sum256([]byte(payload.QuoteTextAR))
	statementIDText := ""
	if statementID.Valid {
		statementIDText = uuidValue(statementID).String()
	}
	if err := writeAudit(ctx, tx, &actorUUID, "source_book_assertion_reviewed", "source_book_assertion", assertionUUID, map[string]any{"revision": row.Revision, "state": row.State}, map[string]any{"decision": input.Decision, "statementId": statementIDText, "quoteSha256": hex.EncodeToString(quoteHash[:])}); err != nil {
		return BookAssertionView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BookAssertionView{}, err
	}
	return s.GetBookAssertion(ctx, assertionUUID.String(), actorUUID.String())
}

func validateBookAssertionInput(input BookAssertionInput) (BookAssertionInput, error) {
	input.PageEvidenceID = strings.TrimSpace(input.PageEvidenceID)
	input.PageReviewRevisionID = strings.TrimSpace(input.PageReviewRevisionID)
	input.SubjectMentionAR = strings.TrimSpace(input.SubjectMentionAR)
	input.Predicate = strings.TrimSpace(input.Predicate)
	input.ObjectMentionAR = strings.TrimSpace(input.ObjectMentionAR)
	if input.Predicate == "mention_only" {
		// The UI disables this field for name-only mentions. Clear stale values
		// when an assertion is changed from a relationship to a mention.
		input.ObjectMentionAR = ""
	}
	input.PrintedPageLabel = strings.TrimSpace(input.PrintedPageLabel)
	input.AttributionAR = strings.TrimSpace(input.AttributionAR)
	input.Polarity = strings.TrimSpace(strings.ToLower(input.Polarity))
	input.Certainty = strings.TrimSpace(strings.ToLower(input.Certainty))
	input.NoteAR = strings.TrimSpace(input.NoteAR)
	if _, err := uuid.Parse(input.PageEvidenceID); err != nil {
		return BookAssertionInput{}, ErrValidation
	}
	if _, err := uuid.Parse(input.PageReviewRevisionID); err != nil {
		return BookAssertionInput{}, ErrValidation
	}
	if input.ExpectedReviewRevision < 1 || input.QuoteStartCodepoint < 0 || input.QuoteEndCodepoint <= input.QuoteStartCodepoint || !utf8.ValidString(input.QuoteTextAR) || len([]rune(input.QuoteTextAR)) > MaxBookAssertionQuoteRunes || strings.TrimSpace(input.QuoteTextAR) == "" || input.SubjectMentionAR == "" || len([]rune(input.SubjectMentionAR)) > 200 || len([]rune(input.ObjectMentionAR)) > 200 || len([]rune(input.AttributionAR)) > 300 || len([]rune(input.PrintedPageLabel)) > 100 || len([]rune(input.NoteAR)) > 2000 {
		return BookAssertionInput{}, ErrValidation
	}
	switch input.Predicate {
	case "parent_of", "spouse_of", "sibling_of", "descendant_of", "reported_relationship", "mention_only":
	default:
		return BookAssertionInput{}, ErrValidation
	}
	if input.Predicate != "reported_relationship" && input.Predicate != "mention_only" && input.ObjectMentionAR == "" {
		return BookAssertionInput{}, ErrValidation
	}
	if input.Polarity != "affirmed" && input.Polarity != "negated" {
		return BookAssertionInput{}, ErrValidation
	}
	if input.Certainty != "asserted" && input.Certainty != "reported" && input.Certainty != "uncertain" {
		return BookAssertionInput{}, ErrValidation
	}
	return input, nil
}

func bookAssertionPayloadFromInput(input BookAssertionInput) bookAssertionPayload {
	return bookAssertionPayload{QuoteStartCodepoint: input.QuoteStartCodepoint, QuoteEndCodepoint: input.QuoteEndCodepoint, OffsetUnit: "unicode_codepoints_half_open", QuoteTextAR: input.QuoteTextAR, SubjectMentionAR: input.SubjectMentionAR, Predicate: input.Predicate, ObjectMentionAR: input.ObjectMentionAR, PrintedPageLabel: input.PrintedPageLabel, AttributionAR: input.AttributionAR, Polarity: input.Polarity, Certainty: input.Certainty, NoteAR: input.NoteAR}
}

func readBookAssertion(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id uuid.UUID) (bookAssertionRow, error) {
	var row bookAssertionRow
	err := q.QueryRow(ctx, `SELECT id, source_id, source_file_id, page_evidence_id, page_review_revision_id, state, current_revision, source_statement_id, created_at, updated_at FROM source_book_assertions WHERE id = $1`, id).Scan(&row.ID, &row.SourceID, &row.SourceFileID, &row.PageEvidenceID, &row.PageReviewRevisionID, &row.State, &row.Revision, &row.SourceStatementID, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookAssertionRow{}, ErrNotFound
	}
	return row, err
}

func readBookAssertionForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (bookAssertionRow, error) {
	var row bookAssertionRow
	err := tx.QueryRow(ctx, `SELECT id, source_id, source_file_id, page_evidence_id, page_review_revision_id, state, current_revision, source_statement_id, created_at, updated_at FROM source_book_assertions WHERE id = $1 FOR UPDATE`, id).Scan(&row.ID, &row.SourceID, &row.SourceFileID, &row.PageEvidenceID, &row.PageReviewRevisionID, &row.State, &row.Revision, &row.SourceStatementID, &row.CreatedAt, &row.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return bookAssertionRow{}, ErrNotFound
	}
	return row, err
}

func bookAssertionView(row bookAssertionRow, body []byte) (BookAssertionView, error) {
	var payload bookAssertionPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return BookAssertionView{}, ErrValidation
	}
	view := BookAssertionView{ID: row.ID.String(), SourceID: row.SourceID.String(), SourceFileID: row.SourceFileID.String(), PageEvidenceID: row.PageEvidenceID.String(), PageReviewRevisionID: row.PageReviewRevisionID.String(), State: row.State, Revision: row.Revision, QuoteStartCodepoint: payload.QuoteStartCodepoint, QuoteEndCodepoint: payload.QuoteEndCodepoint, OffsetUnit: payload.OffsetUnit, QuoteTextAR: payload.QuoteTextAR, SubjectMentionAR: payload.SubjectMentionAR, Predicate: payload.Predicate, ObjectMentionAR: payload.ObjectMentionAR, PrintedPageLabel: payload.PrintedPageLabel, AttributionAR: payload.AttributionAR, Polarity: payload.Polarity, Certainty: payload.Certainty, NoteAR: payload.NoteAR, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
	if row.SourceStatementID.Valid {
		view.SourceStatementID = uuidValue(row.SourceStatementID).String()
	}
	return view, nil
}

func nullableUUID(id uuid.UUID, valid bool) any {
	if !valid {
		return nil
	}
	return id
}
