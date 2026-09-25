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
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	var items []evidence.SourceView
	var err error
	if actorID == "" {
		items, err = h.Service.ListSources(r.Context())
	} else {
		items, err = h.Service.ListSourcesForActor(r.Context(), actorID)
	}
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h evidenceHandler) getSource(w http.ResponseWriter, r *http.Request) {
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	var result evidence.SourceDetail
	var err error
	if actorID == "" {
		result, err = h.Service.GetSource(r.Context(), r.PathValue("sourceID"))
	} else {
		result, err = h.Service.GetSourceForActor(r.Context(), r.PathValue("sourceID"), actorID)
	}
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

func (h evidenceHandler) listDependencies(w http.ResponseWriter, r *http.Request) {
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	result, err := h.Service.ListSourceDependencies(r.Context(), r.PathValue("sourceID"), actorID)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h evidenceHandler) createDependency(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input evidence.CreateSourceDependencyInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.CreateSourceDependency(r.Context(), r.PathValue("sourceID"), user.ID, input)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h evidenceHandler) detectDependencies(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.DetectSourceDependencies(r.Context(), r.PathValue("sourceID"), user.ID)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h evidenceHandler) reviewDependency(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input evidence.ReviewSourceDependencyInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReviewSourceDependency(r.Context(), r.PathValue("dependencyID"), user.ID, input)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h evidenceHandler) listClaims(w http.ResponseWriter, r *http.Request) {
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	items, err := h.Service.ListClaims(r.Context(), actorID)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h evidenceHandler) getClaim(w http.ResponseWriter, r *http.Request) {
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	result, err := h.Service.GetClaim(r.Context(), r.PathValue("claimID"), actorID)
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

func (h evidenceHandler) startSourceCharacterization(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input evidence.SourceCharacterizationInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.StartSourceCharacterization(r.Context(), user.ID, input)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h evidenceHandler) getSourceCharacterizationRun(w http.ResponseWriter, r *http.Request) {
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	result, err := h.Service.GetSourceCharacterizationRun(r.Context(), actorID, r.PathValue("runID"))
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h evidenceHandler) getLatestSourceCharacterization(w http.ResponseWriter, r *http.Request) {
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	result, err := h.Service.GetLatestSourceCharacterization(r.Context(), actorID, evidence.SourceCharacterizationInput{SourceID: r.URL.Query().Get("source_id"), QuestionID: r.URL.Query().Get("question_id"), ClaimID: r.URL.Query().Get("claim_id"), PlaceID: r.URL.Query().Get("place_id")})
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h evidenceHandler) reviewSourceCharacterization(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input evidence.ReviewSourceCharacterizationInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReviewSourceCharacterization(r.Context(), user.ID, r.PathValue("runID"), input)
	if err != nil {
		writeEvidenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
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
