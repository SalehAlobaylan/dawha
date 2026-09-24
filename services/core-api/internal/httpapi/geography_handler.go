package httpapi

import (
	"net/http"
	"strconv"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geography"
)

type geographyHandler struct {
	Service *geography.Service
}

func (h geographyHandler) mapFeatures(w http.ResponseWriter, r *http.Request) {
	input, ok := mapInputFromRequest(w, r)
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
	input, ok := mapInputFromRequest(w, r)
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

func mapInputFromRequest(w http.ResponseWriter, r *http.Request) (geography.MapInput, bool) {
	input := geography.MapInput{Status: r.URL.Query().Get("status"), PlaceID: r.URL.Query().Get("place_id")}
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
