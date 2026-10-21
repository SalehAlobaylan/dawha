package sourceprocessing

import (
	"context"
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
