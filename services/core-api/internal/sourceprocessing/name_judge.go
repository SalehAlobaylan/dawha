package sourceprocessing

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// A judge is not a reader. It sees a bounded candidate set that the review
// bench already built and may choose one option or abstain. It never writes a
// name, person, identity, or relationship, and the core rejects any response
// outside the supplied options.
var ErrNameJudgeUnavailable = errors.New("name judge provider is unavailable")
var ErrNameJudgeSetup = errors.New("name judge provider needs setup")
var ErrNameJudgeRejected = errors.New("name judge request was rejected")
var ErrNameJudgeOutputInvalid = errors.New("name judge output failed local validation")

const (
	MaxNameJudgeCandidates = 12
	maxJudgeNameRunes      = 200
	maxJudgeImageBytes     = 4 * 1024 * 1024
	maxJudgeResponseBytes  = 2 * 1024 * 1024
	maxJudgeLatencyMS      = 120_000
)

// NameJudgeSettings are the feature flags that decide whether a judge is
// configured and whether the experimental tight crop may leave the platform.
type NameJudgeSettings struct {
	Mode         string
	Sample       float64
	ImageEnabled bool
}

func DefaultNameJudgeSettings() NameJudgeSettings {
	return NameJudgeSettings{Mode: "off", Sample: 0.2}
}

// NameJudgeSettingsFromEnvironment is deliberately fail-closed: a missing or
// misspelled mode becomes off, a malformed sample becomes the documented
// default, and image mode is true only when it is explicitly true.
func NameJudgeSettingsFromEnvironment(getenv func(string) string) NameJudgeSettings {
	settings := DefaultNameJudgeSettings()
	switch strings.ToLower(strings.TrimSpace(getenv("NAME_JUDGE_MODE"))) {
	case "shadow":
		settings.Mode = "shadow"
	case "active":
		settings.Mode = "active"
	default:
		settings.Mode = "off"
	}
	if raw := strings.TrimSpace(getenv("NAME_JUDGE_SAMPLE")); raw != "" {
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
			settings.Sample = math.Max(0, math.Min(1, parsed))
		}
	}
	switch strings.ToLower(strings.TrimSpace(getenv("NAME_JUDGE_IMAGES"))) {
	case "1", "true", "yes", "on":
		settings.ImageEnabled = true
	}
	return settings
}

type NameJudgeConfig struct {
	Mode         string  `json:"mode"`
	ImageEnabled bool    `json:"imageEnabled"`
	Available    bool    `json:"available"`
	Sample       float64 `json:"sample"`
}

// JudgeCandidate is one option the reviewer could see. Source and rank are
// evidence for the judge, not facts about a person.
type JudgeCandidate struct {
	Value  string `json:"value"`
	Source string `json:"source"`
	Rank   int    `json:"rank"`
}

type JudgeCrop struct {
	WidthPx        int `json:"width_px"`
	HeightPx       int `json:"height_px"`
	SourceWidthPx  int `json:"source_width_px"`
	SourceHeightPx int `json:"source_height_px"`
	NameHeightPx   int `json:"name_height_px"`
}

type JudgeState struct {
	Reading      string    `json:"reading"`
	Alternatives []string  `json:"alternatives"`
	FileContext  []string  `json:"file_context"`
	Crop         JudgeCrop `json:"crop"`
}

// NameJudgeRequest is the internal request to a judge adapter. ImagePNG is
// never serialized into an error or log by this package.
type NameJudgeRequest struct {
	State            JudgeState
	Candidates       []JudgeCandidate
	Mode             string
	ImagePNG         []byte
	ExpectedIdentity *FrozenJudgeIdentity
}

