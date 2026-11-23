package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/tribepairings"
)

// tribePairingsHandler is مِسلك التكرار's transport (plan 031-E): a bounded
// queue of duplicate candidates, the three effects' single decide endpoint,
// and the append-only history. The service keeps the lane's own law: no
// statement writes a tribes row, and the undo is data, not surgery.
type tribePairingsHandler struct {
	Service *tribepairings.Service
	Auth    *auth.Service
}

func (h tribePairingsHandler) listCandidates(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.List(r.Context(), h.actorID(r), r.URL.Query().Get("status"))
	if err != nil {
		writeTribePairingsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h tribePairingsHandler) decideCandidate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action    string `json:"action"`
		Canonical string `json:"canonical"`
		NoteAR    string `json:"noteAr"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decision body must be JSON"})
		return
	}
	report, err := h.Service.Decide(r.Context(), h.actorID(r), r.PathValue("candidateID"), input.Action, input.Canonical, input.NoteAR)
	if err != nil {
		writeTribePairingsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h tribePairingsHandler) history(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.History(r.Context(), h.actorID(r), r.PathValue("candidateID"))
	if err != nil {
		writeTribePairingsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h tribePairingsHandler) actorID(r *http.Request) string {
	if h.Auth == nil {
		return ""
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		return ""
	}
	return user.ID
}

func writeTribePairingsError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "tribe pairings operation failed"
	switch err {
	case tribepairings.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case tribepairings.ErrUnauthenticated:
		status = http.StatusUnauthorized
		message = err.Error()
	case tribepairings.ErrForbidden:
		status = http.StatusForbidden
		message = err.Error()
	case tribepairings.ErrNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case tribepairings.ErrAlreadyDecided:
		status = http.StatusConflict
		message = err.Error()
	case tribepairings.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "tribe pairings database is unavailable"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
