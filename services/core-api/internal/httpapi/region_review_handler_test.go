package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/regionreview"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
)

// laneRequest performs one request the way the workbench would: a session
// cookie, a JSON body, and nothing else.
func laneRequest(router http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	var request *http.Request
	if body == "" {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: token})
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestRegionReviewErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{regionreview.ErrValidation, http.StatusBadRequest},
		{regionreview.ErrUnauthenticated, http.StatusUnauthorized},
		{regionreview.ErrForbidden, http.StatusForbidden},
		{regionreview.ErrNotFound, http.StatusNotFound},
		{regionreview.ErrAlreadyDecided, http.StatusConflict},
		{regionreview.ErrCannotReopen, http.StatusConflict},
		// A stored scan that is missing or unreadable is a configuration or
		// upload state, not a client mistake — and never a silent empty image.
		{regionreview.ErrCropUnavailable, http.StatusServiceUnavailable},
		{regionreview.ErrDatabaseUnavailable, http.StatusServiceUnavailable},
	}
	for _, testCase := range cases {
		recorder := httptest.NewRecorder()
		writeRegionReviewError(recorder, testCase.err)
		if recorder.Code != testCase.status {
			t.Fatalf("error %v: expected status %d, got %d", testCase.err, testCase.status, recorder.Code)
		}
		var payload map[string]string
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil || payload["error"] == "" {
			t.Fatalf("error %v: body %q is not an error object", testCase.err, recorder.Body.String())
		}
	}
}

func TestRegionReviewAnonymousBodyRefusedWithoutAService(t *testing.T) {
	// A malformed decision body is a transport 400 before the service is
	// ever consulted — the gate order a hostile client exercises first.
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/region-candidates/00000000-0000-0000-0000-00000000e004/decide", strings.NewReader("not json"))
	request.Header.Set("Content-Type", "application/json")
	NewRouter(Dependencies{}).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed body status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestRegionReviewLaneRolesOverHTTP(t *testing.T) {
	fixture := testsupport.New(t)
	moderator := actor.RegisterAndLogin(t, fixture, "مشرف القبائل")
	fixture.GrantRole(moderator.User.ID, "moderator")
	reader := actor.RegisterAndLogin(t, fixture, "قارئ القبائل")

	router := NewRouter(Dependencies{DB: fixture.Pool()})

	cases := []struct {
		name   string
		method string
		path   string
		body   string
		token  string
		status int
	}{
		{"anonymous list", http.MethodGet, "/api/v1/admin/region-candidates", "", "", http.StatusUnauthorized},
		{"reader list", http.MethodGet, "/api/v1/admin/region-candidates", "", reader.Token, http.StatusForbidden},
		{"moderator list", http.MethodGet, "/api/v1/admin/region-candidates", "", moderator.Token, http.StatusOK},
		{"anonymous decide", http.MethodPost, "/api/v1/admin/region-candidates/00000000-0000-0000-0000-00000000e004/decide", `{"action":"reject"}`, "", http.StatusUnauthorized},
		{"moderator decide on unknown candidate", http.MethodPost, "/api/v1/admin/region-candidates/00000000-0000-0000-0000-00000000e004/decide", `{"action":"reject"}`, moderator.Token, http.StatusNotFound},
		{"moderator bogus action", http.MethodPost, "/api/v1/admin/region-candidates/00000000-0000-0000-0000-00000000e004/decide", `{"action":"publish"}`, moderator.Token, http.StatusBadRequest},
		{"moderator history of unknown candidate", http.MethodGet, "/api/v1/admin/region-candidates/00000000-0000-0000-0000-00000000e004/history", "", moderator.Token, http.StatusNotFound},
	}
	for _, testCase := range cases {
		recorder := laneRequest(router, testCase.method, testCase.path, testCase.body, testCase.token)
		if recorder.Code != testCase.status {
			t.Fatalf("%s: status %d, want %d (body %s)", testCase.name, recorder.Code, testCase.status, recorder.Body.String())
		}
	}

	// The moderator's empty lane is a real empty lane, not an error page.
	empty := laneRequest(router, http.MethodGet, "/api/v1/admin/region-candidates", "", moderator.Token)
	var report regionreview.ListResponse
	if err := json.Unmarshal(empty.Body.Bytes(), &report); err != nil {
		t.Fatalf("empty lane body: %v (%s)", err, empty.Body.String())
	}
	if report.Status != "suggested" || report.Candidates == nil || len(report.Candidates) != 0 || report.Truncated {
		t.Fatalf("empty lane = %+v", report)
	}
}
