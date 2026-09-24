package httpapi

import (
	"errors"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/entityresolution"
)

type entityResolutionHandler struct {
	Service *entityresolution.Service
	Auth    *auth.Service
}

func (h entityResolutionHandler) run(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input entityresolution.RunInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.Run(r.Context(), user.ID, input)
	if err != nil {
		writeEntityResolutionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h entityResolutionHandler) getRun(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetRun(r.Context(), user.ID, r.PathValue("runID"))
	if err != nil {
		writeEntityResolutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h entityResolutionHandler) listCandidates(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.ListCandidates(r.Context(), user.ID, r.URL.Query().Get("status"), r.URL.Query().Get("entity_type"))
	if err != nil {
		writeEntityResolutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h entityResolutionHandler) listMerges(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.ListMerges(r.Context(), user.ID)
	if err != nil {
		writeEntityResolutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h entityResolutionHandler) getCandidate(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetCandidate(r.Context(), user.ID, r.PathValue("candidateID"))
	if err != nil {
		writeEntityResolutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h entityResolutionHandler) reviewCandidate(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input entityresolution.ReviewInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReviewCandidate(r.Context(), user.ID, r.PathValue("candidateID"), input)
	if err != nil {
		writeEntityResolutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h entityResolutionHandler) mergeCandidate(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input entityresolution.MergeInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.MergeCandidate(r.Context(), user.ID, r.PathValue("candidateID"), input)
	if err != nil {
		writeEntityResolutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h entityResolutionHandler) reverseMerge(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input struct {
		ReasonAR string `json:"reason_ar"`
	}
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReverseMerge(r.Context(), user.ID, r.PathValue("mergeID"), input.ReasonAR)
	if err != nil {
		writeEntityResolutionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h entityResolutionHandler) requireUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if h.Service == nil || h.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "entity resolution service is not configured"})
		return auth.User{}, false
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	return user, true
}

func writeEntityResolutionError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "entity resolution operation failed"
	switch {
	case errors.Is(err, entityresolution.ErrValidation):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, entityresolution.ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, entityresolution.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, entityresolution.ErrConflict):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, entityresolution.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "entity resolution service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
