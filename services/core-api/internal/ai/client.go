package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/platform/telemetry"
)

var (
	ErrValidation  = errors.New("AI response validation failed")
	ErrUnavailable = errors.New("AI service is unavailable")
)

type NameNormalization struct {
	Original          string `json:"original"`
	Normalized        string `json:"normalized"`
	Method            string `json:"method"`
	PreservesOriginal bool   `json:"preserves_original"`
}

type EmbeddingRequest struct {
	Text       string `json:"text"`
	Dimensions int    `json:"dimensions"`
}

type EmbeddingResponse struct {
	Embedding     []float64 `json:"embedding"`
	Dimensions    int       `json:"dimensions"`
	Model         string    `json:"model"`
	Deterministic bool      `json:"deterministic"`
}

type ClassificationRequest struct {
	Text    string   `json:"text"`
	Labels  []string `json:"labels"`
	Context string   `json:"context,omitempty"`
}

type ClassificationCandidate struct {
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale"`
}

type ClassificationResponse struct {
	Candidates     []ClassificationCandidate `json:"candidates"`
	Model          string                    `json:"model"`
	ReviewRequired bool                      `json:"review_required"`
}

type ExtractionRequest struct {
	Text string `json:"text"`
}

type EntityCandidate struct {
	Text       string  `json:"text"`
	EntityType string  `json:"entity_type"`
	Confidence float64 `json:"confidence"`
	Status     string  `json:"status"`
	Rationale  string  `json:"rationale"`
}

type EntityExtractionResponse struct {
	Entities       []EntityCandidate `json:"entities"`
	Model          string            `json:"model"`
	ReviewRequired bool              `json:"review_required"`
}

type ClaimCandidate struct {
	SubjectText string  `json:"subject_text"`
	Predicate   string  `json:"predicate"`
	ObjectText  string  `json:"object_text,omitempty"`
	Confidence  float64 `json:"confidence"`
	Status      string  `json:"status"`
	Rationale   string  `json:"rationale"`
}

type ClaimExtractionResponse struct {
	Claims         []ClaimCandidate `json:"claims"`
	Model          string           `json:"model"`
	ReviewRequired bool             `json:"review_required"`
}

type EntityReference struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases,omitempty"`
}

type EntityResolutionRequest struct {
	Name       string            `json:"name"`
	Candidates []EntityReference `json:"candidates"`
}

type EntityMatch struct {
	CandidateID   string  `json:"candidate_id"`
	CandidateName string  `json:"candidate_name"`
	Score         float64 `json:"score"`
	MatchedOn     string  `json:"matched_on"`
}

type EntityResolutionResponse struct {
	Matches        []EntityMatch `json:"matches"`
	Model          string        `json:"model"`
	ReviewRequired bool          `json:"review_required"`
}

type Statement struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type ContradictionRequest struct {
	Statements []Statement `json:"statements"`
}

type ContradictionPair struct {
	LeftID     string  `json:"left_id"`
	RightID    string  `json:"right_id"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale"`
	Status     string  `json:"status"`
}

type ContradictionResponse struct {
	HasCandidateContradiction bool                `json:"has_candidate_contradiction"`
	Pairs                     []ContradictionPair `json:"pairs"`
	Model                     string              `json:"model"`
	ReviewRequired            bool                `json:"review_required"`
}

type RerankDocument struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type RerankRequest struct {
	Query     string           `json:"query"`
	Documents []RerankDocument `json:"documents"`
}

type RerankedDocument struct {
	ID      string  `json:"id"`
	Score   float64 `json:"score"`
	Excerpt string  `json:"excerpt"`
}

type RerankResponse struct {
	Documents []RerankedDocument `json:"documents"`
	Model     string             `json:"model"`
}

type SourceContext struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

type ResearchQueryRequest struct {
	Query    string          `json:"query"`
	Contexts []SourceContext `json:"contexts"`
}

type ResearchCitation struct {
	SourceID string `json:"source_id"`
	Title    string `json:"title"`
	Excerpt  string `json:"excerpt"`
}

type ResearchQueryResponse struct {
	Answer         string             `json:"answer"`
	Citations      []ResearchCitation `json:"citations"`
	Model          string             `json:"model"`
	ReviewRequired bool               `json:"review_required"`
}

var _ Provider = (*HTTPProvider)(nil)
var _ Provider = (*Client)(nil)

