package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterRequiresJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"user@example.com"}`))
	request.Header.Set("Content-Type", "application/json")

	Handler{Service: NewService(nil)}.Register(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable service status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestLoginWithoutDatabaseIsUnavailable(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"user@example.com","password":"password1234"}`))
	request.Header.Set("Content-Type", "application/json")

	Handler{Service: NewService(nil)}.Login(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected unavailable service status %d, got %d", http.StatusServiceUnavailable, recorder.Code)
	}
}

func TestDevQuickSignInRequiresALoopbackRequest(t *testing.T) {
	tests := []struct {
		name       string
		host       string
		remoteAddr string
		origin     string
	}{
		{name: "remote client", host: "localhost:8080", remoteAddr: "192.0.2.10:1234", origin: "http://localhost:5173"},
		{name: "public host", host: "dawha.example:8080", remoteAddr: "127.0.0.1:1234", origin: "http://localhost:5173"},
		{name: "cross origin", host: "localhost:8080", remoteAddr: "127.0.0.1:1234", origin: "https://attacker.example"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/dev/quick-sign-in", nil)
			request.Host = testCase.host
			request.RemoteAddr = testCase.remoteAddr
			request.Header.Set("Origin", testCase.origin)
			recorder := httptest.NewRecorder()

			Handler{Service: NewService(nil)}.DevQuickSignIn(recorder, request)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func TestDevQuickSignInReportsMissingDatabaseAfterLocalChecks(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/dev/quick-sign-in", nil)
	request.Host = "localhost:8080"
	request.RemoteAddr = "127.0.0.1:1234"
	request.Header.Set("Origin", "http://localhost:5173")
	recorder := httptest.NewRecorder()

	Handler{Service: NewService(nil)}.DevQuickSignIn(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}
