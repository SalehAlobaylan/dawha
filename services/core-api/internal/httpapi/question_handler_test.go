package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/questions"
)

func TestQuestionWriteRoutesRequireAuthentication(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/questions", body: `{"title_ar":"سؤال"}`},
		{method: http.MethodPatch, path: "/api/v1/questions/00000000-0000-0000-0000-000000000001", body: `{"status":"resolved"}`},
		{method: http.MethodPost, path: "/api/v1/questions/00000000-0000-0000-0000-000000000001/notes", body: `{"note_ar":"ملاحظة"}`},
		{method: http.MethodPost, path: "/api/v1/questions/00000000-0000-0000-0000-000000000001/entities", body: `{"entity_type":"person","entity_id":"10000000-0000-0000-0000-000000000001"}`},
		{method: http.MethodPost, path: "/api/v1/questions/00000000-0000-0000-0000-000000000001/findings", body: `{"finding_id":"70000000-0000-0000-0000-000000000001"}`},
		{method: http.MethodPost, path: "/api/v1/disputes", body: `{"title_ar":"خلاف"}`},
		{method: http.MethodPatch, path: "/api/v1/disputes/00000000-0000-0000-0000-000000000001", body: `{"status":"under_review"}`},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(testCase.method, testCase.path, strings.NewReader(testCase.body))
		request.Header.Set("Content-Type", "application/json")
		NewRouter(Dependencies{}).ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected status %d, got %d", testCase.method, testCase.path, http.StatusUnauthorized, recorder.Code)
		}
	}
}

func TestQuestionErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{questions.ErrValidation, http.StatusBadRequest},
		{questions.ErrNotFound, http.StatusNotFound},
		{questions.ErrForbidden, http.StatusForbidden},
		{questions.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeQuestionError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
