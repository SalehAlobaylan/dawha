// Package identity is the write surface for the global identity rows: people,
// person aliases, families, tribes, branches, places and generic entity
// relationships. It also owns the normalization those rows are indexed by.
//
// The package draws two lines that the rest of the platform depends on, so they
// are stated here once and enforced by every function below.
//
// Research records. A person, a person alias and an entity relationship are
// research records. Plan 001 makes a person public only when it appears in a
// published tree version, and it scopes a person alias by the source it was
// taken from. Nothing in this package can make a person public: a create writes
// no tree version, a create binds no identity_status, and a create binds no
// merged_into_id, so a row created here belongs to no published interpretation
// and the central visibility policy keeps it invisible to an anonymous reader.
// Reads go through visibility.Load plus the matching predicate, exactly as the
// dictionary and the search service do, so this package cannot become a second,
// laxer answer to "may this actor read this row".
//
// Published interpretations. A mutation that would rewrite something a
// published tree version already rests on is refused, not applied. That is
// ErrPublishedInterpretation for a person, a person alias and an entity
// relationship, ErrReferencedByInterpretation for a delete that any tree version
// holds a node for, ErrMergedIdentity for a row an audited merge has already
// frozen, and ErrPublishedReference for a reference row that is already public.
//
// Research-only reference rows. A family, tribe, branch or place created through
// this API is research-only: the create binds the visibility column to 'private',
// so the central policy keeps the row off the anonymous dictionary index, the
// dictionary detail, the search index and the map, and only the identity write
// role set can read it. Editing one in place is refused with
// ErrResearchReference until it is published, because the table has no draft to
// edit - publishing the row is the supported path.
//
// Every mutation is one transaction covering the row and its audit event, with
// the authorization decision taken inside that transaction before the first
// write, so a refusal or a failed audit insert leaves nothing behind.
package identity

import (
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrDatabaseUnavailable is returned when the service has no pool.
	ErrDatabaseUnavailable = errors.New("identity database is unavailable")
	// ErrNotFound is returned for a missing row. A denied row answers with the
	// same error, so a read cannot be used to discover which rows exist.
	ErrNotFound = errors.New("identity resource not found")
	// ErrForbidden is returned when the actor is not permitted to write a global
	// identity row.
	ErrForbidden = errors.New("identity actor is not permitted")
	// ErrValidation is returned for a malformed or out-of-vocabulary field.
	ErrValidation = errors.New("identity input is invalid")
	// ErrConflict is returned for a uniqueness conflict.
	ErrConflict = errors.New("identity record conflicts with an existing one")
	// ErrPublishedInterpretation is returned when a mutation would rewrite a row a
	// published tree version already rests on. It is stable: the same condition
	// always answers with this error and the same wording, so a caller can tell
	// "you may not change this" from "this does not exist".
	ErrPublishedInterpretation = errors.New("identity record is part of a published interpretation and is not editable here")
	// ErrReferencedByInterpretation is returned when a delete would remove a row
	// that any tree version holds a node for. The tree is the interpretation of
	// record, so the node is removed by editing the tree, never by deleting the
	// identity row behind it.
	ErrReferencedByInterpretation = errors.New("identity record is used by a tree interpretation and is not deletable here")
	// ErrMergedIdentity is returned for a row an audited merge has already frozen.
	// Editing it would rewrite the outcome of a recorded review decision, and
	// unmerging belongs to the merge path that set it.
	ErrMergedIdentity = errors.New("identity record was merged and is not editable here")
	// ErrPublishedReference is returned for an update or delete of a family, tribe,
	// branch or place that is already public. Those four tables have a visibility
	// column but no version table, so a published row has nothing to supersede it:
	// an edit would silently rewrite what the public dictionary index and the public
	// map already serve, and a reader would have no way to see that it changed.
	ErrPublishedReference = errors.New("published family, tribe, branch and place records have no version to supersede them and are not editable here")
	// ErrResearchReference is returned for an update or delete of a family, tribe,
	// branch or place that is still research-only. The reason is the mirror image
	// and is deliberately a different error: publishing the row is the supported
	// path, and editing it in place is not, because there is no draft to edit.
	ErrResearchReference = errors.New("research-only family, tribe, branch and place records become editable once they are published")
	// ErrSettledInterpretation is returned for an update or delete of an entity
	// relationship whose status is no longer unresolved. A resolved relationship
	// records a human decision; this API may create, refine or drop an unresolved
	// one, and reversing a settled reading is a different, reviewed action.
	ErrSettledInterpretation = errors.New("entity relationship was already resolved and is not editable here")
)

