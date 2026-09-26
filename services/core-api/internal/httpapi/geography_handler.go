package httpapi

import (
	"net/http"
	"net/url"
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

// mapInput reads the map's filters from the query string. An absent parameter means
// "no filter", which is why the viewport is four parameters rather than one: a
// half-sent box would be a rectangle whose meaning depends on which half arrived.
func (h geographyHandler) mapInput(w http.ResponseWriter, r *http.Request) (geography.MapInput, bool) {
	input := geography.MapInput{ActorID: h.optionalUserID(r), Status: r.URL.Query().Get("status"), PlaceID: r.URL.Query().Get("place_id")}
	query := r.URL.Query()
	var err error
	if value := query.Get("from_year"); value != "" {
		input.FromYear, err = strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from_year must be a number"})
			return geography.MapInput{}, false
		}
	}
	if value := query.Get("to_year"); value != "" {
		input.ToYear, err = strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to_year must be a number"})
			return geography.MapInput{}, false
		}
	}
	if value := query.Get("limit"); value != "" {
		input.Limit, err = strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be a number"})
			return geography.MapInput{}, false
		}
	}
	viewport, ok := mapViewport(w, query)
	if !ok {
		return geography.MapInput{}, false
	}
	input.Viewport = viewport
	return input, true
}

// mapViewport reads the bounding box. Any one of the four edges is enough to ask
// for a viewport, and all four are then required: a box missing an edge is refused
// rather than completed with a guess, because a guessed edge moves the map's horizon.
func mapViewport(w http.ResponseWriter, query url.Values) (*geography.Viewport, bool) {
	viewport := &geography.Viewport{}
	targets := []struct {
		name  string
		value *float64
	}{
		{"min_longitude", &viewport.MinLongitude},
		{"min_latitude", &viewport.MinLatitude},
		{"max_longitude", &viewport.MaxLongitude},
		{"max_latitude", &viewport.MaxLatitude},
	}
	present := 0
	for _, target := range targets {
		raw := query.Get(target.name)
		if raw == "" {
			continue
		}
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": target.name + " must be a number"})
			return nil, false
		}
		*target.value = parsed
		present++
	}
	if present == 0 {
		return nil, true
	}
	if present != len(targets) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a viewport needs min_longitude, min_latitude, max_longitude and max_latitude together"})
		return nil, false
	}
	return viewport, true
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