type Provider interface {
	NormalizeName(context.Context, string) (NameNormalization, error)
	Embed(context.Context, EmbeddingRequest) (EmbeddingResponse, error)
	Classify(context.Context, ClassificationRequest) (ClassificationResponse, error)
	ExtractEntities(context.Context, ExtractionRequest) (EntityExtractionResponse, error)
	ExtractClaims(context.Context, ExtractionRequest) (ClaimExtractionResponse, error)
	ResolveEntity(context.Context, EntityResolutionRequest) (EntityResolutionResponse, error)
	AnalyzeContradiction(context.Context, ContradictionRequest) (ContradictionResponse, error)
	Rerank(context.Context, RerankRequest) (RerankResponse, error)
	ResearchQuery(context.Context, ResearchQueryRequest) (ResearchQueryResponse, error)
}

type HTTPProvider struct {
	BaseURL    string
	HTTPClient *http.Client
	MaxRetries int
	RetryBase  time.Duration
	// Metrics records one sample per call. Nil means no metrics, which is the
	// default in every existing caller, so nothing about this field changes the
	// behaviour of a deployment that has not asked for telemetry.
	Metrics *telemetry.Metrics
}

func NewHTTPProvider(baseURL string) *HTTPProvider {
	return &HTTPProvider{
		BaseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		MaxRetries: 2,
		RetryBase:  200 * time.Millisecond,
	}
}

// WithMetrics returns the same provider, recording into a registry. The call site
// is in the ai-research HTTP client, which every process in this repository builds
// the same way, so this is one line in one constructor rather than a field
// somebody has to remember.
func (p *HTTPProvider) WithMetrics(metrics *telemetry.Metrics) *HTTPProvider {
	if p == nil {
		return nil
	}
	p.Metrics = metrics
	return p
}

type Client struct {
	Provider     Provider
	Timeout      time.Duration
	RouteTimeout time.Duration
}

func NewClient(provider Provider) *Client {
	return &Client{Provider: provider, Timeout: 20 * time.Second, RouteTimeout: routeTimeout}
}

func NewHTTPClient(baseURL string) *Client {
	return NewClient(NewHTTPProvider(baseURL))
}

func (p *HTTPProvider) NormalizeName(ctx context.Context, value string) (NameNormalization, error) {
	if err := validateNameRequest(value); err != nil {
		return NameNormalization{}, err
	}
	var result NameNormalization
	err := p.post(ctx, "/v1/normalize-name", map[string]string{"value": value}, &result)
	if err == nil {
		err = validateNameNormalization(result)
	}
	return result, err
}

func (p *HTTPProvider) Embed(ctx context.Context, input EmbeddingRequest) (EmbeddingResponse, error) {
	if err := validateEmbeddingRequest(input); err != nil {
		return EmbeddingResponse{}, err
	}
	var result EmbeddingResponse
	err := p.post(ctx, "/v1/embed", input, &result)
	if err == nil {
		err = validateEmbedding(result)
	}
	return result, err
}

func (p *HTTPProvider) Classify(ctx context.Context, input ClassificationRequest) (ClassificationResponse, error) {
	if err := validateClassificationRequest(input); err != nil {
		return ClassificationResponse{}, err
	}
	var result ClassificationResponse
	err := p.post(ctx, "/v1/classify", input, &result)
	if err == nil {
		err = validateClassification(result)
	}
	return result, err
}

func (p *HTTPProvider) ExtractEntities(ctx context.Context, input ExtractionRequest) (EntityExtractionResponse, error) {
	if err := validateExtractionRequest(input); err != nil {
		return EntityExtractionResponse{}, err
	}
	var result EntityExtractionResponse
	err := p.post(ctx, "/v1/extract/entities", input, &result)
	if err == nil {
		err = validateEntityExtraction(result)
	}
	return result, err
}

func (p *HTTPProvider) ExtractClaims(ctx context.Context, input ExtractionRequest) (ClaimExtractionResponse, error) {
	if err := validateExtractionRequest(input); err != nil {
		return ClaimExtractionResponse{}, err
	}
	var result ClaimExtractionResponse
	err := p.post(ctx, "/v1/extract/claims", input, &result)
	if err == nil {
		err = validateClaimExtraction(result)
	}
	return result, err
}