// Service writes the global identity rows. It holds no repository abstraction:
// every statement below is written out, because the write surface for shared
// identity data is exactly the place where a clever generic layer would hide
// which column a caller string ended up in.
type Service struct {
	Pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool}
}

// ---------------------------------------------------------------- requests --

// CreatePersonInput is the person create request. identity_status, merged_into_id
// and created_by are absent on purpose: a new person is unreviewed, unmerged, and
// attributed to the authenticated actor.
type CreatePersonInput struct {
	CanonicalNameAR string `json:"canonical_name_ar"`
	Gender          string `json:"gender"`
	BirthDateFrom   string `json:"birth_date_from"`
	BirthDateTo     string `json:"birth_date_to"`
	DeathDateFrom   string `json:"death_date_from"`
	DeathDateTo     string `json:"death_date_to"`
	NotesAR         string `json:"notes_ar"`
}

// UpdatePersonInput is the person update request. Every field is bound
// explicitly, so an unset field clears the column rather than being ignored: a
// partial update that silently kept a stale date would be indistinguishable from
// one that removed it.
type UpdatePersonInput struct {
	CanonicalNameAR string `json:"canonical_name_ar"`
	Gender          string `json:"gender"`
	BirthDateFrom   string `json:"birth_date_from"`
	BirthDateTo     string `json:"birth_date_to"`
	DeathDateFrom   string `json:"death_date_from"`
	DeathDateTo     string `json:"death_date_to"`
	NotesAR         string `json:"notes_ar"`
	ReasonAR        string `json:"reason_ar"`
}

// PersonAliasInput is the person alias create and update request. source_id
// names the source a spelling was taken from; an alias without one is a platform
// record, which is the only kind plan 001 lets a public reader see.
type PersonAliasInput struct {
	ValueAR   string `json:"value_ar"`
	AliasType string `json:"alias_type"`
	SourceID  string `json:"source_id"`
	ReasonAR  string `json:"reason_ar"`
}

type CreateFamilyInput struct {
	CanonicalNameAR string `json:"canonical_name_ar"`
	DescriptionAR   string `json:"description_ar"`
	OriginPlaceID   string `json:"origin_place_id"`
}

type UpdateFamilyInput struct {
	CanonicalNameAR string `json:"canonical_name_ar"`
	DescriptionAR   string `json:"description_ar"`
	OriginPlaceID   string `json:"origin_place_id"`
	ReasonAR        string `json:"reason_ar"`
}

type CreateTribeInput struct {
	CanonicalNameAR string `json:"canonical_name_ar"`
	DescriptionAR   string `json:"description_ar"`
}

type UpdateTribeInput struct {
	CanonicalNameAR string `json:"canonical_name_ar"`
	DescriptionAR   string `json:"description_ar"`
	ReasonAR        string `json:"reason_ar"`
}

type CreateBranchInput struct {
	FamilyID        string `json:"family_id"`
	ParentBranchID  string `json:"parent_branch_id"`
	CanonicalNameAR string `json:"canonical_name_ar"`
	ValidFrom       string `json:"valid_from"`
	ValidTo         string `json:"valid_to"`
	NotesAR         string `json:"notes_ar"`
}

type UpdateBranchInput struct {
	ParentBranchID  string `json:"parent_branch_id"`
	CanonicalNameAR string `json:"canonical_name_ar"`
	ValidFrom       string `json:"valid_from"`
	ValidTo         string `json:"valid_to"`
	NotesAR         string `json:"notes_ar"`
	ReasonAR        string `json:"reason_ar"`
}

type CreatePlaceInput struct {
	CanonicalNameAR string   `json:"canonical_name_ar"`
	PlaceType       string   `json:"place_type"`
	ParentPlaceID   string   `json:"parent_place_id"`
	NotesAR         string   `json:"notes_ar"`
	Longitude       *float64 `json:"longitude"`
	Latitude        *float64 `json:"latitude"`
}

type UpdatePlaceInput struct {
	CanonicalNameAR string   `json:"canonical_name_ar"`
	PlaceType       string   `json:"place_type"`
	ParentPlaceID   string   `json:"parent_place_id"`
	NotesAR         string   `json:"notes_ar"`
	Longitude       *float64 `json:"longitude"`
	Latitude        *float64 `json:"latitude"`
	ReasonAR        string   `json:"reason_ar"`
}

