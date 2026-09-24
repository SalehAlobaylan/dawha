package httpapi

import (
	"net/http"
	"strconv"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/search"
)

type searchHandler struct {
	Service *search.Service
}

func (h searchHandler) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	input := search.Input{
		Query:     query.Get("q"),
		Kind:      query.Get("kind"),
		Status:    query.Get("status"),
		PersonID:  query.Get("person_id"),
		PlaceID:   query.Get("place_id"),
		SourceID:  query.Get("source_id"),
		EntityID:  query.Get("entity_id"),
		Embedding: query.Get("embedding"),
	}
	var err error
	if value := query.Get("from_year"); value != "" {
		input.FromYear, err = strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from_year must be a number"})
			return
		}
	}
	if value := query.Get("to_year"); value != "" {
		input.ToYear, err = strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to_year must be a number"})
			return
		}
	}
	if value := query.Get("limit"); value != "" {
		input.Limit, err = strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a number"})
			return
		}
	}
	result, err := h.Service.Search(r.Context(), input)
	if err != nil {
		writeSearchError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeSearchError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "search operation failed"
	switch err {
	case search.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case search.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "search service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