// NameJudgeResult is the mapped response the core API returns to the browser.
// Choice is null when the judge abstained.
type NameJudgeResult struct {
	Choice        *string            `json:"choice"`
	Abstained     bool               `json:"abstained"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
	Readable      *bool              `json:"readable"`
	Provider      string             `json:"provider"`
	Model         string             `json:"model"`
	PromptVersion string             `json:"promptVersion"`
	CacheHit      bool               `json:"cacheHit"`
}

type NameJudge interface {
	Judge(context.Context, NameJudgeRequest) (NameJudgeResult, error)
}

type nameJudgeAIRequest struct {
	State            JudgeState           `json:"state"`
	Candidates       []JudgeCandidate     `json:"candidates"`
	Mode             string               `json:"mode"`
	ImageBase64      string               `json:"image_base64,omitempty"`
	ExpectedIdentity *FrozenJudgeIdentity `json:"expected_identity,omitempty"`
}

type nameJudgeAIResponse struct {
	Choice        *string            `json:"choice"`
	Abstained     bool               `json:"abstained"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
	Readable      *bool              `json:"readable"`
	Provider      string             `json:"provider"`
	Model         string             `json:"model"`
	PromptVersion string             `json:"promptVersion"`
	CacheHit      bool               `json:"cacheHit"`
}

type nameJudgeHTTPClient struct {
	baseURL string
	client  *http.Client
}

// NewHTTPNameJudgeProvider returns a client for the AI service judge endpoint.
// It is separate from ChartVisionProvider because a judge is not a reader.
func NewHTTPNameJudgeProvider(baseURL string) NameJudge {
	return &nameJudgeHTTPClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *nameJudgeHTTPClient) Judge(ctx context.Context, request NameJudgeRequest) (NameJudgeResult, error) {
	if c == nil || c.baseURL == "" {
		return NameJudgeResult{}, ErrNameJudgeUnavailable
	}
	if request.Mode != "shadow" && request.Mode != "active" {
		return NameJudgeResult{}, ErrNameJudgeRejected
	}
	if err := validateJudgeCandidates(request.Candidates); err != nil {
		return NameJudgeResult{}, err
	}
	if len(request.ImagePNG) > maxJudgeImageBytes {
		return NameJudgeResult{}, ErrNameJudgeRejected
	}
	if request.State.Alternatives == nil {
		request.State.Alternatives = []string{}
	}
	if request.State.FileContext == nil {
		request.State.FileContext = []string{}
	}
	payload := nameJudgeAIRequest{
		State:            request.State,
		Candidates:       request.Candidates,
		Mode:             request.Mode,
		ExpectedIdentity: request.ExpectedIdentity,
	}
	if len(request.ImagePNG) > 0 {
		payload.ImageBase64 = base64.StdEncoding.EncodeToString(request.ImagePNG)
	}
	encoded, err := json.Marshal(payload)
	if err != nil || len(encoded) > 8*1024*1024 {
		return NameJudgeResult{}, ErrNameJudgeRejected
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/names/judge", bytes.NewReader(encoded))
	if err != nil {
		return NameJudgeResult{}, ErrNameJudgeUnavailable
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(httpRequest)
	if err != nil {
		return NameJudgeResult{}, fmt.Errorf("%w: %v", ErrNameJudgeUnavailable, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxJudgeResponseBytes))
	if err != nil || len(body) >= maxJudgeResponseBytes {
		return NameJudgeResult{}, ErrNameJudgeUnavailable
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return NameJudgeResult{}, mapJudgeHTTPError(response.StatusCode, body)
	}
	var decoded nameJudgeAIResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return NameJudgeResult{}, ErrNameJudgeOutputInvalid
	}
	result := NameJudgeResult{
		Choice:        decoded.Choice,
		Abstained:     decoded.Abstained,
		Probabilities: decoded.Probabilities,
		Confidence:    decoded.Confidence,
		Readable:      decoded.Readable,
		Provider:      decoded.Provider,
		Model:         decoded.Model,
		PromptVersion: decoded.PromptVersion,
		CacheHit:      decoded.CacheHit,
	}
	if err := validateNameJudgeResult(result, request.Candidates, len(request.ImagePNG) > 0); err != nil {
		return NameJudgeResult{}, err
	}
	return result, nil
}

func mapJudgeHTTPError(status int, body []byte) error {
	var providerError struct {
		Detail string `json:"detail"`
	}
	_ = json.Unmarshal(body, &providerError)
	detail := strings.ToLower(providerError.Detail)
	switch {
	case status == http.StatusServiceUnavailable:
		if strings.Contains(detail, "not configured") || strings.Contains(detail, "jev_") || strings.Contains(detail, "liquid_") || strings.Contains(detail, "credentials") {
			return fmt.Errorf("%w: %w", ErrNameJudgeUnavailable, ErrNameJudgeSetup)
		}
		return ErrNameJudgeUnavailable
	case status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound ||
		status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500:
		return fmt.Errorf("%w: provider returned status %d", ErrNameJudgeUnavailable, status)
	case status == http.StatusUnprocessableEntity || status == http.StatusBadRequest:
		return ErrNameJudgeOutputInvalid
	default:
		return fmt.Errorf("%w: provider returned status %d", ErrNameJudgeRejected, status)
	}
}

func validateJudgeCandidates(candidates []JudgeCandidate) error {
	if len(candidates) == 0 || len(candidates) > MaxNameJudgeCandidates {
		return ErrNameJudgeRejected
	}
	values := make(map[string]struct{}, len(candidates))
	for index := range candidates {
		candidates[index].Value = strings.TrimSpace(candidates[index].Value)
		candidate := candidates[index]
		if !validJudgeNameValue(candidate.Value, true) || !validCandidateSource(candidate.Source) || candidate.Rank < 1 || candidate.Rank > MaxNameJudgeCandidates {
			return ErrNameJudgeRejected
		}
		if _, exists := values[candidate.Value]; exists {
			return ErrNameJudgeRejected
		}
		values[candidate.Value] = struct{}{}
	}
	return nil
}

func validNameJudgeProvider(provider string) bool {
	return provider == "jev" || provider == "liquid"
}

func validateNameJudgeResult(result NameJudgeResult, candidates []JudgeCandidate, hasImage bool) error {
	if !validNameJudgeProvider(result.Provider) || strings.TrimSpace(result.Model) == "" || strings.TrimSpace(result.PromptVersion) == "" {
		return ErrNameJudgeOutputInvalid
	}
	if strings.EqualFold(result.Model, "latest") || strings.EqualFold(result.PromptVersion, "latest") {
		return ErrNameJudgeOutputInvalid
	}
	if math.IsNaN(result.Confidence) || math.IsInf(result.Confidence, 0) || result.Confidence < 0 || result.Confidence > 1 {
		return ErrNameJudgeOutputInvalid
	}
	values := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		values[candidate.Value] = struct{}{}
	}
	if result.Abstained {
		if result.Choice != nil {
			return ErrNameJudgeOutputInvalid
		}
	} else {
		if result.Choice == nil {
			return ErrNameJudgeOutputInvalid
		}
		if _, ok := values[*result.Choice]; !ok {
			return ErrNameJudgeOutputInvalid
		}
	}
	expected := make(map[string]struct{}, len(values)+1)
	for value := range values {
		expected[value] = struct{}{}
	}
	expected["unclear"] = struct{}{}
	if len(result.Probabilities) != len(expected) {
		return ErrNameJudgeOutputInvalid
	}
	for value, probability := range result.Probabilities {
		if _, ok := expected[value]; !ok {
			return ErrNameJudgeOutputInvalid
		}
		if math.IsNaN(probability) || math.IsInf(probability, 0) || probability < 0 || probability > 1 {
			return ErrNameJudgeOutputInvalid
		}
	}
	if !hasImage && result.Readable != nil {
		return ErrNameJudgeOutputInvalid
	}
	return nil
}

