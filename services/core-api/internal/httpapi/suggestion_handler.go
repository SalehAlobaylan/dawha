package httpapi

import (
	"errors"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/suggestions"
)

type suggestionHandler struct {
	Service *suggestions.Service
	Auth    *auth.Service
}

func (h suggestionHandler) submit(w http.ResponseWriter, r *http.Request) {
	var input suggestions.SubmitInput
	if !decodeRequest(w, r, &input) {
		return
	}
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	result, err := h.Service.Submit(r.Context(), input, actorID)
	if err != nil {
		writeSuggestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h suggestionHandler) list(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	items, err := h.Service.List(r.Context(), r.PathValue("treeID"), user.ID)
	if err != nil {
		writeSuggestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h suggestionHandler) review(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input suggestions.ReviewInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.Review(r.Context(), r.PathValue("suggestionID"), user.ID, input)
	if err != nil {
		writeSuggestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h suggestionHandler) requireUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if h.Service == nil || h.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "suggestions service is not configured"})
		return auth.User{}, false
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	return user, true
}

// writeSuggestionError maps a service error onto a status and a body. The comparison
// is by errors.Is rather than by identity, because a change-set refusal is a
// FieldError that unwraps to suggestions.ErrValidation - and that unwrapping is what
// lets this handler report WHICH field was wrong. A client that has to guess which of
// a change set's four typed blocks was malformed would have to re-implement the
// service's rules, so the field travels in the body when the service knows it.
func writeSuggestionError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "suggestion operation failed"
	body := map[string]string{"error": message}
	switch {
	case errors.Is(err, suggestions.ErrValidation):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, suggestions.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, suggestions.ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, suggestions.ErrConflict):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, suggestions.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "suggestions service is not configured"
	}
	body["error"] = message
	var fieldErr suggestions.FieldError
	if errors.As(err, &fieldErr) {
		body["field"] = fieldErr.Field
	}
	writeJSON(w, status, body)
}
