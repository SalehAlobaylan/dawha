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
