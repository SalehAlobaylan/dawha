package sourceprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNameJudgeSettingsFailClosed(t *testing.T) {
	settings := NameJudgeSettingsFromEnvironment(func(name string) string {
		return map[string]string{
			"NAME_JUDGE_MODE":   "actve",
			"NAME_JUDGE_SAMPLE": "not-a-number",
			"NAME_JUDGE_IMAGES": "sometimes",
		}[name]
	})
	if settings.Mode != "off" || settings.ImageEnabled {
		t.Fatalf("invalid settings = %+v, want off and images false", settings)
	}
	if settings.Sample != 0.2 {
		t.Fatalf("sample = %v, want the documented default", settings.Sample)
	}

	settings = NameJudgeSettingsFromEnvironment(func(name string) string {
		return map[string]string{
			"NAME_JUDGE_MODE":   "active",
			"NAME_JUDGE_SAMPLE": "2",
			"NAME_JUDGE_IMAGES": "true",
		}[name]
	})
	if settings.Mode != "active" || !settings.ImageEnabled || settings.Sample != 1 {
		t.Fatalf("valid settings = %+v", settings)
	}
}

func TestNameJudgeClientSendsTheBoundedStateAndMapsTheResult(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/names/judge" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"choice":"أحمد",
			"abstained":false,
			"probabilities":{"محمد":0.2,"أحمد":0.7,"unclear":0.1},
			"confidence":0.7,
			"readable":null,
			"provider":"jev",
			"model":"jev-pinned",
			"promptVersion":"name-judge-v1",
			"cacheHit":false
		}`))
	}))
	defer server.Close()

	provider := NewHTTPNameJudgeProvider(server.URL)
	result, err := provider.Judge(context.Background(), NameJudgeRequest{
		State: JudgeState{
			Reading:      "محمد",
			Alternatives: []string{"أحمد"},
			FileContext:  []string{"عبدالله"},
			Crop:         JudgeCrop{WidthPx: 120, HeightPx: 40, SourceWidthPx: 600, SourceHeightPx: 400, NameHeightPx: 30},
		},
		Candidates: []JudgeCandidate{
			{Value: "محمد", Source: "reader", Rank: 1},
			{Value: "أحمد", Source: "common", Rank: 2},
		},
		Mode: "shadow",
	})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if result.Choice == nil || *result.Choice != "أحمد" || result.Abstained {
		t.Fatalf("result = %+v", result)
	}
	if result.Probabilities["أحمد"] != 0.7 || result.Confidence != 0.7 {
		t.Fatalf("probabilities/confidence = %+v / %v", result.Probabilities, result.Confidence)
	}
	state, ok := received["state"].(map[string]any)
	if !ok || state["reading"] != "محمد" {
		t.Fatalf("state = %#v", received["state"])
	}
	if _, ok := received["image_base64"]; ok {
		t.Fatal("a text-only judge request included an image")
	}
	encoded, _ := json.Marshal(received)
	for _, forbidden := range []string{"source_id", "sourceId", "family_id", "familyId", "account_id", "reviewer_id", "page_image"} {
		if jsonContainsKey(encoded, forbidden) {
			t.Fatalf("request included %q: %s", forbidden, encoded)
		}
	}
}

func TestNameJudgeClientAcceptsTheLocalLiquidProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"choice":"أحمد",
			"abstained":false,
			"probabilities":{"محمد":0.2,"أحمد":0.7,"unclear":0.1},
			"confidence":0.7,
			"readable":null,
			"provider":"liquid",
			"model":"d1:3b",
			"promptVersion":"name-judge-liquid-v1",
			"cacheHit":false
		}`))
	}))
	defer server.Close()

	result, err := NewHTTPNameJudgeProvider(server.URL).Judge(context.Background(), NameJudgeRequest{
		State: JudgeState{
			Reading:      "محمد",
			Alternatives: []string{"أحمد"},
			Crop:         JudgeCrop{WidthPx: 120, HeightPx: 40, SourceWidthPx: 600, SourceHeightPx: 400, NameHeightPx: 30},
		},
		Candidates: []JudgeCandidate{
			{Value: "محمد", Source: "reader", Rank: 1},
			{Value: "أحمد", Source: "common", Rank: 2},
		},
		Mode: "active",
	})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if result.Provider != "liquid" || result.Model != "d1:3b" || result.PromptVersion != "name-judge-liquid-v1" {
		t.Fatalf("local provider metadata = %+v", result)
	}
	if result.Choice == nil || *result.Choice != "أحمد" {
		t.Fatalf("choice = %v", result.Choice)
	}
}

