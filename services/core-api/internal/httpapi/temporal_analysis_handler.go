package httpapi

import (
	"errors"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/temporalanalysis"
)

type temporalAnalysisHandler struct {
	Service *temporalanalysis.Service
	Auth    *auth.Service
}

func (h temporalAnalysisHandler) start(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input temporalanalysis.StartRunInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.StartRun(r.Context(), user.ID, input)
	if err != nil {
		writeTemporalAnalysisError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h temporalAnalysisHandler) getRun(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetRun(r.Context(), user.ID, r.PathValue("runID"))
	if err != nil {
		writeTemporalAnalysisError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h temporalAnalysisHandler) getLatestRun(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetLatestRun(r.Context(), user.ID, r.URL.Query().Get("question_id"), r.URL.Query().Get("tree_id"), r.URL.Query().Get("tree_version_id"), r.URL.Query().Get("target_person_id"))
	if err != nil {
		writeTemporalAnalysisError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h temporalAnalysisHandler) listFindings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.ListFindings(r.Context(), user.ID, r.URL.Query().Get("run_id"), r.URL.Query().Get("status"))
	if err != nil {
		writeTemporalAnalysisError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h temporalAnalysisHandler) getFinding(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetFinding(r.Context(), user.ID, r.PathValue("findingID"))
	if err != nil {
		writeTemporalAnalysisError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h temporalAnalysisHandler) review(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input temporalanalysis.ReviewInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReviewFinding(r.Context(), user.ID, r.PathValue("findingID"), input)
	if err != nil {
		writeTemporalAnalysisError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h temporalAnalysisHandler) requireUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if h.Service == nil || h.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "temporal analysis service is not configured"})
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

func writeTemporalAnalysisError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "تعذر إكمال التحليل الزمني."
	switch {
	case errors.Is(err, temporalanalysis.ErrValidation):
		status = http.StatusBadRequest
		message = "بيانات طلب التحليل الزمني غير صالحة."
	case errors.Is(err, temporalanalysis.ErrForbidden):
		status = http.StatusForbidden
		message = "لا تملك صلاحية تنفيذ هذا الإجراء."
	case errors.Is(err, temporalanalysis.ErrNotFound):
		status = http.StatusNotFound
		message = "لم يُعثر على المورد المطلوب."
	case errors.Is(err, temporalanalysis.ErrConflict):
		status = http.StatusConflict
		message = "تغيرت حالة المورد أثناء المعالجة."
	case errors.Is(err, temporalanalysis.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "خدمة التحليل غير متاحة حالياً."
	}
	writeJSON(w, status, map[string]string{"error": message})
}
