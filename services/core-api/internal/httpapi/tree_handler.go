package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/trees"
)

type treeHandler struct {
	Service *trees.Service
	Auth    *auth.Service
}

type publishTreeRequest struct {
	Note string `json:"note"`
}

func (h treeHandler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.ListPublicTrees(r.Context())
	if err != nil {
		if err == trees.ErrDatabaseUnavailable {
			writeJSON(w, http.StatusOK, map[string]any{
				"mode":  "demo",
				"items": []map[string]any{{"id": "tree-demo", "name": "شجرة بيت العنبر", "latestState": "published", "latestVersionNumber": 3}},
			})
			return
		}
		writeTreeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": "api", "items": items})
}

func (h treeHandler) get(w http.ResponseWriter, r *http.Request) {
	viewerID := h.optionalUserID(r)
	detail, err := h.Service.GetTree(r.Context(), r.PathValue("treeID"), viewerID)
	if err != nil {
		writeTreeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h treeHandler) versions(w http.ResponseWriter, r *http.Request) {
	viewerID := h.optionalUserID(r)
	items, err := h.Service.ListVersions(r.Context(), r.PathValue("treeID"), viewerID)
	if err != nil {
		writeTreeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h treeHandler) create(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input trees.CreateTreeInput
	if !decodeRequest(w, r, &input) {
		return
	}
	detail, err := h.Service.CreateTree(r.Context(), user.ID, input)
	if err != nil {
		writeTreeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

func (h treeHandler) publish(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var request publishTreeRequest
	if r.ContentLength != 0 {
		if !decodeRequest(w, r, &request) {
			return
		}
	}
	detail, err := h.Service.PublishLatestDraft(r.Context(), r.PathValue("treeID"), user.ID, request.Note)
	if err != nil {
		writeTreeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h treeHandler) optionalUserID(r *http.Request) string {
	if h.Auth == nil {
		return ""
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		return ""
	}
	return user.ID
}

func decodeRequest(w http.ResponseWriter, r *http.Request, destination any) bool {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil || strings.TrimSpace(r.Header.Get("Content-Type")) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "valid JSON is required"})
		return false
	}
	return true
}

func writeTreeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "tree operation failed"
	switch err {
	case trees.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case trees.ErrNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case trees.ErrForbidden:
		status = http.StatusForbidden
		message = err.Error()
	case trees.ErrNoDraft:
		status = http.StatusConflict
		message = err.Error()
	case trees.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "tree service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