func (p *HTTPProvider) ResolveEntity(ctx context.Context, input EntityResolutionRequest) (EntityResolutionResponse, error) {
	if err := validateEntityResolutionRequest(input); err != nil {
		return EntityResolutionResponse{}, err
	}
	var result EntityResolutionResponse
	err := p.post(ctx, "/v1/resolve/entity", input, &result)
	if err == nil {
		err = validateEntityResolution(result)
	}
	return result, err
}

func (p *HTTPProvider) AnalyzeContradiction(ctx context.Context, input ContradictionRequest) (ContradictionResponse, error) {
	if err := validateContradictionRequest(input); err != nil {
		return ContradictionResponse{}, err
	}
	var result ContradictionResponse
	err := p.post(ctx, "/v1/analyze/contradiction", input, &result)
	if err == nil {
		err = validateContradiction(result)
	}
	return result, err
}

func (p *HTTPProvider) Rerank(ctx context.Context, input RerankRequest) (RerankResponse, error) {
	if err := validateRerankRequest(input); err != nil {
		return RerankResponse{}, err
	}
	var result RerankResponse
	err := p.post(ctx, "/v1/rerank", input, &result)
	if err == nil {
		err = validateRerank(result)
	}
	return result, err
}

func (p *HTTPProvider) ResearchQuery(ctx context.Context, input ResearchQueryRequest) (ResearchQueryResponse, error) {
	if err := validateResearchRequest(input); err != nil {
		return ResearchQueryResponse{}, err
	}
	var result ResearchQueryResponse
	err := p.post(ctx, "/v1/research/query", input, &result)
	if err == nil {
		err = validateResearchQuery(result)
	}
	return result, err
}

func (c *Client) NormalizeName(ctx context.Context, value string) (NameNormalization, error) {
	if err := validateNameRequest(value); err != nil {
		return NameNormalization{}, err
	}
	return callClient(c, ctx, func(callCtx context.Context) (NameNormalization, error) {
		return c.Provider.NormalizeName(callCtx, value)
	}, validateNameNormalization)
}

func (c *Client) Embed(ctx context.Context, input EmbeddingRequest) (EmbeddingResponse, error) {
	return callClient(c, ctx, func(callCtx context.Context) (EmbeddingResponse, error) { return c.Provider.Embed(callCtx, input) }, validateEmbedding)
}

func (c *Client) Classify(ctx context.Context, input ClassificationRequest) (ClassificationResponse, error) {
	return callClient(c, ctx, func(callCtx context.Context) (ClassificationResponse, error) {
		return c.Provider.Classify(callCtx, input)
	}, validateClassification)
}

func (c *Client) ExtractEntities(ctx context.Context, input ExtractionRequest) (EntityExtractionResponse, error) {
	return callClient(c, ctx, func(callCtx context.Context) (EntityExtractionResponse, error) {
		return c.Provider.ExtractEntities(callCtx, input)
	}, validateEntityExtraction)
}

func (c *Client) ExtractClaims(ctx context.Context, input ExtractionRequest) (ClaimExtractionResponse, error) {
	return callClient(c, ctx, func(callCtx context.Context) (ClaimExtractionResponse, error) {
		return c.Provider.ExtractClaims(callCtx, input)
	}, validateClaimExtraction)
}

func (c *Client) ResolveEntity(ctx context.Context, input EntityResolutionRequest) (EntityResolutionResponse, error) {
	return callClient(c, ctx, func(callCtx context.Context) (EntityResolutionResponse, error) {
		return c.Provider.ResolveEntity(callCtx, input)
	}, validateEntityResolution)
}

func (c *Client) AnalyzeContradiction(ctx context.Context, input ContradictionRequest) (ContradictionResponse, error) {
	return callClient(c, ctx, func(callCtx context.Context) (ContradictionResponse, error) {
		return c.Provider.AnalyzeContradiction(callCtx, input)
	}, validateContradiction)
}

func (c *Client) Rerank(ctx context.Context, input RerankRequest) (RerankResponse, error) {
	return callClient(c, ctx, func(callCtx context.Context) (RerankResponse, error) { return c.Provider.Rerank(callCtx, input) }, validateRerank)
}

func (c *Client) ResearchQuery(ctx context.Context, input ResearchQueryRequest) (ResearchQueryResponse, error) {
	return callClient(c, ctx, func(callCtx context.Context) (ResearchQueryResponse, error) {
		return c.Provider.ResearchQuery(callCtx, input)
	}, validateResearchQuery)
}

