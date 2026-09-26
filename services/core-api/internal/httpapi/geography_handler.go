package httpapi

import (
	"net/http"
	"strconv"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geography"
)

// geographyHandler renders the map. The map stays a public read path: an anonymous
// caller sees only published places. The session is read anyway, because a research
// place is on the map of the role that wrote it and on nobody else's, and the
// endpoint must not be a second, unfiltered reader of a row the dictionary hides.
type geographyHandler struct {
	Service *geography.Service
	Auth    *auth.Service
}

func (h geographyHandler) mapFeatures(w http.ResponseWriter, r *http.Request) {
	input, ok := h.mapInput(w, r)
	if !ok {
		return
	}
	result, err := h.Service.List(r.Context(), input)
	if err != nil {
		writeGeographyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h geographyHandler) placeMap(w http.ResponseWriter, r *http.Request) {
	input, ok := h.mapInput(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetPlace(r.Context(), r.PathValue("placeID"), input)
	if err != nil {
		writeGeographyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h geographyHandler) mapInput(w http.ResponseWriter, r *http.Request) (geography.MapInput, bool) {
	input := geography.MapInput{ActorID: h.optionalUserID(r), Status: r.URL.Query().Get("status"), PlaceID: r.URL.Query().Get("place_id")}
	var err error
	if value := r.URL.Query().Get("from_year"); value != "" {
		input.FromYear, err = strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from_year must be a number"})
			return geography.MapInput{}, false
		}
	}
	if value := r.URL.Query().Get("to_year"); value != "" {
		input.ToYear, err = strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to_year must be a number"})
			return geography.MapInput{}, false
		}
	}
	return input, true
}

// optionalUserID resolves the session when there is one. A caller without a session
// is the anonymous reader, which is the map's default state.
func (h geographyHandler) optionalUserID(r *http.Request) string {
	if h.Auth == nil {
		return ""
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		return ""
	}
	return user.ID
}

func writeGeographyError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "geography operation failed"
	switch err {
	case geography.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case geography.ErrNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case geography.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "geography service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
