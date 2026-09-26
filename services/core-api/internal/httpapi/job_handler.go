package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/jobs"
)

type jobHandler struct {
	Service *jobs.Service
	Auth    *auth.Service
}

func (h jobHandler) list(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireOperator(w, r); !ok {
		return
	}
	query := r.URL.Query()
	limit := 0
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a number"})
			return
		}
		limit = parsed
	}
	items, err := h.Service.List(r.Context(), jobs.ListFilter{Status: query.Get("status"), Type: query.Get("type"), Limit: limit})
	if err != nil {
		writeJobError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h jobHandler) enqueue(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireOperator(w, r); !ok {
		return
	}
	var input jobs.EnqueueInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.Enqueue(r.Context(), input)
	if err != nil {
		writeJobError(w, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (h jobHandler) claim(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireOperator(w, r); !ok {
		return
	}
	var input jobs.ClaimInput
	if !decodeRequest(w, r, &input) {
		return
	}
	job, err := h.Service.Claim(r.Context(), input)
	if err != nil {
		writeJobError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h jobHandler) complete(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireOperator(w, r); !ok {
		return
	}
	var input jobs.CompleteInput
	if !decodeRequest(w, r, &input) {
		return
	}
	job, err := h.Service.Complete(r.Context(), r.PathValue("jobID"), input)
	if err != nil {
		writeJobError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h jobHandler) fail(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireOperator(w, r); !ok {
		return
	}
	var input jobs.FailInput
	if !decodeRequest(w, r, &input) {
		return
	}
	job, err := h.Service.Fail(r.Context(), r.PathValue("jobID"), input)
	if err != nil {
		writeJobError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (h jobHandler) recover(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireOperator(w, r); !ok {
		return
	}
	var input struct {
		OlderThanSeconds int `json:"older_than_seconds"`
	}
	if r.ContentLength != 0 && !decodeRequest(w, r, &input) {
		return
	}
	duration := time.Duration(input.OlderThanSeconds) * time.Second
	result, err := h.Service.RecoverStale(r.Context(), duration)
	if err != nil {
		writeJobError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h jobHandler) requireOperator(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if h.Service == nil || h.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "jobs service is not configured"})
		return auth.User{}, false
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	allowed, err := h.Service.CanOperate(r.Context(), user.ID)
	if err != nil {
		writeJobError(w, err)
		return auth.User{}, false
	}
	if !allowed {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "job operations require a research role"})
		return auth.User{}, false
	}
	return user, true
}

func writeJobError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "job operation failed"
	switch err {
	case jobs.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case jobs.ErrNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case jobs.ErrForbidden:
		status = http.StatusForbidden
		message = err.Error()
	case jobs.ErrConflict:
		status = http.StatusConflict
		message = err.Error()
	case jobs.ErrLeaseLost:
		// The caller is late, not unauthorised: the claim it holds has expired or
		// been taken. 409 is the honest answer, and it is the one an operator can
		// act on by re-claiming rather than by giving up on the job.
		status = http.StatusConflict
		message = err.Error()
	case jobs.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "jobs service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