func callClient[T any](c *Client, ctx context.Context, call func(context.Context) (T, error), validators ...func(T) error) (T, error) {
	var zero T
	if c == nil || c.Provider == nil {
		return zero, ErrUnavailable
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := call(callCtx)
	if err == nil && len(validators) > 0 {
		err = validators[0](result)
	}
	return result, err
}

// post is the single choke point every AI call goes through, which is why it is
// also the single place a call is measured. The measurement is three things: the
// endpoint (as an enumeration, never the path a caller chose), how long it took,
// and whether it worked - plus, where the provider reports usage, the number of
// units it was billed. The request body is not measured, not sampled and not
// logged: an AI call's input is exactly the source text this repository exists to
// keep careful, and a telemetry package that grew a field for it would be a bug
// nobody would find by reading the metric names.
func (p *HTTPProvider) post(ctx context.Context, path string, input, output any) error {
	operation := telemetry.AIOperationFor(path)
	started := time.Now()
	err := p.postOnce(ctx, path, input, output)
	if p.Metrics.Enabled() {
		outcome := telemetry.AIOutcomeOK
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(ctx.Err(), context.DeadlineExceeded):
			outcome = telemetry.AIOutcomeTimeout
		case err != nil:
			outcome = telemetry.AIOutcomeError
		}
		p.Metrics.AICall(operation, outcome, time.Since(started), 0)
	}
	return err
}

