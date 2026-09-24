package httpapi

import (
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/dictionary"
)

type dictionaryHandler struct {
	Service *dictionary.Service
}

func (h dictionaryHandler) index(w http.ResponseWriter, r *http.Request) {
	result, err := h.Service.ListIndex(r.Context(), r.URL.Query().Get("kind"), r.URL.Query().Get("q"))
	if err != nil {
		writeDictionaryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h dictionaryHandler) detail(w http.ResponseWriter, r *http.Request) {
	result, err := h.Service.Get(r.Context(), r.PathValue("kind"), r.PathValue("id"))
	if err != nil {
		writeDictionaryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeDictionaryError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "dictionary operation failed"
	switch err {
	case dictionary.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case dictionary.ErrNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case dictionary.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "dictionary service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