func (s *Service) NameJudgeConfig() NameJudgeConfig {
	if s == nil {
		return NameJudgeConfig{Mode: "off", Sample: 0}
	}
	settings := s.nameJudgeSettings()
	return NameJudgeConfig{
		Mode:         settings.Mode,
		ImageEnabled: settings.ImageEnabled,
		Available:    s.NameJudge != nil && settings.Mode != "off",
		Sample:       settings.Sample,
	}
}

func (s *Service) nameJudgeSettings() NameJudgeSettings {
	settings := NameJudgeSettings{
		Mode:         strings.ToLower(strings.TrimSpace(s.NameJudgeMode)),
		Sample:       s.NameJudgeSample,
		ImageEnabled: s.NameJudgeImageEnabled,
	}
	if settings.Mode != "shadow" && settings.Mode != "active" {
		settings.Mode = "off"
	}
	if math.IsNaN(settings.Sample) || math.IsInf(settings.Sample, 0) {
		settings.Sample = 0
	}
	settings.Sample = math.Max(0, math.Min(1, settings.Sample))
	if settings.Mode == "off" {
		settings.ImageEnabled = false
	}
	return settings
}

// JudgeSuggestion asks a configured judge to choose among the supplied
// candidates, or to abstain. It only reads the suggestion, source, and file
// context; it writes nothing.
func (s *Service) JudgeSuggestion(ctx context.Context, suggestionID, actorID string, candidates []JudgeCandidate) (NameJudgeResult, error) {
	if s == nil || s.Pool == nil {
		return NameJudgeResult{}, ErrDatabaseUnavailable
	}
	settings := s.nameJudgeSettings()
	if settings.Mode == "off" {
		return NameJudgeResult{}, ErrNameJudgeUnavailable
	}
	if s.NameJudge == nil {
		return NameJudgeResult{}, ErrNameJudgeUnavailable
	}
	if err := validateJudgeCandidates(candidates); err != nil {
		return NameJudgeResult{}, err
	}
	source, err := s.loadSuggestionContext(ctx, suggestionID, actorID, false)
	if err != nil {
		return NameJudgeResult{}, err
	}
	frame, err := s.ensureNameFrame(ctx, source, nil)
	if err != nil {
		return NameJudgeResult{}, err
	}
	tightBounds, err := suggestionCropBounds(source.region, frame.Width, frame.Height, "tight")
	if err != nil {
		return NameJudgeResult{}, err
	}
	fileContext, err := s.judgeFileContext(ctx, source.sourceFileID)
	if err != nil {
		return NameJudgeResult{}, fmt.Errorf("%w: %v", ErrDatabaseUnavailable, err)
	}
	request := NameJudgeRequest{
		State: JudgeState{
			Reading:      boundedJudgeReading(source.suggestedText),
			Alternatives: boundedJudgeNames(source.alternatives, 5),
			FileContext:  boundedJudgeNames(fileContext, 20),
			Crop: JudgeCrop{
				WidthPx:        tightBounds.Width,
				HeightPx:       tightBounds.Height,
				SourceWidthPx:  frame.Width,
				SourceHeightPx: frame.Height,
				NameHeightPx:   int(math.Round(source.region.Height * float64(frame.Height))),
			},
		},
		Candidates: candidates,
		Mode:       settings.Mode,
	}
	if settings.ImageEnabled {
		// Source authorization happened before cache lookup. Images remain opt-in.
		if artifact, cropErr := s.prepareNameCrop(ctx, source, nil, nil); cropErr == nil && len(artifact.Crop.TightPNG) <= maxJudgeImageBytes {
			request.ImagePNG = artifact.Crop.TightPNG
		}
	}

	result, err := s.NameJudge.Judge(ctx, request)
	if err != nil {
		return NameJudgeResult{}, err
	}
	if err := validateNameJudgeResult(result, candidates, len(request.ImagePNG) > 0); err != nil {
		return NameJudgeResult{}, err
	}
	return result, nil
}

