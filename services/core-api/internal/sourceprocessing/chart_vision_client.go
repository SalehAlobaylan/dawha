package sourceprocessing

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const ChartVisionModel = "deepseek-flash"
const ChartLocateOutputTokenLimit = 16384
const ChartReadOutputTokenLimit = 4096

var ErrChartVisionUnavailable = errors.New("chart vision provider is unavailable")
var ErrChartVisionSetup = errors.New("hosted chart vision needs setup")
var ErrChartVisionRejected = errors.New("chart vision request was rejected")
var ErrChartVisionTruncated = errors.New("chart vision output reached its token limit")
var ErrChartVisionOutputInvalid = errors.New("chart vision output failed local validation")

type chartVisionOutputValidationError struct {
	category string
}

func (e chartVisionOutputValidationError) Error() string {
	return ErrChartVisionOutputInvalid.Error()
}

func (e chartVisionOutputValidationError) Is(target error) bool {
	return target == ErrChartVisionOutputInvalid
}

type ChartVisionProvider interface {
	LocateTile(context.Context, string, []byte) (ChartTileReading, error)
	ReadBatch(context.Context, []ChartCropInput) ([]ChartNameReading, error)
	// SuggestNames asks the same vision model for spelling candidates for ONE
	// tight name crop. It is an on-demand aid, not a reading: the caller presents
	// the names as hypotheses beside the source and never writes one without a
	// human decision.
	SuggestNames(ctx context.Context, tightPNG []byte) ([]string, error)
}

type ChartBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type ChartPoint [2]float64

type ChartBranchProposal struct {
	ID      string       `json:"id"`
	Label   string       `json:"label,omitempty"`
	Polygon []ChartPoint `json:"polygon"`
}

type ChartLabelProposal struct {
	ID          string   `json:"id"`
	Box         ChartBox `json:"box"`
	BranchIndex *int     `json:"branch_index,omitempty"`
	Legibility  string   `json:"legibility"`
}

type ChartConnectorProposal struct {
	ID     string       `json:"id"`
	Points []ChartPoint `json:"points"`
	Kind   string       `json:"kind"`
}

type ChartTileReading struct {
	Branches         []ChartBranchProposal    `json:"branches"`
	Labels           []ChartLabelProposal     `json:"labels"`
	Connectors       []ChartConnectorProposal `json:"connectors"`
	ResidualRegions  []ChartBox               `json:"residual_regions"`
	CompletenessNote string                   `json:"completeness_note,omitempty"`
}

type ChartCropInput struct {
	OccurrenceID string
	TightPNG     []byte
	ContextPNG   []byte
}

type ChartNameReading struct {
	OccurrenceID   string   `json:"occurrence_id"`
	Status         string   `json:"status"`
	TextAR         string   `json:"text_ar"`
	Alternatives   []string `json:"alternatives"`
	UncertainSpans []string `json:"uncertain_spans"`
}

type chartVisionHTTPClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPChartVisionProvider(baseURL string) ChartVisionProvider {
	return &chartVisionHTTPClient{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		client:  &http.Client{Timeout: 115 * time.Second},
	}
}

func (c *chartVisionHTTPClient) LocateTile(ctx context.Context, tileID string, png []byte) (ChartTileReading, error) {
	var response ChartTileReading
	if len(png) == 0 || len(png) > 8<<20 {
		return response, ErrValidation
	}
	request := map[string]any{"tile_id": tileID, "image_base64": base64.StdEncoding.EncodeToString(png)}
	if err := c.post(ctx, "/v1/vision/chart/locate", request, &response); err != nil {
		if errors.Is(err, ErrValidation) {
			return ChartTileReading{}, ErrChartVisionOutputInvalid
		}
		return ChartTileReading{}, err
	}
	if err := validateChartTileReading(response); err != nil {
		return ChartTileReading{}, chartVisionOutputValidationError{category: "geometry"}
	}
	return response, nil
}

func (c *chartVisionHTTPClient) ReadBatch(ctx context.Context, items []ChartCropInput) ([]ChartNameReading, error) {
	if len(items) == 0 || len(items) > 8 {
		return nil, ErrValidation
	}
	encoded := make([]map[string]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.OccurrenceID == "" || len(item.TightPNG) == 0 || len(item.ContextPNG) == 0 || len(item.TightPNG) > 4<<20 || len(item.ContextPNG) > 4<<20 {
			return nil, ErrValidation
		}
		if _, ok := seen[item.OccurrenceID]; ok {
			return nil, ErrValidation
		}
		seen[item.OccurrenceID] = struct{}{}
		encoded = append(encoded, map[string]string{
			"occurrence_id":        item.OccurrenceID,
			"tight_image_base64":   base64.StdEncoding.EncodeToString(item.TightPNG),
			"context_image_base64": base64.StdEncoding.EncodeToString(item.ContextPNG),
		})
	}
	var response struct {
		Readings []ChartNameReading `json:"readings"`
	}
	if err := c.post(ctx, "/v1/vision/chart/read", map[string]any{"items": encoded}, &response); err != nil {
		return nil, err
	}
	if len(response.Readings) != len(items) {
		return nil, ErrValidation
	}
	seen = make(map[string]struct{}, len(items))
	for _, reading := range response.Readings {
		if _, ok := seen[reading.OccurrenceID]; ok || !validReadingStatus(reading.Status) || len([]rune(reading.TextAR)) > 200 || len(reading.Alternatives) > 5 || len(reading.UncertainSpans) > 20 {
			return nil, ErrValidation
		}
		if (reading.Status == "readable" || reading.Status == "partial") && strings.TrimSpace(reading.TextAR) == "" || reading.Status == "unreadable" && strings.TrimSpace(reading.TextAR) != "" {
			return nil, ErrValidation
		}
		for _, value := range append(append([]string(nil), reading.Alternatives...), reading.UncertainSpans...) {
			if len([]rune(value)) > 200 {
				return nil, ErrValidation
			}
		}
		seen[reading.OccurrenceID] = struct{}{}
	}
	for _, item := range items {
		if _, ok := seen[item.OccurrenceID]; !ok {
			return nil, ErrValidation
		}
	}
	return response.Readings, nil
}

