package httpapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/sourceprocessing"
)

type sourceProcessingHandler struct {
	Service *sourceprocessing.Service
	Auth    *auth.Service
}

func (h sourceProcessingHandler) uploadFile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, sourceprocessing.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		writeSourceProcessingError(w, sourceprocessing.ErrValidation)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeSourceProcessingError(w, sourceprocessing.ErrValidation)
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, sourceprocessing.MaxUploadBytes+1))
	if err != nil {
		writeSourceProcessingError(w, sourceprocessing.ErrValidation)
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	result, err := h.Service.Upload(r.Context(), r.PathValue("sourceID"), user.ID, sourceprocessing.UploadInput{
		Filename:    header.Filename,
		ContentType: contentType,
		Content:     content,
	})
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h sourceProcessingHandler) getProcessing(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetProcessing(r.Context(), r.PathValue("sourceID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) reviewCandidate(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.ReviewInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReviewCandidate(r.Context(), r.PathValue("candidateID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) requireUser(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	if h.Service == nil || h.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "source processing service is not configured"})
		return auth.User{}, false
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	return user, true
}

func writeSourceProcessingError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "source processing operation failed"
	switch {
	case errors.Is(err, sourceprocessing.ErrValidation):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, sourceprocessing.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, sourceprocessing.ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, sourceprocessing.ErrConflict):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, sourceprocessing.ErrQueueUnavailable), errors.Is(err, sourceprocessing.ErrStorageUnavailable), errors.Is(err, sourceprocessing.ErrAIUnavailable), errors.Is(err, sourceprocessing.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "source processing service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
