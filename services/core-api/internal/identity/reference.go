package identity

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// This file holds the four reference families: families, tribes, branches and
// places.
//
// Two properties decide what this file is allowed to do, and both come from the
// same fact: none of these four tables has a version table or a draft state, so
// there is nothing in which an interpretation could be staged and then published.
//
//   - A created row is research-only. Since db/migrations/0039_reference_visibility
//     each of the four tables carries a visibility column, and every create here
//     binds 'private' explicitly as well as relying on the column default. The
//     central policy keeps a research-only row off the anonymous dictionary index,
//     the dictionary detail, the search index and the map. Publishing one is a
//     deliberate act against the row, not a side effect of writing it.
//   - Changing or removing a published row is refused with ErrPublishedReference.
//     A published row with no version behind it has nothing to supersede it, so an
//     edit would silently rewrite what the public index already serves and a
//     reader would have no way to see that it changed. Making a row public is the
//     supported path; editing one in place is not.
//
// People, person aliases and entity relationships are in people.go and
// relationships.go.

// ------------------------------------------------------------------ families --

func (s *Service) CreateFamily(ctx context.Context, actorID string, input CreateFamilyInput) (FamilyView, error) {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return FamilyView{}, err
	}
	name, description, originPlace, err := validateFamilyInput(input.CanonicalNameAR, input.DescriptionAR, input.OriginPlaceID)
	if err != nil {
		return FamilyView{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return FamilyView{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireIdentityWrite(ctx, tx, actorUUID); err != nil {
		return FamilyView{}, err
	}
	if originPlace != nil {
		if err := requireExistingRow(ctx, tx, "places", originPlace.(uuid.UUID)); err != nil {
			return FamilyView{}, err
		}
	}
	familyID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO families (id, canonical_name_ar, normalized_name_ar, description_ar, origin_place_id, created_by, visibility)
		VALUES ($1, $2, $3, $4, $5, $6, 'private')
	`, familyID, name, NormalizeArabicName(name), nullText(description), originPlace, actorUUID); err != nil {
		return FamilyView{}, mapWriteError(err)
	}
	view, err := readFamily(ctx, tx, familyID)
	if err != nil {
		return FamilyView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, actionFamilyCreated, entityFamily, familyID, nil, familySnapshot(view), ""); err != nil {
		return FamilyView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return FamilyView{}, err
	}
	return view, nil
}

func (s *Service) UpdateFamily(ctx context.Context, familyID, actorID string, input UpdateFamilyInput) (FamilyView, error) {
	return FamilyView{}, s.refusePublicReference(ctx, familyID, actorID, "family")
}

func (s *Service) DeleteFamily(ctx context.Context, familyID, actorID, reasonAR string) error {
	return s.refusePublicReference(ctx, familyID, actorID, "family")
}

func (s *Service) GetFamily(ctx context.Context, familyID, actorID string) (FamilyView, error) {
	if err := s.gateRead(ctx, actorID); err != nil {
		return FamilyView{}, err
	}
	familyUUID, err := parseID(familyID)
	if err != nil {
		return FamilyView{}, err
	}
	return readFamily(ctx, s.Pool, familyUUID)
}

func (s *Service) ListFamilies(ctx context.Context, actorID, search string) (ListResponse[FamilyView], error) {
	if err := s.gateRead(ctx, actorID); err != nil {
		return ListResponse[FamilyView]{}, err
	}
	term, err := validateListQuery(search)
	if err != nil {
		return ListResponse[FamilyView]{}, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT `+familyColumns+`
		FROM families f
		WHERE $1 = '' OR f.normalized_name_ar ILIKE '%' || $1 || '%'
		ORDER BY f.canonical_name_ar, f.id
		LIMIT $2
	`, term, defaultListLimit)
	if err != nil {
		return ListResponse[FamilyView]{}, err
	}
	defer rows.Close()
	items := make([]FamilyView, 0)
	for rows.Next() {
		item, err := scanFamily(rows)
		if err != nil {
			return ListResponse[FamilyView]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResponse[FamilyView]{}, err
	}
	return ListResponse[FamilyView]{Kind: "families", Query: term, Items: items}, nil
}

// -------------------------------------------------------------------- tribes --

func (s *Service) CreateTribe(ctx context.Context, actorID string, input CreateTribeInput) (TribeView, error) {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return TribeView{}, err
	}
	name, description, err := validateTribeInput(input.CanonicalNameAR, input.DescriptionAR)
	if err != nil {
		return TribeView{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return TribeView{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireIdentityWrite(ctx, tx, actorUUID); err != nil {
		return TribeView{}, err
	}
	tribeID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO tribes (id, canonical_name_ar, normalized_name_ar, description_ar, created_by, visibility)
		VALUES ($1, $2, $3, $4, $5, 'private')
	`, tribeID, name, NormalizeArabicName(name), nullText(description), actorUUID); err != nil {
		return TribeView{}, mapWriteError(err)
	}
	view, err := readTribe(ctx, tx, tribeID)
	if err != nil {
		return TribeView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, actionTribeCreated, entityTribe, tribeID, nil, tribeSnapshot(view), ""); err != nil {
		return TribeView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TribeView{}, err
	}
	return view, nil
}

func (s *Service) UpdateTribe(ctx context.Context, tribeID, actorID string, input UpdateTribeInput) (TribeView, error) {
	return TribeView{}, s.refusePublicReference(ctx, tribeID, actorID, "tribe")
}

func (s *Service) DeleteTribe(ctx context.Context, tribeID, actorID, reasonAR string) error {
	return s.refusePublicReference(ctx, tribeID, actorID, "tribe")
}

func (s *Service) GetTribe(ctx context.Context, tribeID, actorID string) (TribeView, error) {
	if err := s.gateRead(ctx, actorID); err != nil {
		return TribeView{}, err
	}
	tribeUUID, err := parseID(tribeID)
	if err != nil {
		return TribeView{}, err
	}
	return readTribe(ctx, s.Pool, tribeUUID)
}

func (s *Service) ListTribes(ctx context.Context, actorID, search string) (ListResponse[TribeView], error) {
	if err := s.gateRead(ctx, actorID); err != nil {
		return ListResponse[TribeView]{}, err
	}
	term, err := validateListQuery(search)
	if err != nil {
		return ListResponse[TribeView]{}, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT `+tribeColumns+`
		FROM tribes t
		WHERE $1 = '' OR t.normalized_name_ar ILIKE '%' || $1 || '%'
		ORDER BY t.canonical_name_ar, t.id
		LIMIT $2
	`, term, defaultListLimit)
	if err != nil {
		return ListResponse[TribeView]{}, err
	}
	defer rows.Close()
	items := make([]TribeView, 0)
	for rows.Next() {
		item, err := scanTribe(rows)
		if err != nil {
			return ListResponse[TribeView]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResponse[TribeView]{}, err
	}
	return ListResponse[TribeView]{Kind: "tribes", Query: term, Items: items}, nil
}

// ------------------------------------------------------------------ branches --

func (s *Service) CreateBranch(ctx context.Context, actorID string, input CreateBranchInput) (BranchView, error) {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return BranchView{}, err
	}
	familyID, err := requiredID(input.FamilyID)
	if err != nil {
		return BranchView{}, err
	}
	name, err := requiredName(input.CanonicalNameAR)
	if err != nil {
		return BranchView{}, err
	}
	notes, err := optionalText(input.NotesAR, maxNotesRunes)
	if err != nil {
		return BranchView{}, err
	}
	parentBranch, err := optionalID(input.ParentBranchID)
	if err != nil {
		return BranchView{}, err
	}
	validity, err := parseDateRange(input.ValidFrom, input.ValidTo)
	if err != nil {
		return BranchView{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return BranchView{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireIdentityWrite(ctx, tx, actorUUID); err != nil {
		return BranchView{}, err
	}
	if err := requireExistingRow(ctx, tx, "families", familyID); err != nil {
		return BranchView{}, err
	}
	if parentBranch != nil {
		parentID := parentBranch.(uuid.UUID)
		if err := requireExistingRow(ctx, tx, "branches", parentID); err != nil {
			return BranchView{}, err
		}
		// A branch cannot descend from a branch of another family: the family
		// column is what the dictionary groups by, so a cross-family parent would
		// make the same branch appear under two families.
		withinFamily, err := branchWithinFamily(ctx, tx, parentID, familyID)
		if err != nil {
			return BranchView{}, err
		}
		if !withinFamily {
			return BranchView{}, ErrValidation
		}
	}
	branchID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO branches (id, family_id, parent_branch_id, canonical_name_ar, normalized_name_ar, valid_from, valid_to, notes_ar, created_by, visibility)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'private')
	`, branchID, familyID, parentBranch, name, NormalizeArabicName(name), validity.From, validity.To, nullText(notes), actorUUID); err != nil {
		return BranchView{}, mapWriteError(err)
	}
	view, err := readBranch(ctx, tx, branchID)
	if err != nil {
		return BranchView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, actionBranchCreated, entityBranch, branchID, nil, branchSnapshot(view), ""); err != nil {
		return BranchView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return BranchView{}, err
	}
	return view, nil
}

func (s *Service) UpdateBranch(ctx context.Context, branchID, actorID string, input UpdateBranchInput) (BranchView, error) {
	return BranchView{}, s.refusePublicReference(ctx, branchID, actorID, "branch")
}

func (s *Service) DeleteBranch(ctx context.Context, branchID, actorID, reasonAR string) error {
	return s.refusePublicReference(ctx, branchID, actorID, "branch")
}

func (s *Service) GetBranch(ctx context.Context, branchID, actorID string) (BranchView, error) {
	if err := s.gateRead(ctx, actorID); err != nil {
		return BranchView{}, err
	}
	branchUUID, err := parseID(branchID)
	if err != nil {
		return BranchView{}, err
	}
	return readBranch(ctx, s.Pool, branchUUID)
}

func (s *Service) ListBranches(ctx context.Context, actorID, search string) (ListResponse[BranchView], error) {
	if err := s.gateRead(ctx, actorID); err != nil {
		return ListResponse[BranchView]{}, err
	}
	term, err := validateListQuery(search)
	if err != nil {
		return ListResponse[BranchView]{}, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT `+branchColumns+`
		FROM branches b
		WHERE $1 = '' OR b.normalized_name_ar ILIKE '%' || $1 || '%'
		ORDER BY b.canonical_name_ar, b.id
		LIMIT $2
	`, term, defaultListLimit)
	if err != nil {
		return ListResponse[BranchView]{}, err
	}
	defer rows.Close()
	items := make([]BranchView, 0)
	for rows.Next() {
		item, err := scanBranch(rows)
		if err != nil {
			return ListResponse[BranchView]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResponse[BranchView]{}, err
	}
	return ListResponse[BranchView]{Kind: "branches", Query: term, Items: items}, nil
}

// -------------------------------------------------------------------- places --

func (s *Service) CreatePlace(ctx context.Context, actorID string, input CreatePlaceInput) (PlaceView, error) {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return PlaceView{}, err
	}
	name, placeType, parentPlace, notes, longitude, latitude, err := validatePlaceInput(input.CanonicalNameAR, input.PlaceType, input.ParentPlaceID, input.NotesAR, input.Longitude, input.Latitude)
	if err != nil {
		return PlaceView{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return PlaceView{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireIdentityWrite(ctx, tx, actorUUID); err != nil {
		return PlaceView{}, err
	}
	if parentPlace != nil {
		if err := requireExistingRow(ctx, tx, "places", parentPlace.(uuid.UUID)); err != nil {
			return PlaceView{}, err
		}
	}
	placeID := uuid.New()
	// The point is built by PostGIS from two validated scalars rather than passed
	// as a geometry literal, so no caller string is ever parsed as spatial
	// syntax. places.approximate_area is left NULL: this API exposes a point, not
	// an area, and an unexposed column cannot be filled by mass assignment.
	if _, err := tx.Exec(ctx, `
		INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type, parent_place_id, notes_ar, geometry, created_by, visibility)
		VALUES ($1, $2, $3, $4, $5, $6,
		        CASE WHEN $7::float8 IS NULL THEN NULL ELSE ST_SetSRID(ST_MakePoint($7::float8, $8::float8), 4326) END,
		        $9, 'private')
	`, placeID, name, NormalizeArabicName(name), placeType, parentPlace, nullText(notes), longitude, latitude, actorUUID); err != nil {
		return PlaceView{}, mapWriteError(err)
	}
	view, err := readPlace(ctx, tx, placeID)
	if err != nil {
		return PlaceView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, actionPlaceCreated, entityPlace, placeID, nil, placeSnapshot(view), ""); err != nil {
		return PlaceView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PlaceView{}, err
	}
	return view, nil
}

func (s *Service) UpdatePlace(ctx context.Context, placeID, actorID string, input UpdatePlaceInput) (PlaceView, error) {
	return PlaceView{}, s.refusePublicReference(ctx, placeID, actorID, "place")
}

func (s *Service) DeletePlace(ctx context.Context, placeID, actorID, reasonAR string) error {
	return s.refusePublicReference(ctx, placeID, actorID, "place")
}

func (s *Service) GetPlace(ctx context.Context, placeID, actorID string) (PlaceView, error) {
	if err := s.gateRead(ctx, actorID); err != nil {
		return PlaceView{}, err
	}
	placeUUID, err := parseID(placeID)
	if err != nil {
		return PlaceView{}, err
	}
	return readPlace(ctx, s.Pool, placeUUID)
}

func (s *Service) ListPlaces(ctx context.Context, actorID, search string) (ListResponse[PlaceView], error) {
	if err := s.gateRead(ctx, actorID); err != nil {
		return ListResponse[PlaceView]{}, err
	}
	term, err := validateListQuery(search)
	if err != nil {
		return ListResponse[PlaceView]{}, err
	}
	rows, err := s.Pool.Query(ctx, `
		SELECT `+placeColumns+`
		FROM places p
		WHERE $1 = '' OR p.normalized_name_ar ILIKE '%' || $1 || '%'
		ORDER BY p.canonical_name_ar, p.id
		LIMIT $2
	`, term, defaultListLimit)
	if err != nil {
		return ListResponse[PlaceView]{}, err
	}
	defer rows.Close()
	items := make([]PlaceView, 0)
	for rows.Next() {
		item, err := scanPlace(rows)
		if err != nil {
			return ListResponse[PlaceView]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResponse[PlaceView]{}, err
	}
	return ListResponse[PlaceView]{Kind: "places", Query: term, Items: items}, nil
}

// ------------------------------------------------------------- shared pieces --

// refusePublicReference is the one refusal the four public families share. It
// takes the same gate as every other mutation first - so an unauthorized caller
// is told nothing about the row - and then answers with ErrPublicReference. No
// statement in it writes, so a refused update or delete leaves no audit event
// behind and holds no lock: the refusal is the absence of a write, not a
// recorded one.
func (s *Service) refusePublicReference(ctx context.Context, recordID, actorID, kind string) error {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return err
	}
	recordUUID, err := parseID(recordID)
	if err != nil {
		return err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := requireIdentityWrite(ctx, tx, actorUUID); err != nil {
		return err
	}
	if err := requireExistingRow(ctx, tx, referenceTable(kind), recordUUID); err != nil {
		return err
	}
	published, err := referenceIsPublished(ctx, tx, referenceTable(kind), recordUUID)
	if err != nil {
		return err
	}
	if !published {
		return ErrResearchReference
	}
	return ErrPublishedReference
}

func referenceIsPublished(ctx context.Context, q identityReader, table string, id uuid.UUID) (bool, error) {
	var published bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = $1 AND visibility = 'public')`, id).Scan(&published)
	return published, err
}

// gateRead is the read half of the identity gate. The four public families have
// no visibility policy of their own, so the gate on reading them is the same
// platform role the gate on writing them is: this package must not become a new
// anonymous read path for rows the dictionary already serves through its own
// endpoints.
func (s *Service) gateRead(ctx context.Context, actorID string) error {
	if err := s.ready(); err != nil {
		return err
	}
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return err
	}
	return requireIdentityWrite(ctx, s.Pool, actorUUID)
}

func referenceTable(kind string) string {
	switch kind {
	case "family":
		return "families"
	case "tribe":
		return "tribes"
	case "branch":
		return "branches"
	case "place":
		return "places"
	}
	return ""
}

// requireExistingRow refuses a reference that does not exist. The table name comes
// from a closed map inside this package, never from a request, and the value is
// bound as a parameter, so a caller cannot name a table or a column.
func requireExistingRow(ctx context.Context, q identityReader, table string, id uuid.UUID) error {
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+table+` WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}

func branchWithinFamily(ctx context.Context, q identityReader, branchID, familyID uuid.UUID) (bool, error) {
	var within bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM branches WHERE id = $1 AND family_id = $2)`, branchID, familyID).Scan(&within)
	return within, err
}

const familyColumns = `f.id, f.canonical_name_ar, f.description_ar, f.origin_place_id, f.created_at, f.updated_at`

func scanFamily(row pgx.Row) (FamilyView, error) {
	var view FamilyView
	var id, originPlace pgtype.UUID
	var description pgtype.Text
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &view.NameAR, &description, &originPlace, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return FamilyView{}, ErrNotFound
		}
		return FamilyView{}, err
	}
	view.ID = uuidValue(id)
	view.Kind = entityFamily
	view.DescriptionAR = textValue(description)
	view.OriginPlaceID = uuidValue(originPlace)
	view.CreatedAt = timestampValue(createdAt)
	view.UpdatedAt = timestampValue(updatedAt)
	return view, nil
}

func readFamily(ctx context.Context, q identityReader, familyID uuid.UUID) (FamilyView, error) {
	return scanFamily(q.QueryRow(ctx, `SELECT `+familyColumns+` FROM families f WHERE f.id = $1`, familyID))
}

const tribeColumns = `t.id, t.canonical_name_ar, t.description_ar, t.created_at, t.updated_at`

func scanTribe(row pgx.Row) (TribeView, error) {
	var view TribeView
	var id pgtype.UUID
	var description pgtype.Text
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &view.NameAR, &description, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TribeView{}, ErrNotFound
		}
		return TribeView{}, err
	}
	view.ID = uuidValue(id)
	view.Kind = entityTribe
	view.DescriptionAR = textValue(description)
	view.CreatedAt = timestampValue(createdAt)
	view.UpdatedAt = timestampValue(updatedAt)
	return view, nil
}

func readTribe(ctx context.Context, q identityReader, tribeID uuid.UUID) (TribeView, error) {
	return scanTribe(q.QueryRow(ctx, `SELECT `+tribeColumns+` FROM tribes t WHERE t.id = $1`, tribeID))
}

const branchColumns = `b.id, b.family_id, b.parent_branch_id, b.canonical_name_ar, b.valid_from, b.valid_to, b.notes_ar, b.created_at, b.updated_at`

func scanBranch(row pgx.Row) (BranchView, error) {
	var view BranchView
	var id, familyID, parentBranch pgtype.UUID
	var validFrom, validTo pgtype.Date
	var notes pgtype.Text
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &familyID, &parentBranch, &view.NameAR, &validFrom, &validTo, &notes, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return BranchView{}, ErrNotFound
		}
		return BranchView{}, err
	}
	view.ID = uuidValue(id)
	view.Kind = entityBranch
	view.FamilyID = uuidValue(familyID)
	view.ParentBranchID = uuidValue(parentBranch)
	view.ValidFrom = dateValue(validFrom)
	view.ValidTo = dateValue(validTo)
	view.NotesAR = textValue(notes)
	view.CreatedAt = timestampValue(createdAt)
	view.UpdatedAt = timestampValue(updatedAt)
	return view, nil
}

func readBranch(ctx context.Context, q identityReader, branchID uuid.UUID) (BranchView, error) {
	return scanBranch(q.QueryRow(ctx, `SELECT `+branchColumns+` FROM branches b WHERE b.id = $1`, branchID))
}

const placeColumns = `p.id, p.canonical_name_ar, p.place_type, p.parent_place_id, p.notes_ar, ST_X(p.geometry), ST_Y(p.geometry), p.created_at, p.updated_at`

func scanPlace(row pgx.Row) (PlaceView, error) {
	var view PlaceView
	var id, parentPlace pgtype.UUID
	var notes pgtype.Text
	var longitude, latitude pgtype.Float8
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &view.NameAR, &view.PlaceType, &parentPlace, &notes, &longitude, &latitude, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PlaceView{}, ErrNotFound
		}
		return PlaceView{}, err
	}
	view.ID = uuidValue(id)
	view.Kind = entityPlace
	view.ParentPlaceID = uuidValue(parentPlace)
	view.NotesAR = textValue(notes)
	if value, ok := floatValue(longitude); ok {
		view.Longitude = &value
	}
	if value, ok := floatValue(latitude); ok {
		view.Latitude = &value
	}
	view.CreatedAt = timestampValue(createdAt)
	view.UpdatedAt = timestampValue(updatedAt)
	return view, nil
}

func readPlace(ctx context.Context, q identityReader, placeID uuid.UUID) (PlaceView, error) {
	return scanPlace(q.QueryRow(ctx, `SELECT `+placeColumns+` FROM places p WHERE p.id = $1`, placeID))
}

func validateFamilyInput(name, description, originPlaceID string) (string, string, any, error) {
	trimmedName, err := requiredName(name)
	if err != nil {
		return "", "", nil, err
	}
	trimmedDescription, err := optionalText(description, maxNotesRunes)
	if err != nil {
		return "", "", nil, err
	}
	originPlace, err := optionalID(originPlaceID)
	if err != nil {
		return "", "", nil, err
	}
	return trimmedName, trimmedDescription, originPlace, nil
}

func validateTribeInput(name, description string) (string, string, error) {
	trimmedName, err := requiredName(name)
	if err != nil {
		return "", "", err
	}
	trimmedDescription, err := optionalText(description, maxNotesRunes)
	if err != nil {
		return "", "", err
	}
	return trimmedName, trimmedDescription, nil
}

// validatePlaceInput resolves the place fields. The two coordinates are validated
// as a pair and returned as the caller's own pointers, so an absent point stays
// an absent point all the way into the statement instead of becoming the origin.
func validatePlaceInput(name, placeType, parentPlaceID, notes string, longitude, latitude *float64) (string, string, any, string, *float64, *float64, error) {
	trimmedName, err := requiredName(name)
	if err != nil {
		return "", "", nil, "", nil, nil, err
	}
	resolvedType, err := requiredEnum(placeType, placeTypes)
	if err != nil {
		return "", "", nil, "", nil, nil, err
	}
	parentPlace, err := optionalID(parentPlaceID)
	if err != nil {
		return "", "", nil, "", nil, nil, err
	}
	trimmedNotes, err := optionalText(notes, maxNotesRunes)
	if err != nil {
		return "", "", nil, "", nil, nil, err
	}
	if err := validatePoint(longitude, latitude); err != nil {
		return "", "", nil, "", nil, nil, err
	}
	return trimmedName, resolvedType, parentPlace, trimmedNotes, longitude, latitude, nil
}

func familySnapshot(view FamilyView) map[string]any {
	return map[string]any{
		"canonical_name_ar": view.NameAR,
		"description_ar":    view.DescriptionAR,
		"origin_place_id":   view.OriginPlaceID,
	}
}

func tribeSnapshot(view TribeView) map[string]any {
	return map[string]any{
		"canonical_name_ar": view.NameAR,
		"description_ar":    view.DescriptionAR,
	}
}

func branchSnapshot(view BranchView) map[string]any {
	return map[string]any{
		"family_id":         view.FamilyID,
		"parent_branch_id":  view.ParentBranchID,
		"canonical_name_ar": view.NameAR,
		"valid_from":        view.ValidFrom,
		"valid_to":          view.ValidTo,
		"notes_ar":          view.NotesAR,
	}
}

func placeSnapshot(view PlaceView) map[string]any {
	return map[string]any{
		"canonical_name_ar": view.NameAR,
		"place_type":        view.PlaceType,
		"parent_place_id":   view.ParentPlaceID,
		"notes_ar":          view.NotesAR,
	}
}
