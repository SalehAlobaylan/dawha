package httpapi

import (
	"errors"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geospatialintelligence"
)

type geospatialIntelligenceHandler struct {
	Service *geospatialintelligence.Service
	Auth    *auth.Service
}

func (h geospatialIntelligenceHandler) start(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input geospatialintelligence.RunInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.StartRun(r.Context(), user.ID, input)
	if err != nil {
		writeGeospatialIntelligenceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h geospatialIntelligenceHandler) getRun(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetRun(r.Context(), user.ID, r.PathValue("runID"))
	if err != nil {
		writeGeospatialIntelligenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h geospatialIntelligenceHandler) getLatestRun(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetLatestRun(r.Context(), user.ID, r.URL.Query().Get("question_id"), r.URL.Query().Get("entity_type"), r.URL.Query().Get("entity_id"), r.URL.Query().Get("tree_version_id"))
	if err != nil {
		writeGeospatialIntelligenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h geospatialIntelligenceHandler) listFindings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.ListFindings(r.Context(), user.ID, r.URL.Query().Get("run_id"), r.URL.Query().Get("status"))
	if err != nil {
		writeGeospatialIntelligenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": result})
}

func (h geospatialIntelligenceHandler) getFinding(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetFinding(r.Context(), user.ID, r.PathValue("findingID"))
	if err != nil {
		writeGeospatialIntelligenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h geospatialIntelligenceHandler) review(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input geospatialintelligence.ReviewInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReviewFinding(r.Context(), user.ID, r.PathValue("findingID"), input)
	if err != nil {
		writeGeospatialIntelligenceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h geospatialIntelligenceHandler) requireUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if h.Service == nil || h.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "خدمة التحليل الجغرافي غير متاحة حالياً"})
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

func writeGeospatialIntelligenceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "تعذر إكمال التحليل الجغرافي."
	switch {
	case errors.Is(err, geospatialintelligence.ErrValidation):
		status = http.StatusBadRequest
		message = "بيانات طلب التحليل الجغرافي غير صالحة."
	case errors.Is(err, geospatialintelligence.ErrForbidden):
		status = http.StatusForbidden
		message = "لا تملك صلاحية تنفيذ هذا الإجراء."
	case errors.Is(err, geospatialintelligence.ErrNotFound):
		status = http.StatusNotFound
		message = "لم يُعثر على المورد المطلوب."
	case errors.Is(err, geospatialintelligence.ErrConflict):
		status = http.StatusConflict
		message = "تغيرت حالة المورد أثناء المعالجة."
	case errors.Is(err, geospatialintelligence.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "خدمة التحليل غير متاحة حالياً."
	}
	writeJSON(w, status, map[string]string{"error": message})
}
