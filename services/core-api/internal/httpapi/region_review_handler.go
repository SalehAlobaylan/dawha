package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/geography"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/regionreview"
)

// regionReviewHandler is the transport for plan 030-E's reviewer lane: a
// bounded moderator read over external_region_candidates, a crop endpoint
// that serves the plate's own pixels, and the decision endpoints that carry a
// named reviewer's reading and verdict. The service owns the rules; this file
// owns the HTTP.
type regionReviewHandler struct {
	Service *regionreview.Service
	Auth    *auth.Service
}

func (h regionReviewHandler) listCandidates(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.List(r.Context(), h.actorID(r), regionreview.ListParams{
		Status:   r.URL.Query().Get("status"),
		PlateKey: r.URL.Query().Get("plate_key"),
		Query:    r.URL.Query().Get("q"),
	})
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h regionReviewHandler) getCandidate(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.Get(r.Context(), h.actorID(r), r.PathValue("candidateID"))
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h regionReviewHandler) history(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.History(r.Context(), h.actorID(r), r.PathValue("candidateID"))
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// nameSuggestions serves the workbench's choosing surface (plan 035): the
// dictionary's own tribal names matching the typed prefix, bounded. The
// choose row itself is a plain read — the reviewer still confirms it.
func (h regionReviewHandler) nameSuggestions(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.NameSuggestions(r.Context(), h.actorID(r), r.URL.Query().Get("q"))
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// candidateCrop renders the two review images for one candidate from the
// stored plate: the tight crop the detector ranked, and its context. The
// bytes travel as base64 in a JSON body ([]byte's encoding/json behaviour),
// the same contract the source-suggestion crops use, so the workbench opens
// one request and decodes both.
func (h regionReviewHandler) candidateCrop(w http.ResponseWriter, r *http.Request) {
	report, err := h.Service.CandidateCrop(r.Context(), h.actorID(r), r.PathValue("candidateID"))
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, report)
}

func (h regionReviewHandler) decideCandidate(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action      string  `json:"action"`
		NoteAR      string  `json:"noteAr"`
		LabelTextAR *string `json:"labelTextAr"`
		EraLabel    *string `json:"eraLabel"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decision body must be JSON"})
		return
	}
	report, err := h.Service.Decide(r.Context(), h.actorID(r), r.PathValue("candidateID"), regionreview.DecideInput{
		Action:      input.Action,
		NoteAR:      input.NoteAR,
		LabelTextAR: input.LabelTextAR,
		EraLabel:    input.EraLabel,
	})
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// plateTile renders one warped XYZ tile as PNG (plan 032-B). The response is
// a map layer's raster: immutable for a day keyed on the rendered bytes,
// carrying the transform's measured floor and its honesty flag in headers
// the atlas control surfaces read.
//
// The URL keeps the XYZ contract's own `{z}/{x}/{y}.png` shape; ServeMux
// wildcards cannot end a segment, so the route owns the subtree and this
// handler parses the remainder strictly — exactly three parts, an integer
// zoom in the useful band, a grid-legal x/y, and a literal `.png` suffix.
func (h regionReviewHandler) plateTile(w http.ResponseWriter, r *http.Request) {
	plateKey := r.PathValue("plateKey")
	remainder := strings.TrimPrefix(r.URL.Path, "/api/v1/plates/"+plateKey+"/tiles/")
	parts := strings.Split(remainder, "/")
	if len(parts) != 3 || !strings.HasSuffix(parts[2], ".png") {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	zoom, err := strconv.Atoi(parts[0])
	if err != nil {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	tileX, err := strconv.Atoi(parts[1])
	if err != nil {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	tileY, err := strconv.Atoi(strings.TrimSuffix(parts[2], ".png"))
	if err != nil {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	pngBytes, transformation, err := h.Service.WarpPlateTile(r.Context(), h.actorID(r), r.PathValue("plateKey"), zoom, tileX, tileY)
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("ETag", fmt.Sprintf(`"%x"`, sha256.Sum256(pngBytes)))
	if transformation.Fit.LonRMSDeg > 0 || transformation.Fit.LatRMSDeg > 0 {
		w.Header().Set("X-Plate-Transform-RMS",
			fmt.Sprintf("lon=%.5f deg; lat=%.5f deg", transformation.Fit.LonRMSDeg, transformation.Fit.LatRMSDeg))
	}
	w.Header().Set("X-Plate-Needs-Review", strconv.FormatBool(transformation.NeedsReview))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pngBytes)
}

// platePointCrop reads the comparison panel's point (plan 032-C): the plate's
// own pixels around one map coordinate, through the stored inverse. The crop
// travels as base64 in the JSON body, the crop lane's own contract.
func (h regionReviewHandler) platePointCrop(w http.ResponseWriter, r *http.Request) {
	lon, err := strconv.ParseFloat(r.URL.Query().Get("lon"), 64)
	if err != nil {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	lat, err := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	if err != nil {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	side := regionreview.PointCropSide
	if rawSide := r.URL.Query().Get("size"); rawSide != "" {
		if side, err = strconv.Atoi(rawSide); err != nil {
			writeRegionReviewError(w, regionreview.ErrValidation)
			return
		}
	}
	report, err := h.Service.PlatePointCrop(r.Context(), h.actorID(r), r.PathValue("plateKey"), lon, lat, side)
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, report)
}

// promoteCandidate writes the accepted candidate's own historical_regions
// row: one region, one association, one spatial receipt — the reviewer's
// own reading carried verbatim (plan 030-E's remaining surface).
func (h regionReviewHandler) promoteCandidate(w http.ResponseWriter, r *http.Request) {
	actorIDStr := h.actorID(r)
	candidateID := r.PathValue("candidateID")
	report, err := h.Service.PromoteCandidate(r.Context(), actorIDStr, candidateID)
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (h regionReviewHandler) actorID(r *http.Request) string {
	if h.Auth == nil {
		return ""
	}
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		return ""
	}
	return user.ID
}

func writeRegionReviewError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "region review operation failed"
	switch {
	case errors.Is(err, regionreview.ErrValidation):
		status = http.StatusBadRequest
		message = err.Error()
	case errors.Is(err, regionreview.ErrUnauthenticated):
		status = http.StatusUnauthorized
		message = err.Error()
	case errors.Is(err, regionreview.ErrForbidden):
		status = http.StatusForbidden
		message = err.Error()
	case errors.Is(err, regionreview.ErrNotFound):
		status = http.StatusNotFound
		message = err.Error()
	case errors.Is(err, regionreview.ErrAlreadyDecided):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, regionreview.ErrCannotReopen):
		status = http.StatusConflict
		message = err.Error()
	case errors.Is(err, geography.ErrPlateTransformInvalid):
		// An atlas read over a transform that fails the reader's own
		// validation (plan 032-A): the register carries the honest caption,
		// the tile/crop routes refuse with the config-fix status — the fix
		// is a corrected artifact, not a retry.
		status = http.StatusServiceUnavailable
		message = "plate transform is invalid; the overlay is disabled"
	case errors.Is(err, regionreview.ErrCropUnavailable):
		// A stored scan that is missing or unreadable is a configuration or
		// upload state, not a client mistake; the fix is re-upload, so the
		// status says the service cannot answer, and says why.
		status = http.StatusServiceUnavailable
		message = "candidate crop is unavailable"
	case errors.Is(err, regionreview.ErrDatabaseUnavailable):
		status = http.StatusServiceUnavailable
		message = "region review database is unavailable"
	}
	writeJSON(w, status, map[string]string{"error": message})
}

func (h regionReviewHandler) reviewBoundary(w http.ResponseWriter, r *http.Request) {
	var input regionreview.BoundaryReviewInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 16_000)).Decode(&input); err != nil {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	if err := h.Service.ReviewBoundary(r.Context(), h.actorID(r), r.PathValue("candidateID"), input); err != nil {
		writeRegionReviewError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h regionReviewHandler) captionHistory(w http.ResponseWriter, r *http.Request) {
	history, err := h.Service.CaptionHistory(r.Context(), h.actorID(r), r.PathValue("captionID"))
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, history)
}

func (h regionReviewHandler) proposeCaption(w http.ResponseWriter, r *http.Request) {
	var input struct {
		NameAR string `json:"nameAr"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&input); err != nil {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	if err := h.Service.ProposeCaption(r.Context(), h.actorID(r), r.PathValue("captionID"), input.NameAR); err != nil {
		writeRegionReviewError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h regionReviewHandler) publishCaption(w http.ResponseWriter, r *http.Request) {
	if err := h.Service.PublishCaption(r.Context(), h.actorID(r), r.PathValue("captionID"), r.PathValue("revisionID")); err != nil {
		writeRegionReviewError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h regionReviewHandler) reviewOccurrenceIdentity(w http.ResponseWriter, r *http.Request) {
	var input regionreview.OccurrenceIdentityInput
	if err := json.NewDecoder(io.LimitReader(r.Body, 8192)).Decode(&input); err != nil {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	if err := h.Service.ReviewOccurrenceIdentity(r.Context(), h.actorID(r), r.PathValue("occurrenceID"), input); err != nil {
		writeRegionReviewError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h regionReviewHandler) importAtlasStage(w http.ResponseWriter, r *http.Request) {
	var input regionreview.AtlasStage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	added, err := h.Service.ImportAtlasStage(r.Context(), h.actorID(r), r.PathValue("sourceID"), input)
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"added": added})
}

func (h regionReviewHandler) atlasProposals(w http.ResponseWriter, r *http.Request) {
	page, err := h.Service.AtlasProposals(r.Context(), h.actorID(r), r.URL.Query().Get("after"))
	if err != nil {
		writeRegionReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	writeJSON(w, http.StatusOK, page)
}

func (h regionReviewHandler) decideAtlasProposal(w http.ResponseWriter, r *http.Request) {
	var input regionreview.AtlasProposalDecision
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	if err := h.Service.DecideAtlasProposal(r.Context(), h.actorID(r), r.PathValue("proposalID"), input); err != nil {
		writeRegionReviewError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h regionReviewHandler) attachAtlasClaimReceipt(w http.ResponseWriter, r *http.Request) {
	var input regionreview.AtlasClaimReceiptInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&input); err != nil {
		writeRegionReviewError(w, regionreview.ErrValidation)
		return
	}
	if err := h.Service.AttachAtlasClaimReceipt(r.Context(), h.actorID(r), r.PathValue("occurrenceID"), input); err != nil {
		writeRegionReviewError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
