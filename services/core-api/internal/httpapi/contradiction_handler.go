package httpapi

import (
	"errors"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/contradiction"
)

type contradictionHandler struct {
	Service *contradiction.Service
	Auth    *auth.Service
}

func (h contradictionHandler) start(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.StartRun(r.Context(), user.ID)
	if err != nil {
		writeContradictionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h contradictionHandler) getRun(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetRun(r.Context(), user.ID, r.PathValue("runID"))
	if err != nil {
		writeContradictionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h contradictionHandler) listFindings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.ListFindings(r.Context(), user.ID, r.URL.Query().Get("run_id"), r.URL.Query().Get("status"))
	if err != nil {
		writeContradictionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h contradictionHandler) getFinding(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetFinding(r.Context(), user.ID, r.PathValue("findingID"))
	if err != nil {
		writeContradictionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h contradictionHandler) review(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input contradiction.ReviewInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReviewFinding(r.Context(), user.ID, r.PathValue("findingID"), input)
	if err != nil {
		writeContradictionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h contradictionHandler) requireUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if h.Service == nil || h.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "contradiction service is not configured"})
		return auth.User{}, false
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	return user, true
}

func writeContradictionError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "contradiction operation failed"
	switch {
	case errors.Is(err, contradiction.ErrValidation):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, contradiction.ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, contradiction.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, contradiction.ErrConflict):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, contradiction.ErrQueueUnavailable), errors.Is(err, contradiction.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "contradiction service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
