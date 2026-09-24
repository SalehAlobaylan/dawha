package httpapi

import (
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/collaboration"
)

type collaborationHandler struct {
	Service *collaboration.Service
	Auth    *auth.Service
}

func (h collaborationHandler) listCollaborators(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.ListCollaborators(r.Context(), r.PathValue("treeID"), user.ID)
	if err != nil {
		writeCollaborationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h collaborationHandler) createInvitation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input collaboration.InviteInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.CreateInvitation(r.Context(), r.PathValue("treeID"), user.ID, input)
	if err != nil {
		writeCollaborationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h collaborationHandler) updateCollaborator(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input collaboration.UpdatePermissionInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.UpdateCollaborator(r.Context(), r.PathValue("treeID"), r.PathValue("userID"), user.ID, input)
	if err != nil {
		writeCollaborationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h collaborationHandler) removeCollaborator(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	if err := h.Service.RemoveCollaborator(r.Context(), r.PathValue("treeID"), r.PathValue("userID"), user.ID); err != nil {
		writeCollaborationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h collaborationHandler) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	if err := h.Service.RevokeInvitation(r.Context(), r.PathValue("treeID"), r.PathValue("invitationID"), user.ID); err != nil {
		writeCollaborationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h collaborationHandler) listInvitations(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	items, err := h.Service.ListInvitations(r.Context(), user.ID)
	if err != nil {
		writeCollaborationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h collaborationHandler) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.AcceptInvitation(r.Context(), r.PathValue("token"), user.ID)
	if err != nil {
		writeCollaborationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h collaborationHandler) listActivity(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	items, err := h.Service.ListActivity(r.Context(), r.PathValue("treeID"), user.ID)
	if err != nil {
		writeCollaborationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h collaborationHandler) requireUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if h.Service == nil || h.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collaboration service is not configured"})
		return auth.User{}, false
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	return user, true
}

func writeCollaborationError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "collaboration operation failed"
	switch err {
	case collaboration.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case collaboration.ErrNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case collaboration.ErrForbidden:
		status = http.StatusForbidden
		message = err.Error()
	case collaboration.ErrConflict:
		status = http.StatusConflict
		message = err.Error()
	case collaboration.ErrInvitationExpired:
		status = http.StatusGone
		message = err.Error()
	case collaboration.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "collaboration service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
