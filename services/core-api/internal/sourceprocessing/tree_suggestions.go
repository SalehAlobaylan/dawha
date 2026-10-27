package sourceprocessing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// A suggestion is one machine reading of one place on a page, waiting for a
// person's decision. The service below creates them from a reading run, lists them
// as a queue, and records what a reviewer decided.
//
// The rules it holds to:
//
//   - A suggestion is never a person and never a fact. It becomes a person only
//     through the reviewed import path.
//   - The machine's reading is kept beside the reviewer's answer and never
//     instead of it, so the two can always be compared.
//   - A decision is append-only. A rejected suggestion is data: it is the
//     machine being wrong, and it stays.
//   - Nothing is accepted without a named reviewer. The confidence orders the
//     queue; it does not authorise anything.

// ErrSuggestionNotFound is returned when a suggestion does not exist.
var ErrSuggestionNotFound = errors.New("source tree suggestion was not found")

// ErrSuggestionAlreadyDecided is returned when a suggestion has already been
// decided. A second decision would rewrite history rather than add to it.
var ErrSuggestionAlreadyDecided = errors.New("source tree suggestion was already decided")

// SuggestionDecision is what a reviewer decided about a suggestion.
type SuggestionDecision struct {
	Decision string `json:"decision"`
	// TextAR is the reviewer's reading. It is required for a correction, and
	// must be empty for a rejection: a rejected suggestion carries no text.
	TextAR string `json:"textAr"`
	NoteAR string `json:"noteAr"`

	// The evidence below describes what the reviewer saw. It is optional for
	// older clients, but a new client should send it with every decision. It is
	// stored as review evidence, never as a new suggestion or a fact.
	CandidateSnapshot    []SuggestionCandidateSnapshot `json:"candidateSnapshot,omitempty"`
	SelectedCandidate    string                        `json:"selectedCandidate,omitempty"`
	InputMethod          string                        `json:"inputMethod,omitempty"`
	JudgeOutput          json.RawMessage               `json:"judgeOutput,omitempty"`
	NormalizerVersion    string                        `json:"normalizerVersion,omitempty"`
	CandidatePoolVersion string                        `json:"candidatePoolVersion,omitempty"`
	DurationMS           *int                          `json:"durationMs,omitempty"`
}

// SuggestionCandidateSnapshot is one candidate the reviewer could see. The keys
// are versioned evidence: a later normalizer change must not reinterpret an old
// decision. The value is the display spelling, not a rewritten form.
type SuggestionCandidateSnapshot struct {
	Value       string         `json:"value"`
	Key         string         `json:"key"`
	CompactKey  string         `json:"compactKey,omitempty"`
	ParticleKey string         `json:"particleKey,omitempty"`
	SkeletonKey string         `json:"skeletonKey,omitempty"`
	Sources     []string       `json:"sources"`
	Ranks       map[string]int `json:"ranks,omitempty"`
	Score       float64        `json:"score"`
	FileCount   int            `json:"fileCount,omitempty"`
	Shown       bool           `json:"shown"`
	Position    int            `json:"position"`
}

const (
	maxDecisionCandidateSnapshot = 12
	maxDecisionCandidateRunes    = 200
	maxDecisionVersionRunes      = 80
	maxDecisionJudgeBytes        = 16 * 1024
	maxDecisionDurationMS        = 3_600_000
)

type validatedDecisionEvidence struct {
	candidateSnapshot    []byte
	selectedCandidate    any
	inputMethod          string
	judgeOutput          []byte
	normalizerVersion    string
	candidatePoolVersion string
	durationMS           *int
}

// decisionJudgeOutput is the bounded schema accepted into the append-only
// ledger. Unknown fields are rejected, not stored: an API key, a source id, or
// a reviewer id must never reach this column by accident.
type decisionJudgeOutput struct {
	Mode          string             `json:"mode"`
	Provider      string             `json:"provider"`
	Model         string             `json:"model"`
	PromptVersion string             `json:"promptVersion"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence"`
	Abstained     bool               `json:"abstained"`
	Readable      *bool              `json:"readable"`
	Shown         bool               `json:"shown"`
	Used          bool               `json:"used,omitempty"`
	LatencyMS     *int               `json:"latencyMs"`
}

// The decisions a reviewer may make.
const (
	SuggestionAccepted  = "accepted"
	SuggestionCorrected = "corrected"
	SuggestionRejected  = "rejected"
	SuggestionDeferred  = "deferred"
)