func boundedJudgeReading(value string) string {
	value = strings.TrimSpace(value)
	if !validJudgeText(value, true) {
		return ""
	}
	return value
}

func boundedJudgeNames(values []string, limit int) []string {
	if limit < 1 {
		return nil
	}
	out := make([]string, 0, min(limit, len(values)))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !validJudgeText(value, false) {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func validJudgeNameValue(value string, requireArabic bool) bool {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > maxJudgeNameRunes {
		return false
	}
	hasArabic := false
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
		if (character >= '\u0600' && character <= '\u06FF') || (character >= '\u0750' && character <= '\u077F') {
			hasArabic = true
		}
	}
	return !requireArabic || hasArabic
}

func validJudgeText(value string, allowEmpty bool) bool {
	if value == "" {
		return allowEmpty
	}
	if len([]rune(value)) > maxJudgeNameRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func (s *Service) judgeFileContext(ctx context.Context, sourceFileID uuid.UUID) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT name FROM (
			SELECT DISTINCT COALESCE(NULLIF(decided_text_ar, ''), NULLIF(suggested_text_ar, '')) AS name
			FROM source_tree_suggestions
			WHERE source_file_id = $1
			  AND status IN ('accepted', 'corrected')
		) accepted
		WHERE name IS NOT NULL
		ORDER BY name
		LIMIT 20
	`, sourceFileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := make([]string, 0, 20)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return names, nil
}
