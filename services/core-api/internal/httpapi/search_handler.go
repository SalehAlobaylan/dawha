package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/search"
)

type searchHandler struct {
	Service *search.Service
	Auth    *auth.Service
}

// actorID resolves the optional session. Search stays reachable without a session;
// the visibility policy then answers as the anonymous actor.
func (h searchHandler) actorID(r *http.Request) string {
	if h.Auth == nil {
		return ""
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		return ""
	}
	return user.ID
}

func (h searchHandler) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	input := search.Input{
		ActorID:   h.actorID(r),
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
	switch {
	case errors.Is(err, search.ErrValidation):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, search.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "search service is not configured"
	case errors.Is(err, search.ErrAIUnavailable):
		status = http.StatusServiceUnavailable
		message = "خدمة تضمين النص غير متاحة حالياً."
	}
	writeJSON(w, status, map[string]string{"error": message})
}
