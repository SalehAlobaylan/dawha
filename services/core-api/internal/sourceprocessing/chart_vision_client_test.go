package sourceprocessing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChartVisionClientPreservesTokenTruncationSignal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = writer.Write([]byte(`{"detail":"chart output truncated at configured token limit"}`))
	}))
	defer server.Close()

	provider := NewHTTPChartVisionProvider(server.URL)
	_, err := provider.LocateTile(context.Background(), "tile_1", []byte("bounded png bytes"))
	if !errors.Is(err, ErrChartVisionTruncated) {
		t.Fatalf("locate error = %v, want a truncation signal for the one subdivision pass", err)
	}
}

func TestChartVisionClientClassifiesSafeProviderValidationDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = writer.Write([]byte(`{"detail":"vision_geometry_invalid:labels.0.box.x:less_than_equal"}`))
	}))
	defer server.Close()

	provider := NewHTTPChartVisionProvider(server.URL)
	_, err := provider.LocateTile(context.Background(), "tile_1", []byte("bounded png bytes"))
	if !errors.Is(err, ErrChartVisionOutputInvalid) {
		t.Fatalf("locate error = %v, want a typed output-validation error", err)
	}
	if got := chartErrorCode(err); got != "provider_output_invalid_geometry" {
		t.Fatalf("output error code = %q, want provider_output_invalid_geometry", got)
	}
}

func TestSuggestNamesPostsTheTightCropAndCleansTheCandidates(t *testing.T) {
	var received struct {
		ImageBase64 string `json:"image_base64"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/vision/chart/candidates" || request.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode candidate request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"candidates":[" محمد ","أحمد","محمد","  "]} `))
	}))
	defer server.Close()

	provider := NewHTTPChartVisionProvider(server.URL)
	names, err := provider.SuggestNames(context.Background(), []byte("tight png bytes"))
	if err != nil {
		t.Fatalf("suggest names: %v", err)
	}
	if got := base64.StdEncoding.EncodeToString([]byte("tight png bytes")); received.ImageBase64 != got {
		t.Fatalf("the tight crop was not the image sent: %q", received.ImageBase64)
	}
	if len(names) != 2 || names[0] != "محمد" || names[1] != "أحمد" {
		t.Fatalf("candidates were not trimmed and de-duplicated: %+v", names)
	}
}

func TestChartReaderHTTPFailuresDistinguishPolicySetupAndOutage(t *testing.T) {
	for _, test := range []struct {
		status int
		detail string
		want   error
	}{
		{403, "reader_disabled_by_policy", ErrChartVisionPolicy},
		{401, "private provider error must not escape", ErrChartVisionSetup},
		{403, "credentials rejected", ErrChartVisionSetup},
		{503, "hosted chart vision is not configured", ErrChartVisionSetup},
		{503, "temporarily unavailable", ErrChartVisionUnavailable},
		{429, "rate limit", ErrChartVisionUnavailable},
	} {
		t.Run(test.detail, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_ = json.NewEncoder(w).Encode(map[string]string{"detail": test.detail})
			}))
			defer server.Close()
			_, err := NewHTTPChartVisionProvider(server.URL).SuggestNames(context.Background(), []byte("fixture"))
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v want=%v", err, test.want)
			}
		})
	}
}
