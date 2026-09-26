package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	result, err := h.Service.Upload(r.Context(), r.PathValue("sourceID"), user.ID, sourceprocessing.UploadInput{
		Filename:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
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
	page, ok := candidatePage(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetProcessingPage(r.Context(), r.PathValue("sourceID"), user.ID, page)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// candidatePage reads the optional candidate page from the query string. No
// parameters is the whole list, so a caller that never heard of pagination sees
// exactly what it saw before.
func candidatePage(w http.ResponseWriter, r *http.Request) (sourceprocessing.CandidatePage, bool) {
	page := sourceprocessing.CandidatePage{}
	query := r.URL.Query()
	for name, target := range map[string]*int{"limit": &page.Limit, "offset": &page.Offset} {
		value := query.Get(name)
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": name + " must be a number"})
			return sourceprocessing.CandidatePage{}, false
		}
		*target = parsed
	}
	return page, true
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

// downloadFile mints a signed, expiring link to an uploaded file's bytes.
//
// The response is JSON containing a URL, not a redirect and not a key. A
// redirect would be one fewer round trip, but it would also make the signed URL
// land in the browser's address bar and in its history, and a JSON body lets the
// web app treat the link as data it must not persist. Either way the decision to
// mint was made here, after an authorization check the caller cannot see, and the
// raw storage key appears in no response this service produces.
func (h sourceProcessingHandler) downloadFile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	requested, err := requestedExpiry(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	link, err := h.Service.SignedDownload(r.Context(), r.PathValue("fileID"), user.ID, requested)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	// The URL is a capability, so it must not be cached by anything between here
	// and the caller.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	writeJSON(w, http.StatusOK, link)
}

// serveSignedObject is the local adapter's object route: the other half of the
// signed link the handler above mints.
//
// It is the one route in the service with no session check, and deliberately so -
// that is what makes the link a capability rather than a session-dependent URL.
// The authorization happened when the link was issued; here the signature and its
// expiry are the whole of the request's authority.
func (h sourceProcessingHandler) serveSignedObject(w http.ResponseWriter, r *http.Request) {
	if h.Service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "object storage is not configured"})
		return
	}
	reader, object, err := h.Service.OpenSignedObject(
		r.Context(),
		r.PathValue("key"),
		r.URL.Query().Get("expires"),
		r.URL.Query().Get("signature"),
	)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	defer reader.Close()
	// The content type is sniffed from the bytes rather than taken from the
	// database: the signature binds the key, not the row, and a row that can be
	// edited after the fact must not be able to change what an issued link
	// serves. The upload boundary already checked the declared type against the
	// content, so the two agree.
	buffered := make([]byte, 512)
	read, _ := io.ReadFull(reader, buffered)
	buffered = buffered[:read]
	contentType := http.DetectContentType(buffered)
	if object.ContentType != "" && strings.HasPrefix(object.ContentType, "text/") {
		// Sniffing a short text file says text/plain; the declared text subtype is
		// the more specific truth and costs nothing to prefer.
		contentType = object.ContentType
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.FormatInt(object.Size, 10))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buffered)
	_, _ = io.Copy(w, reader)
}

// requestedExpiry reads the optional ?expires= parameter, in seconds. It is a
// bound the client may lower, never raise: storage.NormalizeExpiry owns the
// ceiling, and a client asking for longer is refused rather than served.
func requestedExpiry(r *http.Request) (time.Duration, error) {
	raw := r.URL.Query().Get("expires")
	if raw == "" {
		return 0, nil
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0, errors.New("expires must be a positive number of seconds")
	}
	return time.Duration(seconds) * time.Second, nil
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

// unsupportedSourceFormatResponse is the 415 body: a sentence for the person
// reading it and the accepted matrix for a client that wants to self-correct.
type unsupportedSourceFormatResponse struct {
	Error                 string   `json:"error"`
	Code                  string   `json:"code"`
	SupportedContentTypes []string `json:"supportedContentTypes"`
}

func writeSourceProcessingError(w http.ResponseWriter, err error) {
	var unsupported *sourceprocessing.UnsupportedContentError
	if errors.As(err, &unsupported) || errors.Is(err, sourceprocessing.ErrUnsupportedContent) || errors.Is(err, sourceprocessing.ErrUnsupportedDocument) {
		writeJSON(w, http.StatusUnsupportedMediaType, unsupportedSourceFormatResponse{
			Error:                 err.Error(),
			Code:                  "unsupported_source_format",
			SupportedContentTypes: sourceprocessing.SupportedContentTypes(),
		})
		return
	}
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
	case errors.Is(err, sourceprocessing.ErrLinkNotAuthorized):
		status = http.StatusForbidden
		message = "the signed link is not valid"
	case errors.Is(err, sourceprocessing.ErrLinkExpired):
		// 410 rather than 403: the link was real and is now spent, and a client
		// that reads this as an access-control problem will not ask for a new one.
		status = http.StatusGone
		message = "the signed link has expired"
	case errors.Is(err, sourceprocessing.ErrQueueUnavailable), errors.Is(err, sourceprocessing.ErrStorageUnavailable), errors.Is(err, sourceprocessing.ErrAIUnavailable), errors.Is(err, sourceprocessing.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "source processing service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
