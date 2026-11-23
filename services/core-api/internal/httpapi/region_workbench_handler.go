package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/regionworkbench"
)

// regionWorkbenchHandler is the transport for plan 030-F's tile lane: a
// bounded queue of plate squares, claim/release/submit over them, and the
// history of what happened. The service owns the rules; this file owns the
// HTTP.
type regionWorkbenchHandler struct {
	Service *regionworkbench.Service
	Auth    *auth.Service
}

func (h regionWorkbenchHandler) listTiles(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.List(r.Context(), h.actorID(r), regionworkbench.ListParams{
		Status:   r.URL.Query().Get("status"),
		PlateKey: r.URL.Query().Get("plate_key"),
	})
	if err != nil {
		writeRegionWorkbenchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h regionWorkbenchHandler) getTile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store")
	report, err := h.Service.Get(r.Context(), h.actorID(r), r.PathValue("taskID"))
	if err != nil {
		writeRegionWorkbenchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h regionWorkbenchHandler) claimTile(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.Claim(r.Context(), h.actorID(r), r.PathValue("taskID"))
	if err != nil {
		writeRegionWorkbenchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h regionWorkbenchHandler) releaseTile(w http.ResponseWriter, r *http.Request) {
	var input struct {
		NoteAR string `json:"noteAr"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON"})
		return
	}
	report, err := h.Service.Release(r.Context(), h.actorID(r), r.PathValue("taskID"), input.NoteAR)
	if err != nil {
		writeRegionWorkbenchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h regionWorkbenchHandler) submitTile(w http.ResponseWriter, r *http.Request) {
	var input struct {
		NoteAR string `json:"noteAr"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON"})
		return
	}
	report, err := h.Service.Submit(r.Context(), h.actorID(r), r.PathValue("taskID"), input.NoteAR)
	if err != nil {
		writeRegionWorkbenchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h regionWorkbenchHandler) tileHistory(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.History(r.Context(), h.actorID(r), r.PathValue("taskID"))
	if err != nil {
		writeRegionWorkbenchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h regionWorkbenchHandler) actorID(r *http.Request) string {
	if h.Auth == nil {
		return ""
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		return ""
	}
	return user.ID
}

func writeRegionWorkbenchError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "region workbench operation failed"
	var incomplete regionworkbench.IncompletePassError
	switch {
	case errors.Is(err, regionworkbench.ErrValidation):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.As(err, &incomplete):
		// An incomplete pass names the work: the untouched candidate ids
		// travel in the body so the tile can mark them.
		status = http.StatusBadRequest
		message = err.Error()
		writeJSON(w, status, map[string]any{"error": message, "missingCandidates": incomplete.Missing})
		return
	case errors.Is(err, regionworkbench.ErrUnauthenticated):
		status = http.StatusUnauthorized
		message = err.Error()
	case errors.Is(err, regionworkbench.ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, regionworkbench.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, regionworkbench.ErrSelfReview):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, regionworkbench.ErrConflict):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, regionworkbench.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "region workbench database is unavailable"
	}
	writeJSON(w, status, map[string]string{"error": message})
}

// summary serves the workbench header's own counts: how many tile tasks sit
// in each lane today. Small, bounded, no per-lane scans.
func (h regionWorkbenchHandler) summary(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.Summary(r.Context(), h.actorID(r))
	if err != nil {
		writeRegionWorkbenchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
