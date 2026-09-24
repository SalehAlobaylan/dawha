package httpapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSourceProcessingRoutesRequireAuthentication(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "source.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("نص عربي"))
	_ = writer.Close()
	upload := httptest.NewRequest(http.MethodPost, "/api/v1/sources/00000000-0000-0000-0000-000000000001/files", &body)
	upload.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(recorder, upload)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("upload status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/sources/00000000-0000-0000-0000-000000000001/processing", nil)
	recorder = httptest.NewRecorder()
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("processing status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}
