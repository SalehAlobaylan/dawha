package httpapi

import (
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/evidence"
)

type evidenceHandler struct {
	Service *evidence.Service
	Auth    *auth.Service
}

func (h evidenceHandler) listSources(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.ListSources(r.Context())
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h evidenceHandler) getSource(w http.ResponseWriter, r *http.Request) {
	result, err := h.Service.GetSource(r.Context(), r.PathValue("sourceID"))
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h evidenceHandler) createSource(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input evidence.CreateSourceInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.CreateSource(r.Context(), user.ID, input)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h evidenceHandler) createPassage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input evidence.SourcePassageInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.CreatePassage(r.Context(), r.PathValue("sourceID"), user.ID, input)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h evidenceHandler) createStatement(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input evidence.SourceStatementInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.CreateStatement(r.Context(), r.PathValue("sourceID"), user.ID, input)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h evidenceHandler) listClaims(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.ListClaims(r.Context())
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h evidenceHandler) getClaim(w http.ResponseWriter, r *http.Request) {
	result, err := h.Service.GetClaim(r.Context(), r.PathValue("claimID"))
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h evidenceHandler) createClaim(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input evidence.CreateClaimInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.CreateClaim(r.Context(), user.ID, input)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h evidenceHandler) addEvidence(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input evidence.AddEvidenceInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.AddEvidence(r.Context(), r.PathValue("claimID"), user.ID, input)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h evidenceHandler) requireUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if h.Service == nil || h.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "evidence service is not configured"})
		return auth.User{}, false
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	return user, true
}

func writeEvidenceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "evidence operation failed"
	switch err {
	case evidence.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case evidence.ErrNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case evidence.ErrForbidden:
		status = http.StatusForbidden
		message = err.Error()
	case evidence.ErrConflict:
		status = http.StatusConflict
		message = err.Error()
	case evidence.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "evidence service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
