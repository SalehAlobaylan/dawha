package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/pairing"
)

type pairingHandler struct {
	Service *pairing.Service
	Auth    *auth.Service
}

func (h pairingHandler) listCandidates(w http.ResponseWriter, r *http.Request) {
	actorID := h.actorID(r)
	report, err := h.Service.ListWithParams(r.Context(), actorID, pairing.ListParams{
		Status:  r.URL.Query().Get("status"),
		Query:   r.URL.Query().Get("q"),
		PlaceID: r.URL.Query().Get("place_id"),
	})
	if err != nil {
		writePairingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h pairingHandler) decideCandidate(w http.ResponseWriter, r *http.Request) {
	actorID := h.actorID(r)
	var input struct {
		Effect string `json:"effect"`
		Note   string `json:"noteAr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decision body must be JSON"})
		return
	}
	report, err := h.Service.Decide(r.Context(), actorID, r.PathValue("candidateID"), input.Effect, input.Note)
	if err != nil {
		writePairingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h pairingHandler) reopenCandidate(w http.ResponseWriter, r *http.Request) {
	actorID := h.actorID(r)
	var input struct {
		Note string `json:"noteAr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decision body must be JSON"})
		return
	}
	report, err := h.Service.Reopen(r.Context(), actorID, r.PathValue("candidateID"), input.Note)
	if err != nil {
		writePairingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h pairingHandler) history(w http.ResponseWriter, r *http.Request) {
	actorID := h.actorID(r)
	report, err := h.Service.History(r.Context(), actorID, r.PathValue("candidateID"))
	if err != nil {
		writePairingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h pairingHandler) externalIDs(w http.ResponseWriter, r *http.Request) {
	actorID := h.actorID(r)
	report, err := h.Service.ExternalIDs(r.Context(), r.PathValue("placeID"), actorID)
	if err != nil {
		writePairingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h pairingHandler) actorID(r *http.Request) string {
	if h.Auth == nil {
		return ""
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		return ""
	}
	return user.ID
}

func writePairingError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "pairing operation failed"
	switch err {
	case pairing.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case pairing.ErrForbidden:
		status = http.StatusForbidden
		message = err.Error()
	case pairing.ErrUnauthenticated:
		status = http.StatusUnauthorized
		message = err.Error()
	case pairing.ErrCannotReopen:
		status = http.StatusConflict
		message = err.Error()
	case pairing.ErrNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case pairing.ErrAlreadyDecided:
		status = http.StatusConflict
		message = err.Error()
	case pairing.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "pairing service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