func TestNameJudgeResultRejectsAnUnknownProvider(t *testing.T) {
	choice := "محمد"
	result := NameJudgeResult{
		Choice:        &choice,
		Probabilities: map[string]float64{"محمد": 0.9, "unclear": 0.1},
		Confidence:    0.9,
		Provider:      "unknown",
		Model:         "d1:3b",
		PromptVersion: "name-judge-liquid-v1",
	}
	if err := validateNameJudgeResult(result, []JudgeCandidate{{Value: "محمد", Source: "reader", Rank: 1}}, false); !errors.Is(err, ErrNameJudgeOutputInvalid) {
		t.Fatalf("unknown provider = %v, want invalid output", err)
	}
}

func TestNameJudgeClientSendsTheOptInTightCropOnlyWhenPresent(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"choice":"محمد","abstained":false,
			"probabilities":{"محمد":0.8,"unclear":0.2},
			"confidence":0.8,"readable":true,"provider":"jev",
			"model":"jev-pinned","promptVersion":"name-judge-v1"
		}`))
	}))
	defer server.Close()

	result, err := NewHTTPNameJudgeProvider(server.URL).Judge(context.Background(), NameJudgeRequest{
		State:      JudgeState{Crop: JudgeCrop{WidthPx: 10, HeightPx: 10, SourceWidthPx: 100, SourceHeightPx: 100, NameHeightPx: 8}},
		Candidates: []JudgeCandidate{{Value: "محمد", Source: "reader", Rank: 1}},
		Mode:       "active",
		ImagePNG:   []byte("bounded png bytes"),
	})
	if err != nil {
		t.Fatalf("judge: %v", err)
	}
	if result.Readable == nil || !*result.Readable {
		t.Fatalf("readable = %v", result.Readable)
	}
	if received["image_base64"] == nil || received["image_base64"] == "" {
		t.Fatalf("image_base64 was not sent: %#v", received)
	}
	if received["mode"] != "active" {
		t.Fatalf("mode = %#v", received["mode"])
	}
}

func TestNameJudgeClientRejectsAChoiceOutsideTheOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"choice":"خالد",
			"abstained":false,
			"probabilities":{"محمد":0.2,"أحمد":0.7,"unclear":0.1},
			"confidence":0.7,
			"readable":null,
			"provider":"jev",
			"model":"jev-pinned",
			"promptVersion":"name-judge-v1"
		}`))
	}))
	defer server.Close()

	_, err := NewHTTPNameJudgeProvider(server.URL).Judge(context.Background(), NameJudgeRequest{
		State: JudgeState{Crop: JudgeCrop{WidthPx: 1, HeightPx: 1, SourceWidthPx: 1, SourceHeightPx: 1}},
		Candidates: []JudgeCandidate{
			{Value: "محمد", Source: "reader", Rank: 1},
			{Value: "أحمد", Source: "common", Rank: 2},
		},
		Mode: "active",
	})
	if !errors.Is(err, ErrNameJudgeOutputInvalid) {
		t.Fatalf("error = %v, want invalid output", err)
	}
}

