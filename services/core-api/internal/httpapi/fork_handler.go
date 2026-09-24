package httpapi

import (
	"net/http"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/trees"
)

func (h treeHandler) fork(w http.ResponseWriter, r *http.Request) {
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	var input trees.ForkTreeInput
	if !decodeRequest(w, r, &input) {
		return
	}
	detail, err := h.Service.ForkPublishedVersion(r.Context(), r.PathValue("treeID"), user.ID, input)
	if err != nil {
		writeTreeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, detail)
}

func (h treeHandler) diff(w http.ResponseWriter, r *http.Request) {
	fromTreeID := strings.TrimSpace(r.URL.Query().Get("from_tree_id"))
	fromVersionID := strings.TrimSpace(r.URL.Query().Get("from_version_id"))
	toVersionID := strings.TrimSpace(r.URL.Query().Get("to_version_id"))
	if fromVersionID == "" || toVersionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from_version_id and to_version_id are required"})
		return
	}
	result, err := h.Service.CompareVersions(r.Context(), r.PathValue("treeID"), fromTreeID, fromVersionID, toVersionID, h.optionalUserID(r))
	if err != nil {
		writeTreeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
