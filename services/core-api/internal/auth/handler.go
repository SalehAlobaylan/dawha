package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type Handler struct {
	Service       *Service
	SecureCookies bool
}

type registerRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name_ar"`
	Password    string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h Handler) Register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	user, err := h.Service.Register(r.Context(), request.Email, request.DisplayName, request.Password)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	h.writeUser(w, http.StatusCreated, user)
}

func (h Handler) Login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	user, token, err := h.Service.Login(r.Context(), request.Email, request.Password)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	h.setSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"user": user})
}

func (h Handler) Me(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	user, err := h.Service.UserFromToken(r.Context(), cookie.Value)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	h.writeUser(w, http.StatusOK, user)
}

func (h Handler) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		if revokeErr := h.Service.RevokeToken(r.Context(), cookie.Value); revokeErr != nil {
			writeAuthError(w, revokeErr)
			return
		}
	}
	h.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "signed_out"})
}

func (h Handler) writeUser(w http.ResponseWriter, status int, user User) {
	writeJSON(w, status, map[string]any{"user": user})
}

func (h Handler) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(h.Service.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil || strings.TrimSpace(r.Header.Get("Content-Type")) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid JSON is required"})
		return false
	}
	return true
}

func writeAuthError(w http.ResponseWriter, err error) {
	switch err {
	case ErrEmailTaken, ErrWeakPassword:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case ErrInvalidCredentials:
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid email or password"})
	case ErrDatabaseUnavailable:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication is not configured"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authentication failed"})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
