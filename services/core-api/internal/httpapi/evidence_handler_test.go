package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
)

func TestEvidenceWriteRoutesRequireAuthentication(t *testing.T) {
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/sources", body: `{"title_ar":"مصدر","source_type":"book"}`},
		{method: http.MethodPost, path: "/api/v1/sources/00000000-0000-0000-0000-000000000001/passages", body: `{"text_ar":"نص"}`},
		{method: http.MethodPost, path: "/api/v1/sources/00000000-0000-0000-0000-000000000001/statements", body: `{"statement_text_ar":"عبارة"}`},
		{method: http.MethodPost, path: "/api/v1/claims", body: `{"subject_type":"person","subject_id":"00000000-0000-0000-0000-000000000001","predicate":"father_of","object_type":"person","object_id":"00000000-0000-0000-0000-000000000002"}`},
		{method: http.MethodPost, path: "/api/v1/claims/00000000-0000-0000-0000-000000000001/evidence", body: `{"source_passage_id":"00000000-0000-0000-0000-000000000002","relation":"supports"}`},
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

func TestEvidenceErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{evidence.ErrValidation, http.StatusBadRequest},
		{evidence.ErrNotFound, http.StatusNotFound},
		{evidence.ErrForbidden, http.StatusForbidden},
		{evidence.ErrConflict, http.StatusConflict},
		{evidence.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeEvidenceError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
	}
}