// SuggestionView is one suggestion in the review queue.
type SuggestionView struct {
	ID                   string   `json:"id"`
	OccurrenceID         string   `json:"occurrenceId"`
	PageNumber           int      `json:"pageNumber"`
	Kind                 string   `json:"kind"`
	SuggestedTextAR      string   `json:"suggestedTextAr"`
	DecidedTextAR        string   `json:"decidedTextAr,omitempty"`
	Region               Region   `json:"region"`
	Confidence           float64  `json:"confidence"`
	ViewCount            int      `json:"viewCount"`
	ReadingMethod        string   `json:"readingMethod"`
	ReadingModel         string   `json:"readingModel"`
	Status               string   `json:"status"`
	DecidedBy            string   `json:"decidedBy,omitempty"`
	DecidedAt            string   `json:"decidedAt,omitempty"`
	DecisionNoteAR       string   `json:"decisionNoteAr,omitempty"`
	SourcePageEvidenceID string   `json:"sourcePageEvidenceId,omitempty"`
	Alternatives         []string `json:"alternatives,omitempty"`
}

// Region is a normalised rectangle on a page.
type Region struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// SuggestionInput is one suggestion to record.
type SuggestionInput struct {
	OccurrenceID         string
	PageNumber           int
	SuggestedTextAR      string
	Region               Region
	Confidence           float64
	ViewCount            int
	ReadingMethod        string
	ReadingModel         string
	SourcePageEvidenceID string
	Alternatives         []string
}

