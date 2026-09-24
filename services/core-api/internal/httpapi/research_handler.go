package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/research"
)

type researchHandler struct {
	Service *research.Service
	Auth    *auth.Service
	Logger  *slog.Logger
}

func (h researchHandler) query(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "research service is not configured"})
		return
	}
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	var input research.QueryInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.Query(r.Context(), input, actorID)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("research query failed", "error", err)
		}
		writeResearchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeResearchError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "research operation failed"
	switch {
	case errors.Is(err, research.ErrValidation):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, research.ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, research.ErrAIUnavailable), errors.Is(err, research.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "research service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
