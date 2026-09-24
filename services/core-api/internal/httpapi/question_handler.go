package httpapi

import (
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/questions"
)

type questionHandler struct {
	Service *questions.Service
	Auth    *auth.Service
}

func (h questionHandler) listQuestions(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.ListQuestions(r.Context())
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h questionHandler) getQuestion(w http.ResponseWriter, r *http.Request) {
	result, err := h.Service.GetQuestion(r.Context(), r.PathValue("questionID"))
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h questionHandler) createQuestion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input questions.CreateQuestionInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.CreateQuestion(r.Context(), user.ID, input)
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h questionHandler) updateQuestion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input questions.UpdateQuestionInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.UpdateQuestion(r.Context(), r.PathValue("questionID"), user.ID, input)
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h questionHandler) addNote(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input questions.QuestionNoteInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.AddNote(r.Context(), r.PathValue("questionID"), user.ID, input)
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h questionHandler) linkClaim(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input questions.QuestionClaimInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.LinkClaim(r.Context(), r.PathValue("questionID"), user.ID, input)
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h questionHandler) linkSource(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input questions.QuestionSourceInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.LinkSource(r.Context(), r.PathValue("questionID"), user.ID, input)
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h questionHandler) linkDispute(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input questions.QuestionDisputeInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.LinkDispute(r.Context(), r.PathValue("questionID"), user.ID, input)
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h questionHandler) listDisputes(w http.ResponseWriter, r *http.Request) {
	items, err := h.Service.ListDisputes(r.Context())
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h questionHandler) getDispute(w http.ResponseWriter, r *http.Request) {
	result, err := h.Service.GetDispute(r.Context(), r.PathValue("disputeID"))
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h questionHandler) createDispute(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input questions.CreateDisputeInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.CreateDispute(r.Context(), user.ID, input)
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h questionHandler) updateDispute(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input questions.UpdateDisputeInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.UpdateDispute(r.Context(), r.PathValue("disputeID"), user.ID, input)
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h questionHandler) linkDisputeClaim(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input questions.DisputeClaimInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.LinkDisputeClaim(r.Context(), r.PathValue("disputeID"), user.ID, input)
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h questionHandler) requireUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if h.Service == nil || h.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "questions service is not configured"})
		return auth.User{}, false
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	return user, true
}

func writeQuestionError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "question operation failed"
	switch err {
	case questions.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case questions.ErrNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case questions.ErrForbidden:
		status = http.StatusForbidden
		message = err.Error()
	case questions.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "questions service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
