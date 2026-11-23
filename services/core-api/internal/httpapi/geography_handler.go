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

// tribeMovement reads one tribe's documented مسالِك (plan 033-A): the person
// movement's own contract over MOVED-LAYERED subjects, with the optional
// needs-review include kept away — a subject read is the accepted-only lane.
func (h geographyHandler) tribeMovement(w http.ResponseWriter, r *http.Request) {
	actorID := h.optionalUserID(r)
	report, err := h.Service.TribeMovement(r.Context(), r.PathValue("tribeID"), actorID)
	if err != nil {
		writeGeographyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// atlasRelations reads the era's recorded relationship arcs (plan 032-E):
// the claims lane's own anchors, the 200-edge discipline intact.
func (h geographyHandler) atlasRelations(w http.ResponseWriter, r *http.Request) {
	actorID := h.optionalUserID(r)
	fromYear, toYear := 0, 0
	if raw := r.URL.Query().Get("from"); raw != "" {
		if year, err := strconv.Atoi(raw); err == nil && year >= -10000 && year <= 10000 {
			fromYear = year
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from must be an integer year"})
			return
		}
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		if year, err := strconv.Atoi(raw); err == nil && year >= -10000 && year <= 10000 {
			toYear = year
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to must be an integer year"})
			return
		}
	}
	report, err := h.Service.AtlasRelations(r.Context(), actorID, fromYear, toYear)
	if err != nil {
		writeGeographyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// subjectCorridors reads one subject's recorded routes (plan 033-B): the
// ordered stops' own geometry for the atlas's corridor layer.
func (h geographyHandler) subjectCorridors(w http.ResponseWriter, r *http.Request) {
	actorID := h.optionalUserID(r)
	report, err := h.Service.SubjectCorridors(r.Context(), r.PathValue("subjectType"), r.PathValue("subjectID"), actorID, r.URL.Query().Get("season"), r.URL.Query().Get("mode"))
	if err != nil {
		writeGeographyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// plateRegister is the atlas's plate layer source data (plan 032-A): the
// plates and their validated transforms, or the honest disabled caption.
func (h geographyHandler) plateRegister(w http.ResponseWriter, r *http.Request) {
	actorID := h.optionalUserID(r)
	report, err := h.Service.PlateRegister(r.Context(), actorID)
	if err != nil {
		writeGeographyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, report)
}

func (h geographyHandler) placeEvidence(w http.ResponseWriter, r *http.Request) {
	actorID := h.optionalUserID(r)
	includeNeedsReview := r.URL.Query().Get("include") == "needs_review"
	report, err := h.Service.PlaceEvidence(r.Context(), r.PathValue("placeID"), actorID, includeNeedsReview)
	if err != nil {
		writeGeographyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// tribeEvidence is the tribal dossier's receipt folio (plan 031-A), the
// place-evidence contract read over the tribe's own associations and claims.
func (h geographyHandler) tribeEvidence(w http.ResponseWriter, r *http.Request) {
	actorID := h.optionalUserID(r)
	includeNeedsReview := r.URL.Query().Get("include") == "needs_review"
	report, err := h.Service.TribeEvidence(r.Context(), r.PathValue("tribeID"), actorID, includeNeedsReview)
	if err != nil {
		writeGeographyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h geographyHandler) personMovement(w http.ResponseWriter, r *http.Request) {
	actorID := h.optionalUserID(r)
	report, err := h.Service.PersonMovement(r.Context(), r.PathValue("personID"), actorID)
	if err != nil {
		writeGeographyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
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
	case geography.ErrPlaceEvidenceForbidden:
		status = http.StatusForbidden
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

func (h geographyHandler) sourceOccurrences(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	input := geography.OccurrenceInput{
		Query: query.Get("q"), ID: r.PathValue("occurrenceID"), TribeID: query.Get("tribe"),
		After: query.Get("after"), IncludeUndated: query.Get("undated") == "true",
	}
	for _, window := range []struct {
		key string
		dst **int
	}{{"from", &input.From}, {"to", &input.To}} {
		if raw := query.Get(window.key); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < -9999 || value > 9999 {
				writeGeographyError(w, geography.ErrValidation)
				return
			}
			*window.dst = &value
		}
	}
	result, err := h.Service.SourceOccurrences(r.Context(), h.optionalUserID(r), input)
	if err != nil {
		writeGeographyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h geographyHandler) atlasSourceRelationship(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	result, err := h.Service.AtlasSourceRelationship(r.Context(), h.optionalUserID(r), query.Get("first"), query.Get("second"))
	if err != nil {
		writeGeographyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}
