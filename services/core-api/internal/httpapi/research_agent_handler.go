package httpapi

import (
	"errors"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/researchagent"
)

type researchAgentHandler struct {
	Service *researchagent.Service
	Auth    *auth.Service
}

func (h researchAgentHandler) start(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input researchagent.RunInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.StartRun(r.Context(), user.ID, input)
	if err != nil {
		writeResearchAgentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h researchAgentHandler) getRun(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetRun(r.Context(), user.ID, r.PathValue("runID"))
	if err != nil {
		writeResearchAgentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h researchAgentHandler) getLatestRun(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetLatestRun(r.Context(), user.ID, r.URL.Query().Get("question_id"), r.URL.Query().Get("entity_type"), r.URL.Query().Get("entity_id"))
	if err != nil {
		writeResearchAgentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h researchAgentHandler) generateQuestionCandidates(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GenerateQuestionCandidates(r.Context(), user.ID, r.PathValue("runID"))
	if err != nil {
		writeResearchAgentError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"items": result})
}

func (h researchAgentHandler) listQuestionCandidates(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.ListQuestionCandidates(r.Context(), user.ID, r.PathValue("runID"), r.URL.Query().Get("status"))
	if err != nil {
		writeResearchAgentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h researchAgentHandler) reviewQuestionCandidate(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input researchagent.ReviewQuestionCandidateInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReviewQuestionCandidate(r.Context(), user.ID, r.PathValue("candidateID"), input)
	if err != nil {
		writeResearchAgentError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h researchAgentHandler) requireUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if h.Service == nil || h.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "خدمة وكيل البحث غير متاحة حالياً"})
		return auth.User{}, false
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		if errors.Is(err, auth.ErrDatabaseUnavailable) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "خدمة المصادقة غير متاحة حالياً"})
			return auth.User{}, false
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "يلزم تسجيل الدخول"})
		return auth.User{}, false
	}
	return user, true
}

func writeResearchAgentError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "تعذر إكمال تحقيق وكيل البحث."
	switch {
	case errors.Is(err, researchagent.ErrValidation):
		status = http.StatusBadRequest
		message = "بيانات طلب وكيل البحث غير صالحة."
	case errors.Is(err, researchagent.ErrForbidden):
		status = http.StatusForbidden
		message = "لا تملك صلاحية تنفيذ هذا الإجراء."
	case errors.Is(err, researchagent.ErrNotFound):
		status = http.StatusNotFound
		message = "لم يُعثر على المورد المطلوب."
	case errors.Is(err, researchagent.ErrConflict):
		status = http.StatusConflict
		message = "تغيرت حالة مورد وكيل البحث."
	case errors.Is(err, researchagent.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "خدمة وكيل البحث غير متاحة حالياً."
	}
	writeJSON(w, status, map[string]string{"error": message})
}