// CreateSuggestions records the suggestions of a reading run.
//
// It is safe to call twice for the same run: a suggestion is keyed by its
// occurrence within the file, and a second attempt updates the existing row
// rather than creating a duplicate. A reviewer who has already decided a
// suggestion is not overwritten: their decision stands, and the new reading is
// recorded beside it.
func (s *Service) CreateSuggestions(ctx context.Context, sourceID, sourceFileID uuid.UUID, runID *uuid.UUID, suggestions []SuggestionInput) error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	if len(suggestions) == 0 {
		return nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, suggestion := range suggestions {
		if suggestion.OccurrenceID == "" || suggestion.PageNumber < 1 {
			return ErrValidation
		}
		if suggestion.Confidence < 0 || suggestion.Confidence > 1 {
			return ErrValidation
		}
		region, err := json.Marshal(suggestion.Region)
		if err != nil {
			return ErrValidation
		}
		alternatives, err := cleanSuggestionAlternatives(suggestion.Alternatives)
		if err != nil {
			return err
		}
		alternativesJSON, err := json.Marshal(alternatives)
		if err != nil {
			return ErrValidation
		}
		viewCount := suggestion.ViewCount
		if viewCount < 1 {
			viewCount = 1
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO source_tree_suggestions (
				source_id, source_file_id, source_page_evidence_id, processing_run_id,
				occurrence_id, page_number, kind, suggested_text_ar, region, alternatives,
				confidence, view_count, reading_method, reading_model, status, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, 'name', $7, $8, $9::jsonb, $10, $11, $12, $13, 'suggested', now())
			ON CONFLICT (source_file_id, occurrence_id) DO UPDATE SET
				suggested_text_ar = EXCLUDED.suggested_text_ar,
				region = EXCLUDED.region,
				alternatives = EXCLUDED.alternatives,
				confidence = EXCLUDED.confidence,
				view_count = EXCLUDED.view_count,
				reading_method = EXCLUDED.reading_method,
				reading_model = EXCLUDED.reading_model,
				updated_at = now()
			WHERE source_tree_suggestions.status = 'suggested'
		`, sourceID, sourceFileID, nullableUUIDString(suggestion.SourcePageEvidenceID), runID,
			suggestion.OccurrenceID, suggestion.PageNumber, suggestion.SuggestedTextAR, region, alternativesJSON,
			suggestion.Confidence, viewCount, suggestion.ReadingMethod, suggestion.ReadingModel)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// cleanSuggestionAlternatives bounds and de-duplicates the reader's alternatives.
// They are display-only hypotheses, so an empty list is valid and a bad one is
// rejected rather than silently truncated: a truncated list would look like a
// complete answer.
func cleanSuggestionAlternatives(values []string) ([]string, error) {
	if len(values) == 0 {
		return []string{}, nil
	}
	if len(values) > 5 {
		return nil, ErrValidation
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if len([]rune(trimmed)) > 200 {
			return nil, ErrValidation
		}
		key := strings.Join(strings.Fields(trimmed), " ")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out, nil
}

// ListSuggestions returns the review queue for a file, the most certain first.
//
// The ordering is the point of the confidence: a reviewer works down the queue and
// the certain readings come first. The confidence orders the queue and nothing
// else - it never decides anything.
func (s *Service) ListSuggestions(ctx context.Context, sourceFileID string, actorID string, status string, limit, offset int) ([]SuggestionView, error) {
	if s == nil || s.Pool == nil {
		return nil, ErrDatabaseUnavailable
	}
	fileID, err := uuid.Parse(sourceFileID)
	if err != nil {
		return nil, ErrNotFound
	}
	allowed, err := s.canReview(ctx, fileID, actorID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrForbidden
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	if status == "" {
		status = "suggested"
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT sg.id::text, sg.occurrence_id, sg.page_number, sg.kind,
		       sg.suggested_text_ar, COALESCE(sg.decided_text_ar, ''),
		       sg.region, sg.alternatives, sg.confidence, sg.view_count,
		       sg.reading_method, sg.reading_model, sg.status,
		       COALESCE(sg.decided_by::text, ''),
		       COALESCE(sg.decided_at::text, ''),
		       COALESCE(sg.decision_note_ar, ''),
		       COALESCE(sg.source_page_evidence_id::text, '')
		FROM source_tree_suggestions sg
		WHERE sg.source_file_id = $1 AND sg.status = $2
		ORDER BY sg.confidence DESC, sg.created_at, sg.id
		LIMIT $3 OFFSET $4
	`, fileID, status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	views := make([]SuggestionView, 0, limit)
	for rows.Next() {
		var view SuggestionView
		var regionBytes, alternativesBytes []byte
		if err := rows.Scan(&view.ID, &view.OccurrenceID, &view.PageNumber, &view.Kind,
			&view.SuggestedTextAR, &view.DecidedTextAR, &regionBytes, &alternativesBytes, &view.Confidence, &view.ViewCount,
			&view.ReadingMethod, &view.ReadingModel, &view.Status, &view.DecidedBy, &view.DecidedAt,
			&view.DecisionNoteAR, &view.SourcePageEvidenceID); err != nil {
			return nil, err
		}
		if len(regionBytes) > 0 {
			if err := json.Unmarshal(regionBytes, &view.Region); err != nil {
				return nil, ErrValidation
			}
		}
		if len(alternativesBytes) > 0 {
			if err := json.Unmarshal(alternativesBytes, &view.Alternatives); err != nil {
				return nil, ErrValidation
			}
		}
		views = append(views, view)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return views, nil
}

// DecideSuggestion records what a reviewer decided about a suggestion.
//
// The decision is append-only: the ledger keeps every decision, including the
// ones that were later changed, and the suggestion row carries the current one.
// A suggestion that has already been decided cannot be decided again, because a
// second decision would rewrite history rather than add to it.
func (s *Service) DecideSuggestion(ctx context.Context, suggestionID string, actorID string, input SuggestionDecision) (SuggestionView, error) {
	if s == nil || s.Pool == nil {
		return SuggestionView{}, ErrDatabaseUnavailable
	}
	parsed, err := uuid.Parse(suggestionID)
	if err != nil {
		return SuggestionView{}, ErrNotFound
	}
	switch input.Decision {
	case SuggestionAccepted, SuggestionCorrected, SuggestionRejected, SuggestionDeferred:
	default:
		return SuggestionView{}, ErrValidation
	}
	text := strings.TrimSpace(input.TextAR)
	if input.Decision == SuggestionCorrected && text == "" {
		// A correction without a correction is not a correction.
		return SuggestionView{}, ErrValidation
	}
	if input.Decision == SuggestionRejected && text != "" {
		// A rejected suggestion carries no text: it was not read.
		return SuggestionView{}, ErrValidation
	}
	if len(input.NoteAR) > 2000 {
		return SuggestionView{}, ErrValidation
	}
	evidence, err := validateDecisionEvidence(input)
	if err != nil {
		return SuggestionView{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return SuggestionView{}, err
	}
	defer tx.Rollback(ctx)

	var sourceFileID uuid.UUID
	var status string
	var suggested string
	err = tx.QueryRow(ctx, `
		SELECT source_file_id::text, status, suggested_text_ar
		FROM source_tree_suggestions
		WHERE id = $1
	`, parsed).Scan(&sourceFileID, &status, &suggested)
	if errors.Is(err, pgx.ErrNoRows) {
		return SuggestionView{}, ErrSuggestionNotFound
	}
	if err != nil {
		return SuggestionView{}, err
	}
	if status != "suggested" {
		return SuggestionView{}, ErrSuggestionAlreadyDecided
	}
	allowed, err := s.canReview(ctx, sourceFileID, actorID)
	if err != nil {
		return SuggestionView{}, err
	}
	if !allowed {
		return SuggestionView{}, ErrForbidden
	}
	if input.Decision == SuggestionAccepted {
		// Accepting the machine reading is a human decision. Record that
		// reading as the decided text in the ledger as well as on the queue row.
		text = strings.TrimSpace(suggested)
		if text == "" {
			return SuggestionView{}, ErrValidation
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO source_tree_suggestion_decisions (
			suggestion_id, reviewer_id, decision, suggested_text_ar, decided_text_ar, note_ar,
			candidate_snapshot, selected_candidate, input_method, judge_output,
			normalizer_version, candidate_pool_version, duration_ms
		)
		VALUES (
			$1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''),
			$7::jsonb, NULLIF($8, ''), $9, $10::jsonb, $11, $12, $13
		)
	`, parsed, actorID, input.Decision, suggested, text, strings.TrimSpace(input.NoteAR),
		evidence.candidateSnapshot, evidence.selectedCandidate, evidence.inputMethod,
		evidence.judgeOutput, evidence.normalizerVersion, evidence.candidatePoolVersion,
		evidence.durationMS); err != nil {
		return SuggestionView{}, err
	}

	decidedText := any(nil)
	if input.Decision == SuggestionAccepted || input.Decision == SuggestionCorrected {
		decidedText = text
	}
	if _, err := tx.Exec(ctx, `
		UPDATE source_tree_suggestions
		SET status = $2,
		    decided_text_ar = $3,
		    decided_by = $4,
		    decided_at = now(),
		    decision_note_ar = NULLIF($5, ''),
		    updated_at = now()
		WHERE id = $1 AND status = 'suggested'
	`, parsed, input.Decision, decidedText, actorID, strings.TrimSpace(input.NoteAR)); err != nil {
		return SuggestionView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return SuggestionView{}, err
	}
	return s.getSuggestion(ctx, parsed)
}

// validateDecisionEvidence bounds and checks the snapshot before it is written.
// A bad snapshot is rejected rather than silently truncated: a truncated list
// would look like a complete record of what the reviewer saw.
func validateDecisionEvidence(input SuggestionDecision) (validatedDecisionEvidence, error) {
	out := validatedDecisionEvidence{
		candidateSnapshot:    []byte("[]"),
		judgeOutput:          []byte("{}"),
		inputMethod:          "legacy",
		normalizerVersion:    "legacy",
		candidatePoolVersion: "legacy",
		durationMS:           input.DurationMS,
	}

	if len(input.CandidateSnapshot) > maxDecisionCandidateSnapshot {
		return out, ErrValidation
	}
	cleaned := make([]SuggestionCandidateSnapshot, 0, len(input.CandidateSnapshot))
	values := make(map[string]struct{}, len(input.CandidateSnapshot))
	positions := make(map[int]struct{}, len(input.CandidateSnapshot))
	for _, candidate := range input.CandidateSnapshot {
		candidate.Value = strings.TrimSpace(candidate.Value)
		candidate.Key = strings.TrimSpace(candidate.Key)
		candidate.CompactKey = strings.TrimSpace(candidate.CompactKey)
		candidate.ParticleKey = strings.TrimSpace(candidate.ParticleKey)
		candidate.SkeletonKey = strings.TrimSpace(candidate.SkeletonKey)
		if candidate.Value == "" || candidate.Key == "" {
			return out, ErrValidation
		}
		for _, value := range []string{candidate.Value, candidate.Key, candidate.CompactKey, candidate.ParticleKey, candidate.SkeletonKey} {
			if len([]rune(value)) > maxDecisionCandidateRunes {
				return out, ErrValidation
			}
		}
		if _, ok := values[candidate.Value]; ok {
			return out, ErrValidation
		}
		values[candidate.Value] = struct{}{}

		if len(candidate.Sources) == 0 {
			return out, ErrValidation
		}
		seenSources := make(map[string]struct{}, len(candidate.Sources))
		for _, source := range candidate.Sources {
			if !validCandidateSource(source) {
				return out, ErrValidation
			}
			if _, ok := seenSources[source]; ok {
				return out, ErrValidation
			}
			seenSources[source] = struct{}{}
		}
		for source, rank := range candidate.Ranks {
			if !validCandidateSource(source) || rank < 1 {
				return out, ErrValidation
			}
		}
		if math.IsNaN(candidate.Score) || math.IsInf(candidate.Score, 0) || candidate.Score < 0 || candidate.Score > 100 {
			return out, ErrValidation
		}
		if candidate.FileCount < 0 {
			return out, ErrValidation
		}
		if candidate.Shown {
			if candidate.Position < 1 || candidate.Position > maxDecisionCandidateSnapshot {
				return out, ErrValidation
			}
			if _, ok := positions[candidate.Position]; ok {
				return out, ErrValidation
			}
			positions[candidate.Position] = struct{}{}
		} else if candidate.Position != 0 {
			return out, ErrValidation
		}
		cleaned = append(cleaned, candidate)
	}
	if len(cleaned) > 0 {
		encoded, err := json.Marshal(cleaned)
		if err != nil {
			return out, ErrValidation
		}
		out.candidateSnapshot = encoded
	}

	inputMethod := strings.TrimSpace(input.InputMethod)
	if inputMethod == "" {
		inputMethod = "legacy"
	}
	switch inputMethod {
	case "legacy", "reader", "candidate", "typed", "none":
	default:
		return out, ErrValidation
	}
	out.inputMethod = inputMethod

	selected := strings.TrimSpace(input.SelectedCandidate)
	if len([]rune(selected)) > maxDecisionCandidateRunes {
		return out, ErrValidation
	}
	if selected != "" {
		if _, ok := values[selected]; !ok {
			return out, ErrValidation
		}
		if inputMethod != "reader" && inputMethod != "candidate" {
			return out, ErrValidation
		}
		out.selectedCandidate = selected
	}
	switch inputMethod {
	case "reader", "candidate":
		if selected == "" || len(cleaned) == 0 {
			return out, ErrValidation
		}
	case "typed", "none":
		if selected != "" {
			return out, ErrValidation
		}
	}
	if input.Decision == SuggestionRejected || input.Decision == SuggestionDeferred {
		if selected != "" || inputMethod == "reader" || inputMethod == "candidate" {
			return out, ErrValidation
		}
	}

	out.normalizerVersion = strings.TrimSpace(input.NormalizerVersion)
	if out.normalizerVersion == "" {
		out.normalizerVersion = "legacy"
	}
	if len([]rune(out.normalizerVersion)) > maxDecisionVersionRunes {
		return out, ErrValidation
	}
	out.candidatePoolVersion = strings.TrimSpace(input.CandidatePoolVersion)
	if out.candidatePoolVersion == "" {
		out.candidatePoolVersion = "legacy"
	}
	if len([]rune(out.candidatePoolVersion)) > maxDecisionVersionRunes {
		return out, ErrValidation
	}

	if out.durationMS != nil && (*out.durationMS < 0 || *out.durationMS > maxDecisionDurationMS) {
		return out, ErrValidation
	}

	if len(input.JudgeOutput) > 0 {
		if len(input.JudgeOutput) > maxDecisionJudgeBytes || !json.Valid(input.JudgeOutput) {
			return out, ErrValidation
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(input.JudgeOutput, &raw); err != nil || raw == nil {
			return out, ErrValidation
		}
		if len(raw) > 0 {
			if err := validateJudgeOutput(input.JudgeOutput, values); err != nil {
				return out, err
			}
		}
		out.judgeOutput = input.JudgeOutput
	}

	return out, nil
}

func validateJudgeOutput(rawJSON []byte, values map[string]struct{}) error {
	decoder := json.NewDecoder(bytes.NewReader(rawJSON))
	decoder.DisallowUnknownFields()
	var judge decisionJudgeOutput
	if err := decoder.Decode(&judge); err != nil {
		return ErrValidation
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrValidation
	}
	if judge.Mode != "shadow" && judge.Mode != "active" {
		return ErrValidation
	}
	if !validNameJudgeProvider(judge.Provider) {
		return ErrValidation
	}
	if judge.Model == "" || len([]rune(judge.Model)) > 200 || strings.EqualFold(judge.Model, "latest") {
		return ErrValidation
	}
	if judge.PromptVersion == "" || len([]rune(judge.PromptVersion)) > 100 || strings.EqualFold(judge.PromptVersion, "latest") {
		return ErrValidation
	}
	choice := strings.TrimSpace(judge.Choice)
	if len([]rune(choice)) > maxDecisionCandidateRunes {
		return ErrValidation
	}
	if judge.Abstained {
		if choice != "" && choice != "unclear" {
			return ErrValidation
		}
	} else {
		if choice == "" {
			return ErrValidation
		}
		if choice != "unclear" {
			if _, ok := values[choice]; !ok {
				return ErrValidation
			}
		}
	}
	if judge.Confidence == nil || math.IsNaN(*judge.Confidence) || math.IsInf(*judge.Confidence, 0) || *judge.Confidence < 0 || *judge.Confidence > 1 {
		return ErrValidation
	}
	if judge.LatencyMS == nil || *judge.LatencyMS < 0 || *judge.LatencyMS > maxJudgeLatencyMS {
		return ErrValidation
	}
	if judge.Mode == "shadow" {
		if judge.Shown || judge.Used {
			return ErrValidation
		}
	} else {
		if !judge.Shown || (judge.Used && !judge.Shown) {
			return ErrValidation
		}
	}
	if len(judge.Probabilities) > 0 {
		if len(judge.Probabilities) > maxDecisionCandidateSnapshot+1 {
			return ErrValidation
		}
		for key, probability := range judge.Probabilities {
			if key != "unclear" && !validJudgeNameValue(key, true) {
				return ErrValidation
			}
			if math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0 || probability > 1 {
				return ErrValidation
			}
		}
		if judge.Abstained {
			if _, ok := judge.Probabilities["unclear"]; !ok {
				return ErrValidation
			}
		} else if choice != "unclear" {
			if _, ok := judge.Probabilities[choice]; !ok {
				return ErrValidation
			}
		}
	}
	return nil
}

func validCandidateSource(source string) bool {
	switch source {
	case "reader", "ai", "file", "common":
		return true
	default:
		return false
	}
}

// SuggestionReadResult is what one reading pass produced.
type SuggestionReadResult struct {
	Pages      int  `json:"pages"`
	Detected   int  `json:"detected"`
	Named      int  `json:"named"`
	Unreadable int  `json:"unreadable"`
	Read       bool `json:"read"`
	// MedianNameHeightPx is the median height of the proposed names in
	// source pixels. It is how the result says, without an answer key,
	// whether the original has enough detail to read.
	MedianNameHeightPx int  `json:"medianNameHeightPx"`
	SourceTooSmall     bool `json:"sourceTooSmall"`
}

// minimumReadableNameHeightPx is the point below which a name is too few source
// pixels tall for a reader to have a fair chance. It is a warning, not a gate:
// the regions are still reviewable, and a person may still read them.
const minimumReadableNameHeightPx = 18

// summarizeNameHeights answers the one question the run can answer without an
// answer key: were the proposed names big enough to read? The upper median is
// used so an odd split cannot hide a page of tiny names behind one large one.
func summarizeNameHeights(heights []int) (median int, tooSmall bool) {
	if len(heights) == 0 {
		return 0, false
	}
	sorted := append([]int(nil), heights...)
	sort.Ints(sorted)
	median = sorted[len(sorted)/2]
	return median, median < minimumReadableNameHeightPx
}

// ReadSuggestions runs the reading pipeline over a file and records the
// suggestions it finds.
//
// The pipeline is the one the measurements settled on: render the page at the
// sharpest resolution the source provides, find the name carriers in it, group
// the ink into names, and read each name in isolation. The benchmark arm runs
// the same detection and reading code over the corpus page images at their
// captured size; the product re-renders the page at analysis resolution first,
// so a corpus number is a floor for this path rather than a prediction of it.
//
// Without a reader configured it still records what it found, as detected
// occurrences with no transcription. That is the localiser's recall measured
// against nothing, and it is worth having.
func (s *Service) ReadSuggestions(ctx context.Context, fileID string, actorID string) (SuggestionReadResult, error) {
	if s == nil || s.Pool == nil {
		return SuggestionReadResult{}, ErrDatabaseUnavailable
	}
	parsedFileID, err := uuid.Parse(fileID)
	if err != nil {
		return SuggestionReadResult{}, ErrNotFound
	}
	allowed, err := s.canReview(ctx, parsedFileID, actorID)
	if err != nil {
		return SuggestionReadResult{}, err
	}
	if !allowed {
		return SuggestionReadResult{}, ErrForbidden
	}

	var sourceID uuid.UUID
	var storageKey, mimeType string
	err = s.Pool.QueryRow(ctx, `
		SELECT source_id, storage_key, mime_type FROM source_files WHERE id = $1
	`, parsedFileID).Scan(&sourceID, &storageKey, &mimeType)
	if errors.Is(err, pgx.ErrNoRows) {
		return SuggestionReadResult{}, ErrNotFound
	}
	if err != nil {
		return SuggestionReadResult{}, err
	}
	if !isImageContentType(mimeType) && normalizeContentType(mimeType) != "application/pdf" {
		return SuggestionReadResult{}, ErrUnsupportedDocument
	}
	if s.Store == nil {
		return SuggestionReadResult{}, ErrStorageUnavailable
	}
	reader, object, err := s.Store.Get(ctx, storageKey)
	if err != nil {
		return SuggestionReadResult{}, ErrStorageUnavailable
	}
	content, readErr := io.ReadAll(io.LimitReader(reader, MaxUploadBytes+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || len(content) > MaxUploadBytes || object.Size != int64(len(content)) {
		return SuggestionReadResult{}, ErrStorageUnavailable
	}

	extractor, ok := s.Extractor.(*PDFExtractor)
	if !ok || extractor == nil {
		extractor = NewPDFExtractor()
	}
	pageCount := extractor.PageCount(ctx, content)
	if pageCount == 0 {
		return SuggestionReadResult{}, ErrPDFToolsUnavailable
	}

	localiser := DefaultCarrierLocaliserConfig()
	readerConfig := DefaultCarrierReaderConfig()
	result := SuggestionReadResult{Read: s.ChartVision != nil}
	nameHeights := make([]int, 0, 128)

	for pageNumber := 1; pageNumber <= pageCount; pageNumber++ {
		// Read from the original page at analysis resolution, not from a
		// UI-sized preview. On a wall chart the preview is tens of DPI and a
		// name is a handful of pixels; the source has the detail and the tiles
		// keep it.
		chartPage, err := extractor.OpenChartPageForAnalysis(ctx, content, pageNumber)
		if err != nil {
			// A page that cannot be rendered is not a reason to lose the pages
			// that can be.
			continue
		}
		names, readErr := ReadNamesOnPage(ctx, chartPage, s.ChartVision, localiser, readerConfig)
		closeErr := chartPage.Close()
		if readErr != nil {
			return result, readErr
		}
		if closeErr != nil {
			return result, closeErr
		}
		if len(names) == 0 {
			continue
		}
		result.Pages++
		result.Detected += len(names)
		for _, name := range names {
			nameHeights = append(nameHeights, int(name.Region.Height*float64(chartPage.Height)+0.5))
		}

		suggestions := make([]SuggestionInput, 0, len(names))
		for index, name := range names {
			// The occurrence id carries the page: the table is unique on
			// (source_file_id, occurrence_id), and two pages both starting at
			// carrier-000000 would overwrite one another.
			suggestion := SuggestionInput{
				OccurrenceID:    fmt.Sprintf("carrier-p%03d-%06d", pageNumber, index),
				PageNumber:      pageNumber,
				SuggestedTextAR: "",
				Region:          Region{X: name.Region.X, Y: name.Region.Y, Width: name.Region.Width, Height: name.Region.Height},
				Confidence:      0,
				ViewCount:       1,
				ReadingMethod:   carrierReadingMethod,
				ReadingModel:    nameReadingModel,
				Alternatives:    name.Reading.Alternatives,
			}
			if name.Read {
				suggestion.SuggestedTextAR = name.Reading.TextAR
				suggestion.Confidence = name.Reading.Confidence
				suggestion.ViewCount = readerConfig.Views
				if name.Reading.TextAR != "" {
					result.Named++
				} else {
					result.Unreadable++
				}
			}
			suggestions = append(suggestions, suggestion)
		}
		if err := s.CreateSuggestions(ctx, sourceID, parsedFileID, nil, suggestions); err != nil {
			return result, err
		}
	}
	result.MedianNameHeightPx, result.SourceTooSmall = summarizeNameHeights(nameHeights)
	return result, nil
}

// canReview reports whether an actor may decide suggestions on a file.
func (s *Service) canReview(ctx context.Context, fileID uuid.UUID, actorID string) (bool, error) {
	var sourceID uuid.UUID
	err := s.Pool.QueryRow(ctx, `SELECT source_id FROM source_files WHERE id = $1`, fileID).Scan(&sourceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	parsedActor, err := uuid.Parse(actorID)
	if err != nil {
		return false, ErrForbidden
	}
	return canReviewSource(ctx, s.Pool, sourceID, parsedActor)
}

// getSuggestion reads one suggestion back.
func (s *Service) getSuggestion(ctx context.Context, suggestionID uuid.UUID) (SuggestionView, error) {
	var view SuggestionView
	var regionBytes, alternativesBytes []byte
	err := s.Pool.QueryRow(ctx, `
		SELECT sg.id::text, sg.occurrence_id, sg.page_number, sg.kind,
		       sg.suggested_text_ar, COALESCE(sg.decided_text_ar, ''),
		       sg.region, sg.alternatives, sg.confidence, sg.view_count,
		       sg.reading_method, sg.reading_model, sg.status,
		       COALESCE(sg.decided_by::text, ''),
		       COALESCE(sg.decided_at::text, ''),
		       COALESCE(sg.decision_note_ar, ''),
		       COALESCE(sg.source_page_evidence_id::text, '')
		FROM source_tree_suggestions sg
		WHERE sg.id = $1
	`, suggestionID).Scan(&view.ID, &view.OccurrenceID, &view.PageNumber, &view.Kind,
		&view.SuggestedTextAR, &view.DecidedTextAR, &regionBytes, &alternativesBytes, &view.Confidence, &view.ViewCount,
		&view.ReadingMethod, &view.ReadingModel, &view.Status, &view.DecidedBy, &view.DecidedAt,
		&view.DecisionNoteAR, &view.SourcePageEvidenceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SuggestionView{}, ErrSuggestionNotFound
	}
	if err != nil {
		return SuggestionView{}, err
	}
	if len(regionBytes) > 0 {
		if err := json.Unmarshal(regionBytes, &view.Region); err != nil {
			return SuggestionView{}, ErrValidation
		}
	}
	if len(alternativesBytes) > 0 {
		if err := json.Unmarshal(alternativesBytes, &view.Alternatives); err != nil {
			return SuggestionView{}, ErrValidation
		}
	}
	return view, nil
}

// SuggestionAccuracy is the accuracy the decision log implies.
//
// This is the measurement the feature produces for free: every correction a
// reviewer makes is a labelled example, so the accuracy of the reader is known
// from real use rather than from a labelling project. The count is reported
// beside the figure, because a percentage computed from nine suggestions is not
// a percentage.
type SuggestionAccuracy struct {
	Total      int     `json:"total"`
	Accepted   int     `json:"accepted"`
	Corrected  int     `json:"corrected"`
	Rejected   int     `json:"rejected"`
	Deferred   int     `json:"deferred"`
	Unchanged  float64 `json:"unchangedRate"`
	Measurable bool    `json:"measurable"`
}

// SuggestionAccuracy reports how often the reader was right without correction,
// from the decisions the reviewers made.
func (s *Service) SuggestionAccuracy(ctx context.Context, sourceFileID string, actorID string) (SuggestionAccuracy, error) {
	if s == nil || s.Pool == nil {
		return SuggestionAccuracy{}, ErrDatabaseUnavailable
	}
	fileID, err := uuid.Parse(sourceFileID)
	if err != nil {
		return SuggestionAccuracy{}, ErrNotFound
	}
	allowed, err := s.canReview(ctx, fileID, actorID)
	if err != nil {
		return SuggestionAccuracy{}, err
	}
	if !allowed {
		return SuggestionAccuracy{}, ErrForbidden
	}
	accuracy := SuggestionAccuracy{}
	err = s.Pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE sg.status <> 'suggested'),
			COUNT(*) FILTER (WHERE sg.status = 'accepted'),
			COUNT(*) FILTER (WHERE sg.status = 'corrected'),
			COUNT(*) FILTER (WHERE sg.status = 'rejected'),
			COUNT(*) FILTER (WHERE sg.status = 'deferred')
		FROM source_tree_suggestions sg
		WHERE sg.source_file_id = $1
	`, fileID).Scan(&accuracy.Total, &accuracy.Accepted, &accuracy.Corrected, &accuracy.Rejected, &accuracy.Deferred)
	if err != nil {
		return SuggestionAccuracy{}, err
	}
	decided := accuracy.Accepted + accuracy.Corrected + accuracy.Rejected
	if decided == 0 {
		return accuracy, nil
	}
	accuracy.Unchanged = float64(accuracy.Accepted) / float64(decided)
	// Below a hundred decisions the figure is an anecdote, and it says so rather
	// than presenting itself as a percentage.
	accuracy.Measurable = decided >= 100
	return accuracy, nil
}

// suggestionPageNumber is a small helper for a page number from a path or a form.
func suggestionPageNumber(value string) (int, error) {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || number < 1 {
		return 0, ErrValidation
	}
	return number, nil
}