func (c *chartVisionHTTPClient) SuggestNames(ctx context.Context, tightPNG []byte) ([]string, error) {
	if len(tightPNG) == 0 || len(tightPNG) > 4<<20 {
		return nil, ErrValidation
	}
	var response struct {
		Candidates []string `json:"candidates"`
	}
	if err := c.post(ctx, "/v1/vision/chart/candidates", map[string]any{
		"image_base64": base64.StdEncoding.EncodeToString(tightPNG),
	}, &response); err != nil {
		return nil, err
	}
	if len(response.Candidates) > 8 {
		return nil, ErrValidation
	}
	seen := make(map[string]struct{}, len(response.Candidates))
	names := make([]string, 0, len(response.Candidates))
	for _, candidate := range response.Candidates {
		name := strings.TrimSpace(candidate)
		if name == "" {
			continue
		}
		if len([]rune(name)) > 200 {
			return nil, ErrValidation
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, nil
}

func (c *chartVisionHTTPClient) post(ctx context.Context, path string, input, output any) error {
	if c == nil || c.baseURL == "" {
		return ErrChartVisionUnavailable
	}
	data, err := json.Marshal(input)
	if err != nil || len(data) > 24<<20 {
		return ErrValidation
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return ErrChartVisionUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrChartVisionUnavailable, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil || len(body) >= 2<<20 {
		return ErrChartVisionUnavailable
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == http.StatusServiceUnavailable {
			var providerError struct {
				Detail string `json:"detail"`
			}
			_ = json.Unmarshal(body, &providerError)
			detail := strings.ToLower(providerError.Detail)
			if strings.Contains(detail, "not configured") || strings.Contains(detail, "sdk is unavailable") || strings.Contains(detail, "credentials") {
				return fmt.Errorf("%w: %w", ErrChartVisionUnavailable, ErrChartVisionSetup)
			}
			return ErrChartVisionUnavailable
		}
		if response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
			return fmt.Errorf("%w: provider returned status %d", ErrChartVisionUnavailable, response.StatusCode)
		}
		if response.StatusCode == http.StatusUnprocessableEntity {
			var providerError struct {
				Detail string `json:"detail"`
			}
			_ = json.Unmarshal(body, &providerError)
			if strings.Contains(strings.ToLower(providerError.Detail), "truncated") {
				return ErrChartVisionTruncated
			}
			return chartVisionOutputValidationError{category: chartVisionOutputCategory(providerError.Detail)}
		}
		return fmt.Errorf("%w: provider returned status %d", ErrChartVisionRejected, response.StatusCode)
	}
	if err := json.Unmarshal(body, output); err != nil {
		return ErrValidation
	}
	return nil
}

func chartVisionOutputCategory(detail string) string {
	switch {
	case strings.Contains(detail, "vision_branch_reference_invalid"):
		return "branch_reference"
	case strings.Contains(detail, "vision_geometry_invalid"):
		return "geometry"
	case strings.Contains(detail, "vision_reading_invalid"):
		return "reading"
	default:
		return "schema"
	}
}

func validateChartTileReading(reading ChartTileReading) error {
	if len(reading.Branches) > 200 || len(reading.Labels) > 250 || len(reading.Connectors) > 500 || len(reading.ResidualRegions) > 100 {
		return ErrValidation
	}
	for _, branch := range reading.Branches {
		if len(branch.Polygon) < 3 || len(branch.Polygon) > 100 {
			return ErrValidation
		}
		for _, point := range branch.Polygon {
			if !unit(point[0]) || !unit(point[1]) {
				return ErrValidation
			}
		}
	}
	for _, label := range reading.Labels {
		if !validChartBox(label.Box) || (label.BranchIndex != nil && (*label.BranchIndex < 0 || *label.BranchIndex >= len(reading.Branches))) {
			return ErrValidation
		}
		if label.Legibility != "clear" && label.Legibility != "uncertain" && label.Legibility != "unreadable" {
			return ErrValidation
		}
	}
	for _, connector := range reading.Connectors {
		if len(connector.Points) < 2 || len(connector.Points) > 300 {
			return ErrValidation
		}
		for _, point := range connector.Points {
			if !unit(point[0]) || !unit(point[1]) {
				return ErrValidation
			}
		}
	}
	for _, region := range reading.ResidualRegions {
		if !validChartBox(region) {
			return ErrValidation
		}
	}
	return nil
}

func validChartBox(box ChartBox) bool {
	return unit(box.X) && unit(box.Y) && box.Width > 0 && box.Height > 0 && box.X+box.Width <= 1.000001 && box.Y+box.Height <= 1.000001
}

func validReadingStatus(status string) bool {
	return status == "readable" || status == "partial" || status == "unreadable"
}
func unit(value float64) bool { return value >= 0 && value <= 1 }
