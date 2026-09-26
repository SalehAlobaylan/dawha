package httpapi

import (
	"net/http"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/identity"
)

// identityHandler is the write surface for the global identity rows. Every route
// here is gated on the identity role in internal/auth, which is the same gate the
// service applies inside its transaction: the handler decides who is calling, and
// the service decides whether that caller may write, so neither layer is trusted
// on its own.
type identityHandler struct {
	Service *identity.Service
	Auth    *auth.Service
}

func (h identityHandler) listPeople(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	response, err := h.Service.ListPeople(r.Context(), user.ID, r.URL.Query().Get("q"))
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h identityHandler) getPerson(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	person, err := h.Service.GetPerson(r.Context(), r.PathValue("personID"), user.ID)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, person)
}

func (h identityHandler) createPerson(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.CreatePersonInput
	if !decodeRequest(w, r, &input) {
		return
	}
	person, err := h.Service.CreatePerson(r.Context(), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, person)
}

func (h identityHandler) updatePerson(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.UpdatePersonInput
	if !decodeRequest(w, r, &input) {
		return
	}
	person, err := h.Service.UpdatePerson(r.Context(), r.PathValue("personID"), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, person)
}

func (h identityHandler) deletePerson(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var request identity.ReasonRequest
	if !decodeOptionalRequest(w, r, &request) {
		return
	}
	if err := h.Service.DeletePerson(r.Context(), r.PathValue("personID"), user.ID, request.ReasonAR); err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h identityHandler) listAliases(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	response, err := h.Service.ListPersonAliases(r.Context(), r.PathValue("personID"), user.ID)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h identityHandler) createAlias(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.PersonAliasInput
	if !decodeRequest(w, r, &input) {
		return
	}
	alias, err := h.Service.CreatePersonAlias(r.Context(), r.PathValue("personID"), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, alias)
}

func (h identityHandler) updateAlias(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.PersonAliasInput
	if !decodeRequest(w, r, &input) {
		return
	}
	alias, err := h.Service.UpdatePersonAlias(r.Context(), r.PathValue("aliasID"), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, alias)
}

func (h identityHandler) deleteAlias(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var request identity.ReasonRequest
	if !decodeOptionalRequest(w, r, &request) {
		return
	}
	if err := h.Service.DeletePersonAlias(r.Context(), r.PathValue("aliasID"), user.ID, request.ReasonAR); err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h identityHandler) listFamilies(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	response, err := h.Service.ListFamilies(r.Context(), user.ID, r.URL.Query().Get("q"))
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h identityHandler) getFamily(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	family, err := h.Service.GetFamily(r.Context(), r.PathValue("familyID"), user.ID)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, family)
}

func (h identityHandler) createFamily(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.CreateFamilyInput
	if !decodeRequest(w, r, &input) {
		return
	}
	family, err := h.Service.CreateFamily(r.Context(), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, family)
}

func (h identityHandler) updateFamily(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.UpdateFamilyInput
	if !decodeRequest(w, r, &input) {
		return
	}
	family, err := h.Service.UpdateFamily(r.Context(), r.PathValue("familyID"), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, family)
}

func (h identityHandler) deleteFamily(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var request identity.ReasonRequest
	if !decodeOptionalRequest(w, r, &request) {
		return
	}
	if err := h.Service.DeleteFamily(r.Context(), r.PathValue("familyID"), user.ID, request.ReasonAR); err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h identityHandler) listTribes(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	response, err := h.Service.ListTribes(r.Context(), user.ID, r.URL.Query().Get("q"))
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h identityHandler) getTribe(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	tribe, err := h.Service.GetTribe(r.Context(), r.PathValue("tribeID"), user.ID)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tribe)
}

func (h identityHandler) createTribe(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.CreateTribeInput
	if !decodeRequest(w, r, &input) {
		return
	}
	tribe, err := h.Service.CreateTribe(r.Context(), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, tribe)
}

func (h identityHandler) updateTribe(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.UpdateTribeInput
	if !decodeRequest(w, r, &input) {
		return
	}
	tribe, err := h.Service.UpdateTribe(r.Context(), r.PathValue("tribeID"), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tribe)
}

func (h identityHandler) deleteTribe(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var request identity.ReasonRequest
	if !decodeOptionalRequest(w, r, &request) {
		return
	}
	if err := h.Service.DeleteTribe(r.Context(), r.PathValue("tribeID"), user.ID, request.ReasonAR); err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h identityHandler) listBranches(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	response, err := h.Service.ListBranches(r.Context(), user.ID, r.URL.Query().Get("q"))
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h identityHandler) getBranch(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	branch, err := h.Service.GetBranch(r.Context(), r.PathValue("branchID"), user.ID)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, branch)
}

func (h identityHandler) createBranch(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.CreateBranchInput
	if !decodeRequest(w, r, &input) {
		return
	}
	branch, err := h.Service.CreateBranch(r.Context(), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, branch)
}

