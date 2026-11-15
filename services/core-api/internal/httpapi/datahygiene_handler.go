package httpapi

import (
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/datahygiene"
)

// dataHygieneHandler answers the license dashboard. The route itself carries no
// gate beyond the session: the moderator check lives inside the service, the
// same single-query pattern every other role-scoped read uses, so an anonymous
// caller and a signed-in reader both get the one refusal the role check owns.
type dataHygieneHandler struct {
	Service *datahygiene.Service
	Auth    *auth.Service
}

func (h dataHygieneHandler) report(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	report, err := h.Service.Report(r.Context(), user.ID)
	if err != nil {
		writeDataHygieneError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h dataHygieneHandler) actor(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	return user, true
}

func writeDataHygieneError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "data hygiene operation failed"
	switch err {
	case datahygiene.ErrForbidden:
		status = http.StatusForbidden
		message = err.Error()
	case datahygiene.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "data hygiene service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
