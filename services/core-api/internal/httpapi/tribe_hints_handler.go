package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/tribehints"
)

// tribeHintsHandler is the record-hints lane's transport (plan 031-C): the
// bounded hint queue (cards, one per tribe × source), the matcher's run over
// one source, and the per-card decisions. The service keeps the rules (a hint
// never auto-applies; a dismissal is the reviewer's own hide; an accepted
// hint is a documented receipt).
type tribeHintsHandler struct {
	Service *tribehints.Service
	Auth    *auth.Service
}

func (h tribeHintsHandler) runMatch(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SourceID string `json:"sourceId"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "match body must be JSON with a sourceId"})
		return
	}
	result, err := h.Service.MatchTribeHints(r.Context(), input.SourceID)
	if err != nil {
		writeTribeHintsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h tribeHintsHandler) listHints(w http.ResponseWriter, r *http.Request) {
	response, err := h.Service.List(r.Context(), h.actorID(r), r.URL.Query().Get("tribe_id"), r.URL.Query().Get("status"))
	if err != nil {
		writeTribeHintsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h tribeHintsHandler) hintDetail(w http.ResponseWriter, r *http.Request) {
	detail, err := h.Service.Detail(r.Context(), h.actorID(r), r.PathValue("tribeID"), r.PathValue("sourceID"))
	if err != nil {
		writeTribeHintsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h tribeHintsHandler) decideHint(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action string `json:"action"`
		NoteAR string `json:"noteAr"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decision body must be JSON"})
		return
	}
	result, err := h.Service.Decide(r.Context(), h.actorID(r), r.PathValue("tribeID"), r.PathValue("sourceID"), input.Action, input.NoteAR)
	if err != nil {
		writeTribeHintsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h tribeHintsHandler) actorID(r *http.Request) string {
	if h.Auth == nil {
		return ""
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		return ""
	}
	return user.ID
}

func writeTribeHintsError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "tribe hints operation failed"
	switch err {
	case tribehints.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case tribehints.ErrNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case tribehints.ErrForbidden:
		status = http.StatusForbidden
		message = err.Error()
	case tribehints.ErrUnauthenticated:
		status = http.StatusUnauthorized
		message = err.Error()
	case tribehints.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "tribe hints database is unavailable"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
