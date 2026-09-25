package httpapi

import (
	"errors"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/dictionary"
)

type dictionaryHandler struct {
	Service *dictionary.Service
	Auth    *auth.Service
}

// actorID resolves the optional session. The dictionary stays reachable without a
// session; the visibility policy then answers as the anonymous actor.
func (h dictionaryHandler) actorID(r *http.Request) string {
	if h.Auth == nil {
		return ""
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		return ""
	}
	return user.ID
}

func (h dictionaryHandler) index(w http.ResponseWriter, r *http.Request) {
	result, err := h.Service.ListIndex(r.Context(), r.URL.Query().Get("kind"), r.URL.Query().Get("q"), h.actorID(r))
	if err != nil {
		writeDictionaryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h dictionaryHandler) detail(w http.ResponseWriter, r *http.Request) {
	result, err := h.Service.Get(r.Context(), r.PathValue("kind"), r.PathValue("id"), h.actorID(r))
	if err != nil {
		writeDictionaryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeDictionaryError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "dictionary operation failed"
	switch {
	case errors.Is(err, dictionary.ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, dictionary.ErrValidation):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, dictionary.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, dictionary.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "dictionary service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
