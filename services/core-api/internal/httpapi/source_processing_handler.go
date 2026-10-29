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

type createSourceTreeDraftRequest struct {
	TitleAR string `json:"titleAr"`
}

func (h sourceProcessingHandler) chartLibrary(w http.ResponseWriter, r *http.Request) {
	actorID := ""
	if h.Auth != nil {
		if user, err := h.Auth.UserFromRequest(r.Context(), r); err == nil {
			actorID = user.ID
		}
	}
	items, err := h.Service.ListChartLibrary(r.Context(), actorID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type judgeSourceSuggestionRequest struct {
	Candidates []sourceprocessing.JudgeCandidate `json:"candidates"`
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

func (h sourceProcessingHandler) pageEvidence(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	pageNumber, err := strconv.Atoi(r.PathValue("pageNumber"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page number must be a number"})
		return
	}
	result, err := h.Service.PageEvidence(r.Context(), r.PathValue("fileID"), user.ID, pageNumber)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) pagePreview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	pageNumber, err := strconv.Atoi(r.PathValue("pageNumber"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page number must be a number"})
		return
	}
	preview, err := h.Service.PagePreview(r.Context(), r.PathValue("fileID"), user.ID, pageNumber)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(preview)))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(preview)
}

func (h sourceProcessingHandler) pageEvidenceIndex(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.PageEvidenceIndex(r.Context(), r.PathValue("fileID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) reviewPageEvidence(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	pageNumber, err := strconv.Atoi(r.PathValue("pageNumber"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page number must be a number"})
		return
	}
	var input sourceprocessing.PageReviewInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.SavePageReview(r.Context(), r.PathValue("fileID"), user.ID, pageNumber, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, result)
}

func (h sourceProcessingHandler) reprocessFile(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.ReprocessInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReprocessFile(r.Context(), r.PathValue("fileID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusAccepted, result)
}

func (h sourceProcessingHandler) reextractReviewedPage(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	pageNumber, err := strconv.Atoi(r.PathValue("pageNumber"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "page number must be a number"})
		return
	}
	var input sourceprocessing.ReextractPageInput
	if !decodeRequest(w, r, &input) {
		return
	}
	input.PageNumber = pageNumber
	result, err := h.Service.ReextractReviewedPage(r.Context(), r.PathValue("fileID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusAccepted, result)
}

func (h sourceProcessingHandler) createTreeDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input createSourceTreeDraftRequest
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.CreateTreeDraft(r.Context(), r.PathValue("fileID"), user.ID, input.TitleAR)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, result)
}

func (h sourceProcessingHandler) startChartAnalysis(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.StartChartAnalysis(r.Context(), r.PathValue("fileID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusAccepted, result)
}

func (h sourceProcessingHandler) getChartAnalysis(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetChartAnalysis(r.Context(), r.PathValue("fileID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) retryChartAnalysis(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.RetryChartAnalysis(r.Context(), r.PathValue("runID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusAccepted, result)
}

func (h sourceProcessingHandler) saveChartAnalysisReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.ChartAnalysisReviewPayload
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.SaveChartAnalysisReview(r.Context(), r.PathValue("runID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, result)
}

// listSourceSuggestions returns the review queue for a file: the machine's
// readings, the most certain first.
func (h sourceProcessingHandler) listSourceSuggestions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 200 {
			writeSourceProcessingError(w, sourceprocessing.ErrValidation)
			return
		}
		limit = parsed
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			writeSourceProcessingError(w, sourceprocessing.ErrValidation)
			return
		}
		offset = parsed
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	var result []sourceprocessing.SuggestionView
	var err error
	if strings.Contains(r.URL.Path, "/review-queue") {
		result, err = h.Service.NameReviewQueue(r.Context(), r.PathValue("fileID"), user.ID, status, limit, offset)
	} else {
		result, err = h.Service.ListSuggestions(r.Context(), r.PathValue("fileID"), user.ID, status, limit, offset)
		if err == nil {
			err = h.Service.RecordLegacyNameExposure(r.Context(), result)
		}
	}
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

// decideSourceSuggestion records what a reviewer decided about a suggestion.
func (h sourceProcessingHandler) decideSourceSuggestion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.SuggestionDecision
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.DecideSuggestion(r.Context(), r.PathValue("suggestionID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, result)
}

// sourceSuggestionHistory returns every human revision and review event. It is
// a read of the ledger, not a reconstruction from the current queue row.
func (h sourceProcessingHandler) sourceSuggestionHistory(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.SuggestionHistory(r.Context(), r.PathValue("suggestionID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

// reopenSourceSuggestion appends a reopen event and returns the occurrence to
// the pending queue. The previous decision remains in history.
func (h sourceProcessingHandler) reopenSourceSuggestion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.SuggestionReopenInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReopenSuggestion(r.Context(), r.PathValue("suggestionID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

// sourceSuggestionRegionPreview renders a proposed rectangle without appending a
// revision, so cancel is a no-op on review state.
func (h sourceProcessingHandler) sourceSuggestionRegionPreview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.SuggestionRegionInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.PreviewSuggestionRegion(r.Context(), r.PathValue("suggestionID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) sourceSuggestionRegion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.SuggestionRegionInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReviseSuggestionRegion(r.Context(), r.PathValue("suggestionID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, result)
}

func (h sourceProcessingHandler) splitSourceSuggestion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.SuggestionSplitInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.SplitSuggestion(r.Context(), r.PathValue("suggestionID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, result)
}

func (h sourceProcessingHandler) mergeSourceSuggestion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.SuggestionMergeInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.MergeSuggestions(r.Context(), r.PathValue("suggestionID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, result)
}

// nameReviewSummary reports counts only. It never exposes a reading or a judge
// result, and it is scoped by the same source permission as the queue.
func (h sourceProcessingHandler) nameReviewSummary(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.NameReviewSummary(r.Context(), r.PathValue("fileID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

// sourceSuggestionCrop renders the suggestion's name and its surrounding context
// from the original source, so a person can read the name instead of trusting a
// thumbnail.
func (h sourceProcessingHandler) sourceSuggestionCrop(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	crop, err := h.Service.GetSuggestionCrop(r.Context(), r.PathValue("suggestionID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, crop)
}

// sourceSuggestionCandidates asks the vision model for spelling candidates for
// one suggestion's tight crop. It records nothing: the answer is a short list of
// hypotheses for the editor, and the human decision remains the only record.
func (h sourceProcessingHandler) sourceSuggestionCandidates(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	candidates, err := h.Service.SuggestionCandidates(r.Context(), r.PathValue("suggestionID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, map[string]any{"candidates": candidates})
}

// readSourceSuggestions runs the reading pipeline over a file and records the
// suggestions it finds.
func (h sourceProcessingHandler) readSourceSuggestions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.PrepareNameReading(r.Context(), r.PathValue("fileID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusAccepted, result)
}

// sourceSuggestionAccuracy reports how often the reader was right without
// correction, from the decisions the reviewers made.
func (h sourceProcessingHandler) sourceSuggestionAccuracy(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.SuggestionAccuracy(r.Context(), r.PathValue("fileID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) readingCapabilities(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.SourceReadingCapabilities(r.Context(), r.PathValue("fileID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

// nameJudgeConfig tells the review bench whether a judge is available. The
// configuration is not a health check; a configured judge can still answer 503
// when the hosted provider is down, and the bench must remain usable.
func (h sourceProcessingHandler) nameJudgeConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, h.Service.NameJudgeConfig())
}

// judgeSourceSuggestion asks the configured judge for an opinion on a bounded
// candidate set. It never writes a decision or a name.
func (h sourceProcessingHandler) judgeSourceSuggestion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input judgeSourceSuggestionRequest
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.JudgeSuggestion(r.Context(), r.PathValue("suggestionID"), user.ID, input.Candidates)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) listBookAssertions(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.ListBookAssertions(r.Context(), r.PathValue("fileID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) createBookAssertion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.BookAssertionInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.CreateBookAssertion(r.Context(), r.PathValue("fileID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, result)
}

func (h sourceProcessingHandler) updateBookAssertion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.UpdateBookAssertionInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.UpdateBookAssertion(r.Context(), r.PathValue("assertionID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) reviewBookAssertion(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.ReviewBookAssertionInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ReviewBookAssertion(r.Context(), r.PathValue("assertionID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) getTreeDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.GetTreeDraft(r.Context(), r.PathValue("draftID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) saveTreeDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.SaveTreeDraftInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.SaveTreeDraft(r.Context(), r.PathValue("draftID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) importTreeDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.TreeDraftImportInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.ImportTreeDraft(r.Context(), r.PathValue("draftID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusCreated, result)
}

// previewTreeDraftImport is the import's dry run: the counts and the
// collisions the apply run would realise, with nothing written.
func (h sourceProcessingHandler) previewTreeDraftImport(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		ExpectedRevision int `json:"expectedRevision"`
	}
	if !decodeRequest(w, r, &body) {
		return
	}
	result, err := h.Service.PreviewTreeDraftImport(r.Context(), r.PathValue("draftID"), user.ID, body.ExpectedRevision)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) retractTreeDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.RetractTreeDraft(r.Context(), r.PathValue("draftID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
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
	w.Header().Set("Referrer-Policy", "no-referrer")
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
	// Reader errors carry stable, privacy-safe codes; provider text never escapes.
	readerCode := ""
	readerStatus := http.StatusServiceUnavailable
	switch {
	case errors.Is(err, sourceprocessing.ErrChartVisionPolicy):
		readerCode = "disabled_by_policy"
		readerStatus = http.StatusForbidden
	case errors.Is(err, sourceprocessing.ErrChartVisionSetup):
		readerCode = "not_configured"
	case errors.Is(err, sourceprocessing.ErrChartVisionUnavailable):
		readerCode = "temporarily_unavailable"
	case errors.Is(err, sourceprocessing.ErrChartVisionOutputInvalid), errors.Is(err, sourceprocessing.ErrChartVisionTruncated), errors.Is(err, sourceprocessing.ErrChartVisionRejected):
		readerCode = "unreadable_crop"
		readerStatus = http.StatusUnprocessableEntity
	}
	if readerCode != "" {
		writeJSON(w, readerStatus, map[string]string{"error": "chart reading is unavailable", "code": readerCode})
		return
	}
	status := http.StatusInternalServerError
	message := "source processing operation failed"
	switch {
	case errors.Is(err, sourceprocessing.ErrValidation):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, sourceprocessing.ErrNotFound), errors.Is(err, sourceprocessing.ErrSuggestionNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, sourceprocessing.ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, sourceprocessing.ErrConflict), errors.Is(err, sourceprocessing.ErrSuggestionAlreadyDecided), errors.Is(err, sourceprocessing.ErrSuggestionSuperseded):
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
	case errors.Is(err, sourceprocessing.ErrQueueUnavailable), errors.Is(err, sourceprocessing.ErrStorageUnavailable), errors.Is(err, sourceprocessing.ErrAIUnavailable), errors.Is(err, sourceprocessing.ErrDatabaseUnavailable), errors.Is(err, sourceprocessing.ErrPDFToolsUnavailable), errors.Is(err, sourceprocessing.ErrArabicOCRUnavailable), errors.Is(err, sourceprocessing.ErrChartVisionUnavailable), errors.Is(err, sourceprocessing.ErrChartVisionSetup), errors.Is(err, sourceprocessing.ErrNameJudgeUnavailable), errors.Is(err, sourceprocessing.ErrNameJudgeSetup):
		status = http.StatusServiceUnavailable
		message = "source processing service is not configured"
	case errors.Is(err, sourceprocessing.ErrChartVisionRejected), errors.Is(err, sourceprocessing.ErrChartVisionTruncated), errors.Is(err, sourceprocessing.ErrChartVisionOutputInvalid), errors.Is(err, sourceprocessing.ErrNameJudgeRejected), errors.Is(err, sourceprocessing.ErrNameJudgeOutputInvalid):
		status = http.StatusUnprocessableEntity
		message = "the name judge could not answer for this candidate set"
	}
	writeJSON(w, status, map[string]string{"error": message})
}

func (h sourceProcessingHandler) nameReadingRun(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var result sourceprocessing.NameReadingRun
	var err error
	switch r.PathValue("action") {
	case "cancel":
		result, err = h.Service.CancelNameReading(r.Context(), r.PathValue("runID"), user.ID)
	case "resume":
		result, err = h.Service.ResumeNameReading(r.Context(), r.PathValue("runID"), user.ID)
	default:
		result, err = h.Service.NameReadingRun(r.Context(), r.PathValue("runID"), user.ID)
	}
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}
func (h sourceProcessingHandler) latestNameReading(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var result sourceprocessing.NameReadingRun
	var err error
	if r.PathValue("suggestionID") != "" {
		result, err = h.Service.LatestSuggestionNameReadingRun(r.Context(), r.PathValue("suggestionID"), user.ID)
	} else {
		result, err = h.Service.LatestNameReadingRun(r.Context(), r.PathValue("fileID"), user.ID)
	}
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}
func (h sourceProcessingHandler) localNameReread(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.RereadSuggestionLocally(r.Context(), r.PathValue("suggestionID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusAccepted, result)
}
func (h sourceProcessingHandler) localNameReadings(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.SuggestionLocalReadings(r.Context(), r.PathValue("suggestionID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}

func (h sourceProcessingHandler) nameLexicon(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.NameLexicon(r.Context(), r.PathValue("fileID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}
func (h sourceProcessingHandler) beginNameReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.BeginNameReview(r.Context(), r.PathValue("suggestionID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, result)
}
func (h sourceProcessingHandler) sealNameReview(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	if err := h.Service.SealNameReview(r.Context(), r.PathValue("suggestionID"), user.ID); err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sealed": true})
}
func (h sourceProcessingHandler) prepareNameCandidates(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input sourceprocessing.PrepareNameCandidatesInput
	if !decodeRequest(w, r, &input) {
		return
	}
	result, err := h.Service.PrepareNameCandidates(r.Context(), r.PathValue("suggestionID"), user.ID, input)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusAccepted, result)
}
func (h sourceProcessingHandler) nameEvaluation(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	// The lexicon method performs normal source-review authorization. The
	// operator contract/registration never returns its private entries.
	if _, err := h.Service.NameLexicon(r.Context(), r.PathValue("fileID"), user.ID); err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	if r.Method == http.MethodGet {
		writeJSON(w, http.StatusOK, h.Service.NameEvaluationContract(r.Context()))
		return
	}
	var input sourceprocessing.NameEvaluationRegistration
	if !decodeRequest(w, r, &input) {
		return
	}
	if err := h.Service.RegisterNameEvaluation(r.Context(), r.PathValue("fileID"), user.ID, input); err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"registered": true})
}
func (h sourceProcessingHandler) nameEvidenceStatus(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	result, err := h.Service.NameEvidenceStatus(r.Context(), r.PathValue("fileID"), user.ID)
	if err != nil {
		writeSourceProcessingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