// CreateEntityRelationshipInput is the generic relationship create request. The
// status is absent on purpose: a create always writes the unresolved status, so a
// relationship cannot be born already settled.
type CreateEntityRelationshipInput struct {
	SubjectType string `json:"subject_type"`
	SubjectID   string `json:"subject_id"`
	Predicate   string `json:"predicate"`
	ObjectType  string `json:"object_type"`
	ObjectID    string `json:"object_id"`
	ValidFrom   string `json:"valid_from"`
	ValidTo     string `json:"valid_to"`
}

type UpdateEntityRelationshipInput struct {
	Predicate string `json:"predicate"`
	ValidFrom string `json:"valid_from"`
	ValidTo   string `json:"valid_to"`
	Status    string `json:"status"`
	ReasonAR  string `json:"reason_ar"`
}

// ReasonRequest is the optional body of a delete. The audit event records it
// when it is present; it is never required, because requiring a reason would only
// teach a caller to type a single character to get past the check.
type ReasonRequest struct {
	ReasonAR string `json:"reason_ar"`
}

// ----------------------------------------------------------------- responses -

// PersonView is the person read and write response. It carries the columns the
// dictionary already serves for a public person plus the gender and date bounds,
// and it is only ever returned to an actor that passed the identity gate, so it
// adds no public read path.
type PersonView struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	NameAR         string `json:"nameAr"`
	Gender         string `json:"gender"`
	IdentityStatus string `json:"identityStatus"`
	BirthDateFrom  string `json:"birthDateFrom,omitempty"`
	BirthDateTo    string `json:"birthDateTo,omitempty"`
	DeathDateFrom  string `json:"deathDateFrom,omitempty"`
	DeathDateTo    string `json:"deathDateTo,omitempty"`
	NotesAR        string `json:"notesAr,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
}

// PersonAliasView is one alias of a person.
type PersonAliasView struct {
	ID        string `json:"id"`
	PersonID  string `json:"personId"`
	ValueAR   string `json:"valueAr"`
	AliasType string `json:"aliasType"`
	SourceID  string `json:"sourceId,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// FamilyView is the family read and write response.
type FamilyView struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	NameAR        string `json:"nameAr"`
	DescriptionAR string `json:"descriptionAr,omitempty"`
	OriginPlaceID string `json:"originPlaceId,omitempty"`
	CreatedAt     string `json:"createdAt,omitempty"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
}

// TribeView is the tribe read and write response.
type TribeView struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	NameAR        string `json:"nameAr"`
	DescriptionAR string `json:"descriptionAr,omitempty"`
	CreatedAt     string `json:"createdAt,omitempty"`
	UpdatedAt     string `json:"updatedAt,omitempty"`
}

// BranchView is the branch read and write response.
type BranchView struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	FamilyID       string `json:"familyId"`
	ParentBranchID string `json:"parentBranchId,omitempty"`
	NameAR         string `json:"nameAr"`
	ValidFrom      string `json:"validFrom,omitempty"`
	ValidTo        string `json:"validTo,omitempty"`
	NotesAR        string `json:"notesAr,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
}

// PlaceView is the place read and write response. Only the point geometry is
// exposed; places.approximate_area has no write surface, so a caller cannot put
// a polygon on the map through this API.
type PlaceView struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	NameAR        string   `json:"nameAr"`
	PlaceType     string   `json:"placeType"`
	ParentPlaceID string   `json:"parentPlaceId,omitempty"`
	NotesAR       string   `json:"notesAr,omitempty"`
	Longitude     *float64 `json:"longitude,omitempty"`
	Latitude      *float64 `json:"latitude,omitempty"`
	CreatedAt     string   `json:"createdAt,omitempty"`
	UpdatedAt     string   `json:"updatedAt,omitempty"`
}

// EntityRelationshipView is one generic typed relationship.
type EntityRelationshipView struct {
	ID          string `json:"id"`
	SubjectType string `json:"subjectType"`
	SubjectID   string `json:"subjectId"`
	Predicate   string `json:"predicate"`
	ObjectType  string `json:"objectType"`
	ObjectID    string `json:"objectId"`
	ValidFrom   string `json:"validFrom,omitempty"`
	ValidTo     string `json:"validTo,omitempty"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
}

// ListResponse is the envelope every list endpoint returns.
type ListResponse[T any] struct {
	Kind  string `json:"kind"`
	Query string `json:"query,omitempty"`
	Items []T    `json:"items"`
}
