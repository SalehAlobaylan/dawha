package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/research"
)

type researchHandler struct {
	Service *research.Service
	Auth    *auth.Service
	Logger  *slog.Logger
}

func (h researchHandler) query(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "research service is not configured"})
		return
	}
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	var input research.QueryInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.Query(r.Context(), input, actorID)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("research query failed", "error", err)
		}
		writeResearchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h researchHandler) relationshipImpact(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "research service is not configured"})
		return
	}
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	var input research.GraphRelationshipImpactInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.GraphRelationshipImpact(r.Context(), input, actorID)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("relationship impact failed", "error", err)
		}
		writeResearchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h researchHandler) branchStructureComparison(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "research service is not configured"})
		return
	}
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	var input research.GraphBranchStructureComparisonInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.GraphBranchStructureComparison(r.Context(), input, actorID)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("branch structure comparison failed", "error", err)
		}
		writeResearchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h researchHandler) ancestorFrontier(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "research service is not configured"})
		return
	}
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	var input research.GraphAncestorFrontierInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.GraphAncestorFrontier(r.Context(), input, actorID)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("ancestor frontier failed", "error", err)
		}
		writeResearchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h researchHandler) sourceDependencyNeighborhood(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "research service is not configured"})
		return
	}
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	var input research.GraphSourceDependencyNeighborhoodInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.GraphSourceDependencyNeighborhood(r.Context(), input, actorID)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("source dependency neighborhood failed", "error", err)
		}
		writeResearchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h researchHandler) sourceDependencyCommunities(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "research service is not configured"})
		return
	}
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	var input research.GraphSourceDependencyCommunitiesInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.GraphSourceDependencyCommunities(r.Context(), input, actorID)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("source dependency communities failed", "error", err)
		}
		writeResearchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h researchHandler) workspace(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "research service is not configured"})
		return
	}
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	result, err := h.Service.Workspace(r.Context(), research.WorkspaceInput{
		QuestionID:    r.PathValue("questionID"),
		EntityType:    r.URL.Query().Get("entity_type"),
		EntityID:      r.URL.Query().Get("entity_id"),
		TreeID:        r.URL.Query().Get("tree_id"),
		TreeVersionID: r.URL.Query().Get("tree_version_id"),
	}, actorID)
	if err != nil {
		if h.Logger != nil {
			h.Logger.Error("research workspace failed", "error", err)
		}
		writeResearchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// listRuns and getRun share one public contract: research run metadata is
// research-only data. A caller without a research role is refused with 403 before
// any run row is read, so the response is the same whether or not the run exists and
// no query text, status, model, route, count or timestamp is disclosed. The optional
// session lookup stays optional on purpose: an unauthenticated caller must receive
// the neutral 403 rather than a 401 that would confirm the endpoint is live.
func (h researchHandler) listRuns(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "research service is not configured"})
		return
	}
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	items, err := h.Service.ListRuns(r.Context(), actorID, r.PathValue("questionID"))
	if err != nil {
		writeResearchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h researchHandler) getRun(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "research service is not configured"})
		return
	}
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	result, err := h.Service.GetRun(r.Context(), actorID, r.PathValue("runID"))
	if err != nil {
		writeResearchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeResearchError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "research operation failed"
	switch {
	case errors.Is(err, research.ErrValidation):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, research.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, research.ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, research.ErrGraphUnavailable):
		status = http.StatusServiceUnavailable
		message = "research graph retrieval is unavailable"
	case errors.Is(err, research.ErrAIUnavailable), errors.Is(err, research.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "research service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