func (p *HTTPProvider) postOnce(ctx context.Context, path string, input, output any) error {
	if p == nil || p.BaseURL == "" {
		return ErrUnavailable
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	client := p.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	maxRetries := p.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	base := p.RetryBase
	if base <= 0 {
		base = 200 * time.Millisecond
	}
	for attempt := 0; attempt <= maxRetries; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+path, bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json")
		response, err := client.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if attempt < maxRetries {
				if err := waitForRetry(ctx, base, attempt); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		if response.StatusCode >= 500 || response.StatusCode == http.StatusTooManyRequests {
			response.Body.Close()
			if attempt < maxRetries {
				if err := waitForRetry(ctx, base, attempt); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("%w: HTTP %d", ErrUnavailable, response.StatusCode)
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
			response.Body.Close()
			return fmt.Errorf("AI service returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
		}
		decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
		decodeErr := decoder.Decode(output)
		response.Body.Close()
		if decodeErr != nil {
			return fmt.Errorf("%w: invalid JSON: %v", ErrValidation, decodeErr)
		}
		return nil
	}
	return ErrUnavailable
}

func waitForRetry(ctx context.Context, base time.Duration, attempt int) error {
	delay := base * time.Duration(1<<attempt)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func validScore(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func validateNameRequest(value string) error {
	if strings.TrimSpace(value) == "" || len(value) > 500 {
		return ErrValidation
	}
	return nil
}

func validateNameNormalization(result NameNormalization) error {
	if result.Original == "" || result.Normalized == "" || result.Method == "" || !result.PreservesOriginal {
		return ErrValidation
	}
	return nil
}

func validateEmbeddingRequest(input EmbeddingRequest) error {
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > 20000 || input.Dimensions < 16 || input.Dimensions > 1536 {
		return ErrValidation
	}
	return nil
}

func validateEmbedding(result EmbeddingResponse) error {
	if result.Dimensions < 16 || result.Dimensions > 1536 || len(result.Embedding) != result.Dimensions || result.Model == "" {
		return ErrValidation
	}
	norm := 0.0
	for _, value := range result.Embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < -1 || value > 1 {
			return ErrValidation
		}
		norm += value * value
	}
	if norm <= 1e-12 {
		return ErrValidation
	}
	return nil
}

func validateClassificationRequest(input ClassificationRequest) error {
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > 20000 || len(input.Labels) < 2 || len(input.Labels) > 20 || len(input.Context) > 5000 {
		return ErrValidation
	}
	seen := map[string]bool{}
	for _, label := range input.Labels {
		key := strings.ToLower(strings.TrimSpace(label))
		if key == "" || seen[key] {
			return ErrValidation
		}
		seen[key] = true
	}
	return nil
}

func validateClassification(result ClassificationResponse) error {
	if result.Model == "" || !result.ReviewRequired || len(result.Candidates) == 0 {
		return ErrValidation
	}
	for _, candidate := range result.Candidates {
		if candidate.Label == "" || candidate.Rationale == "" || !validScore(candidate.Confidence) {
			return ErrValidation
		}
	}
	return nil
}

func validateExtractionRequest(input ExtractionRequest) error {
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > 50000 {
		return ErrValidation
	}
	return nil
}

func validateEntityExtraction(result EntityExtractionResponse) error {
	if result.Model == "" || !result.ReviewRequired {
		return ErrValidation
	}
	for _, entity := range result.Entities {
		if entity.Text == "" || !validScore(entity.Confidence) || entity.Status == "" {
			return ErrValidation
		}
	}
	return nil
}

func validateClaimExtraction(result ClaimExtractionResponse) error {
	if result.Model == "" || !result.ReviewRequired {
		return ErrValidation
	}
	for _, claim := range result.Claims {
		if claim.SubjectText == "" || claim.Predicate == "" || !validScore(claim.Confidence) || claim.Status == "" {
			return ErrValidation
		}
	}
	return nil
}

func validateEntityResolutionRequest(input EntityResolutionRequest) error {
	if strings.TrimSpace(input.Name) == "" || len(input.Name) > 500 || len(input.Candidates) > 100 {
		return ErrValidation
	}
	for _, candidate := range input.Candidates {
		if strings.TrimSpace(candidate.ID) == "" || len(candidate.ID) > 100 || strings.TrimSpace(candidate.Name) == "" || len(candidate.Name) > 500 {
			return ErrValidation
		}
	}
	return nil
}

func validateEntityResolution(result EntityResolutionResponse) error {
	if result.Model == "" || !result.ReviewRequired {
		return ErrValidation
	}
	for _, match := range result.Matches {
		if match.CandidateID == "" || match.CandidateName == "" || !validScore(match.Score) {
			return ErrValidation
		}
	}
	return nil
}

func validateContradictionRequest(input ContradictionRequest) error {
	if len(input.Statements) < 2 || len(input.Statements) > 50 {
		return ErrValidation
	}
	for _, statement := range input.Statements {
		if strings.TrimSpace(statement.ID) == "" || len(statement.ID) > 100 || strings.TrimSpace(statement.Text) == "" || len(statement.Text) > 20000 {
			return ErrValidation
		}
	}
	return nil
}

func validateContradiction(result ContradictionResponse) error {
	if result.Model == "" || !result.ReviewRequired {
		return ErrValidation
	}
	for _, pair := range result.Pairs {
		if pair.LeftID == "" || pair.RightID == "" || !validScore(pair.Confidence) || pair.Status == "" {
			return ErrValidation
		}
	}
	return nil
}

func validateRerankRequest(input RerankRequest) error {
	if strings.TrimSpace(input.Query) == "" || len(input.Query) > 5000 || len(input.Documents) == 0 || len(input.Documents) > 100 {
		return ErrValidation
	}
	for _, document := range input.Documents {
		if strings.TrimSpace(document.ID) == "" || len(document.ID) > 100 || strings.TrimSpace(document.Text) == "" || len(document.Text) > 20000 {
			return ErrValidation
		}
	}
	return nil
}

func validateRerank(result RerankResponse) error {
	if result.Model == "" {
		return ErrValidation
	}
	for _, document := range result.Documents {
		if document.ID == "" || document.Excerpt == "" || !validScore(document.Score) {
			return ErrValidation
		}
	}
	return nil
}

func validateResearchRequest(input ResearchQueryRequest) error {
	if strings.TrimSpace(input.Query) == "" || len(input.Query) > 5000 || len(input.Contexts) > 50 {
		return ErrValidation
	}
	for _, context := range input.Contexts {
		if strings.TrimSpace(context.ID) == "" || len(context.ID) > 100 || strings.TrimSpace(context.Title) == "" || len(context.Title) > 500 || strings.TrimSpace(context.Text) == "" || len(context.Text) > 20000 {
			return ErrValidation
		}
	}
	return nil
}

func validateResearchQuery(result ResearchQueryResponse) error {
	if result.Answer == "" || result.Model == "" || !result.ReviewRequired {
		return ErrValidation
	}
	for _, citation := range result.Citations {
		if citation.SourceID == "" || citation.Title == "" || citation.Excerpt == "" {
			return ErrValidation
		}
	}
	return nil
}
