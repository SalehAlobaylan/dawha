package auth

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
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
	token, err := h.Service.createSession(r.Context(), user.ID)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	h.setSessionCookie(w, token)
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

// DevQuickSignIn is a deliberately local-only convenience for development.
// The route itself is omitted unless the server's explicit development flag is
// enabled; this second check keeps a loopback shortcut from being used through a
// public Host or a cross-origin form.
func (h Handler) DevQuickSignIn(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackRequest(r) {
		http.NotFound(w, r)
		return
	}
	user, token, err := h.Service.QuickSignIn(r.Context())
	if err != nil {
		writeAuthError(w, err)
		return
	}
	h.setSessionCookie(w, token)
	h.writeUser(w, http.StatusOK, user)
}

func isLoopbackRequest(r *http.Request) bool {
	host := r.Host
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	if !isLoopbackHost(strings.Trim(host, "[]")) {
		return false
	}

	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	remoteIP := net.ParseIP(remoteHost)
	if remoteIP == nil || !remoteIP.IsLoopback() {
		return false
	}

	if origin := r.Header.Get("Origin"); origin != "" {
		parsedOrigin, err := url.Parse(origin)
		if err != nil || (parsedOrigin.Scheme != "http" && parsedOrigin.Scheme != "https") || !isLoopbackHost(parsedOrigin.Hostname()) {
			return false
		}
	}
	return true
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
	roles, err := h.Service.CurrentRoles(r.Context(), user.ID)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "roles": roles})
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
	case ErrQuickSignInUnsafe:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "local reviewer account is unavailable"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "authentication failed"})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