func TestNameJudgeClientMapsSetupAndProviderFailuresToUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"detail":"hosted name judge is not configured: JEV_API_KEY is missing"}`))
	}))
	defer server.Close()

	_, err := NewHTTPNameJudgeProvider(server.URL).Judge(context.Background(), NameJudgeRequest{
		State:      JudgeState{Crop: JudgeCrop{WidthPx: 1, HeightPx: 1, SourceWidthPx: 1, SourceHeightPx: 1}},
		Candidates: []JudgeCandidate{{Value: "محمد", Source: "reader", Rank: 1}},
		Mode:       "shadow",
	})
	if !errors.Is(err, ErrNameJudgeUnavailable) || !errors.Is(err, ErrNameJudgeSetup) {
		t.Fatalf("error = %v, want unavailable and setup", err)
	}
}

func TestNameJudgeClientMapsLocalLiquidSetupFailureToUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusServiceUnavailable)
		_, _ = writer.Write([]byte(`{"detail":"local name judge is not configured: LIQUID_BASE_URL is missing"}`))
	}))
	defer server.Close()

	_, err := NewHTTPNameJudgeProvider(server.URL).Judge(context.Background(), NameJudgeRequest{
		State:      JudgeState{Crop: JudgeCrop{WidthPx: 1, HeightPx: 1, SourceWidthPx: 1, SourceHeightPx: 1}},
		Candidates: []JudgeCandidate{{Value: "محمد", Source: "reader", Rank: 1}},
		Mode:       "shadow",
	})
	if !errors.Is(err, ErrNameJudgeUnavailable) || !errors.Is(err, ErrNameJudgeSetup) {
		t.Fatalf("error = %v, want unavailable and setup", err)
	}
}

func TestNameJudgeResultRejectsReadabilityWithoutAnImageAndNonFiniteNumbers(t *testing.T) {
	choice := "محمد"
	readable := true
	candidates := []JudgeCandidate{{Value: "محمد", Source: "reader", Rank: 1}}
	result := NameJudgeResult{
		Choice:        &choice,
		Probabilities: map[string]float64{"محمد": 0.9, "unclear": 0.1},
		Confidence:    0.9,
		Readable:      &readable,
		Provider:      "jev",
		Model:         "jev-pinned",
		PromptVersion: "name-judge-v1",
	}
	if err := validateNameJudgeResult(result, candidates, false); !errors.Is(err, ErrNameJudgeOutputInvalid) {
		t.Fatalf("readability without image = %v", err)
	}
	result.Readable = nil
	result.Confidence = math.NaN()
	if err := validateNameJudgeResult(result, candidates, false); !errors.Is(err, ErrNameJudgeOutputInvalid) {
		t.Fatalf("NaN confidence = %v", err)
	}
}

func TestBoundedJudgeInputsDropBlankNamesAndCapTheContext(t *testing.T) {
	names := boundedJudgeNames([]string{" محمد ", "", "أحمد", "محمد", "خالد", "سالم", "علي", "جاسم"}, 3)
	if len(names) != 3 || names[0] != "محمد" || names[1] != "أحمد" || names[2] != "خالد" {
		t.Fatalf("bounded names = %+v", names)
	}
	if boundedJudgeReading(strings.Repeat("ا", maxJudgeNameRunes+1)) != "" {
		t.Fatal("an oversized reading was not dropped")
	}
}

func TestNameJudgeConfigNeverAdvertisesAnOffJudge(t *testing.T) {
	service := &Service{
		NameJudge:             &stubNameJudge{},
		NameJudgeMode:         "off",
		NameJudgeSample:       0.2,
		NameJudgeImageEnabled: true,
	}
	config := service.NameJudgeConfig()
	if config.Mode != "off" || config.Available || config.ImageEnabled {
		t.Fatalf("config = %+v", config)
	}
	service.NameJudgeMode = "active"
	config = service.NameJudgeConfig()
	if config.Mode != "active" || !config.Available || !config.ImageEnabled || config.Sample != 0.2 {
		t.Fatalf("active config = %+v", config)
	}
}

type stubNameJudge struct{}

func (stubNameJudge) Judge(context.Context, NameJudgeRequest) (NameJudgeResult, error) {
	return NameJudgeResult{}, nil
}

func jsonContainsKey(encoded []byte, key string) bool {
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		return false
	}
	return containsKey(value, key)
}

func containsKey(value any, key string) bool {
	switch typed := value.(type) {
	case map[string]any:
		if _, ok := typed[key]; ok {
			return true
		}
		for _, child := range typed {
			if containsKey(child, key) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsKey(child, key) {
				return true
			}
		}
	}
	return false
}