func (h identityHandler) updateBranch(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.UpdateBranchInput
	if !decodeRequest(w, r, &input) {
		return
	}
	branch, err := h.Service.UpdateBranch(r.Context(), r.PathValue("branchID"), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, branch)
}

func (h identityHandler) deleteBranch(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var request identity.ReasonRequest
	if !decodeOptionalRequest(w, r, &request) {
		return
	}
	if err := h.Service.DeleteBranch(r.Context(), r.PathValue("branchID"), user.ID, request.ReasonAR); err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h identityHandler) listPlaces(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	response, err := h.Service.ListPlaces(r.Context(), user.ID, r.URL.Query().Get("q"))
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h identityHandler) getPlace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	place, err := h.Service.GetPlace(r.Context(), r.PathValue("placeID"), user.ID)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, place)
}

func (h identityHandler) createPlace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.CreatePlaceInput
	if !decodeRequest(w, r, &input) {
		return
	}
	place, err := h.Service.CreatePlace(r.Context(), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, place)
}

func (h identityHandler) updatePlace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.UpdatePlaceInput
	if !decodeRequest(w, r, &input) {
		return
	}
	place, err := h.Service.UpdatePlace(r.Context(), r.PathValue("placeID"), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, place)
}

func (h identityHandler) deletePlace(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var request identity.ReasonRequest
	if !decodeOptionalRequest(w, r, &request) {
		return
	}
	if err := h.Service.DeletePlace(r.Context(), r.PathValue("placeID"), user.ID, request.ReasonAR); err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h identityHandler) listRelationships(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	response, err := h.Service.ListEntityRelationships(r.Context(), user.ID, r.URL.Query().Get("q"))
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h identityHandler) getRelationship(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	relationship, err := h.Service.GetEntityRelationship(r.Context(), r.PathValue("relationshipID"), user.ID)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, relationship)
}

func (h identityHandler) createRelationship(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.CreateEntityRelationshipInput
	if !decodeRequest(w, r, &input) {
		return
	}
	relationship, err := h.Service.CreateEntityRelationship(r.Context(), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, relationship)
}

func (h identityHandler) updateRelationship(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var input identity.UpdateEntityRelationshipInput
	if !decodeRequest(w, r, &input) {
		return
	}
	relationship, err := h.Service.UpdateEntityRelationship(r.Context(), r.PathValue("relationshipID"), user.ID, input)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, relationship)
}

func (h identityHandler) deleteRelationship(w http.ResponseWriter, r *http.Request) {
	user, ok := h.actor(w, r)
	if !ok {
		return
	}
	var request identity.ReasonRequest
	if !decodeOptionalRequest(w, r, &request) {
		return
	}
	if err := h.Service.DeleteEntityRelationship(r.Context(), r.PathValue("relationshipID"), user.ID, request.ReasonAR); err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// actor resolves the session once per request. An anonymous caller is 401 here,
// before any body is read, so an unauthenticated request never reaches the
// validation or the database at all.
func (h identityHandler) actor(w http.ResponseWriter, r *http.Request) (auth.User, bool) {
	user, err := h.Auth.UserFromRequest(r.Context(), r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return auth.User{}, false
	}
	return user, true
}

// decodeOptionalRequest reads a body that is allowed to be absent. A delete takes
// an optional reason, so a request with no body and no content type is a valid
// request rather than a malformed one. A body that is present must still be a
// complete JSON object with no unknown fields, or it is a 400.
func decodeOptionalRequest(w http.ResponseWriter, r *http.Request, destination any) bool {
	if r.ContentLength == 0 && strings.TrimSpace(r.Header.Get("Content-Type")) == "" {
		return true
	}
	return decodeRequest(w, r, destination)
}

// writeIdentityError maps the service errors onto the status codes the rest of the
// API already uses. Every refusal answers 409, including the published
// interpretation ones: the request is well formed and the caller is allowed to
// make it, and the record's current state is what forbids it. A refusal body
// carries only the error's own wording, never the record, so a caller cannot read
// the row it was refused.
func writeIdentityError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "identity operation failed"
	switch err {
	case identity.ErrValidation:
		status = http.StatusBadRequest
		message = err.Error()
	case identity.ErrNotFound:
		status = http.StatusNotFound
		message = err.Error()
	case identity.ErrForbidden:
		status = http.StatusForbidden
		message = err.Error()
	case identity.ErrConflict,
		identity.ErrPublishedInterpretation,
		identity.ErrReferencedByInterpretation,
		identity.ErrMergedIdentity,
		identity.ErrPublishedReference,
		identity.ErrResearchReference,
		identity.ErrSettledInterpretation:
		status = http.StatusConflict
		message = err.Error()
	case identity.ErrDatabaseUnavailable:
		status = http.StatusServiceUnavailable
		message = "identity service is not configured"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
