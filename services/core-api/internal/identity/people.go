package identity

import (
	"context"
	"errors"
	"strings"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// identityReader is the read surface this package needs. Both *pgxpool.Pool and
// pgx.Tx satisfy it, so a read can be resolved inside or outside a transaction.
type identityReader interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// CreatePerson records a new person.
//
// The row is research-only by construction and this function is where that is
// guaranteed: it writes no tree version, so the person belongs to no published
// interpretation and plan 001 keeps it invisible to an anonymous reader; it
// binds no identity_status, so the person starts unreviewed and cannot arrive
// pre-approved; and it binds no merged_into_id, so nothing of the merge
// machinery - the one automatic identity operation the product has - is reachable
// from here. The audit event shares the transaction.
func (s *Service) CreatePerson(ctx context.Context, actorID string, input CreatePersonInput) (PersonView, error) {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return PersonView{}, err
	}
	fields, err := validatePersonInput(input.CanonicalNameAR, input.Gender, input.BirthDateFrom, input.BirthDateTo, input.DeathDateFrom, input.DeathDateTo, input.NotesAR)
	if err != nil {
		return PersonView{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return PersonView{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireIdentityWrite(ctx, tx, actorUUID); err != nil {
		return PersonView{}, err
	}
	personID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO people (id, canonical_name_ar, normalized_name_ar, gender, birth_date_from, birth_date_to, death_date_from, death_date_to, notes_ar, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, personID, fields.name, NormalizeArabicName(fields.name), fields.gender, fields.dates.birthFrom, fields.dates.birthTo, fields.dates.deathFrom, fields.dates.deathTo, nullText(fields.notes), actorUUID); err != nil {
		return PersonView{}, mapWriteError(err)
	}
	view, err := readPerson(ctx, tx, personID)
	if err != nil {
		return PersonView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, actionPersonCreated, entityPerson, personID, nil, personSnapshot(view), ""); err != nil {
		return PersonView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PersonView{}, err
	}
	return view, nil
}

// UpdatePerson changes the editable columns of a person.
//
// A person that appears in a published tree version is refused: the published
// interpretation already rests on this row, and rewriting it would change what
// every reader of that version sees without a new version being published. A
// person an audited merge has already frozen is refused for the same reason, one
// step earlier in the history.
func (s *Service) UpdatePerson(ctx context.Context, personID, actorID string, input UpdatePersonInput) (PersonView, error) {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return PersonView{}, err
	}
	personUUID, err := parseID(personID)
	if err != nil {
		return PersonView{}, err
	}
	fields, err := validatePersonInput(input.CanonicalNameAR, input.Gender, input.BirthDateFrom, input.BirthDateTo, input.DeathDateFrom, input.DeathDateTo, input.NotesAR)
	if err != nil {
		return PersonView{}, err
	}
	reason, err := optionalText(input.ReasonAR, maxReasonRunes)
	if err != nil {
		return PersonView{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return PersonView{}, err
	}
	defer tx.Rollback(ctx)
	if err := requireIdentityWrite(ctx, tx, actorUUID); err != nil {
		return PersonView{}, err
	}
	before, merged, err := lockPerson(ctx, tx, personUUID)
	if err != nil {
		return PersonView{}, err
	}
	if merged {
		return PersonView{}, ErrMergedIdentity
	}
	if err := refusePublishedPerson(ctx, tx, personUUID); err != nil {
		return PersonView{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE people
		SET canonical_name_ar = $1, normalized_name_ar = $2, gender = $3,
		    birth_date_from = $4, birth_date_to = $5, death_date_from = $6, death_date_to = $7,
		    notes_ar = $8, updated_at = now()
		WHERE id = $9
	`, fields.name, NormalizeArabicName(fields.name), fields.gender, fields.dates.birthFrom, fields.dates.birthTo, fields.dates.deathFrom, fields.dates.deathTo, nullText(fields.notes), personUUID); err != nil {
		return PersonView{}, mapWriteError(err)
	}
	view, err := readPerson(ctx, tx, personUUID)
	if err != nil {
		return PersonView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, actionPersonUpdated, entityPerson, personUUID, personSnapshot(before), personSnapshot(view), reason); err != nil {
		return PersonView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PersonView{}, err
	}
	return view, nil
}

// DeletePerson removes a person that no tree version holds a node for.
//
// The refusal is deliberately wider than the published check: a person that is
// only in a draft is still a node an interpretation is built from, and deleting
// the identity row underneath would leave the node with nothing to point at.
// Removing a person from an interpretation is an edit of that tree, not of this
// API.
func (s *Service) DeletePerson(ctx context.Context, personID, actorID, reasonAR string) error {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return err
	}
	personUUID, err := parseID(personID)
	if err != nil {
		return err
	}
	reason, err := optionalText(reasonAR, maxReasonRunes)
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
	before, merged, err := lockPerson(ctx, tx, personUUID)
	if err != nil {
		return err
	}
	if merged {
		return ErrMergedIdentity
	}
	referenced, err := personInAnyTreeVersion(ctx, tx, personUUID)
	if err != nil {
		return err
	}
	if referenced {
		return ErrReferencedByInterpretation
	}
	if _, err := tx.Exec(ctx, `DELETE FROM people WHERE id = $1`, personUUID); err != nil {
		return mapWriteError(err)
	}
	return commitWithAudit(ctx, tx, actorUUID, actionPersonDeleted, entityPerson, personUUID, personSnapshot(before), nil, reason)
}

// GetPerson reads one person through the central visibility policy. A person the
// policy hides answers exactly like a person that does not exist, so this
// endpoint cannot be used to discover which people exist.
func (s *Service) GetPerson(ctx context.Context, personID, actorID string) (PersonView, error) {
	_, actorUUID, policy, err := s.scopedRead(ctx, actorID)
	if err != nil {
		return PersonView{}, err
	}
	parsed, err := parseID(personID)
	if err != nil {
		return PersonView{}, err
	}
	if err := requireIdentityWrite(ctx, s.Pool, actorUUID); err != nil {
		return PersonView{}, err
	}
	access, err := policy.Person(ctx, s.Pool, parsed)
	if err != nil {
		return PersonView{}, err
	}
	if !access.Allowed() {
		return PersonView{}, ErrNotFound
	}
	return readPerson(ctx, s.Pool, parsed)
}

// ListPeople lists the people the central visibility policy lets this actor read.
// The list predicate is the same expression the dictionary index and the search
// service use, so the three can never disagree about who may see a person. The
// endpoint is gated on the write role as well, so it adds no public read path.
func (s *Service) ListPeople(ctx context.Context, actorID, search string) (ListResponse[PersonView], error) {
	_, actorUUID, policy, err := s.scopedRead(ctx, actorID)
	if err != nil {
		return ListResponse[PersonView]{}, err
	}
	term, err := validateListQuery(search)
	if err != nil {
		return ListResponse[PersonView]{}, err
	}
	if err := requireIdentityWrite(ctx, s.Pool, actorUUID); err != nil {
		return ListResponse[PersonView]{}, err
	}
	params := visibility.NewParams()
	termRef := params.Add(term)
	limitRef := params.Add(defaultListLimit)
	rows, err := s.Pool.Query(ctx, `
		SELECT `+personColumns+`
		FROM people p
		WHERE `+policy.PersonPredicate(params, "p.id")+`
		  AND (`+termRef+` = '' OR p.normalized_name_ar ILIKE '%' || `+termRef+` || '%'
		       OR EXISTS (SELECT 1 FROM person_aliases pa WHERE pa.person_id = p.id AND pa.normalized_value_ar ILIKE '%' || `+termRef+` || '%'))
		ORDER BY p.canonical_name_ar, p.id
		LIMIT `+limitRef, params.Args()...)
	if err != nil {
		return ListResponse[PersonView]{}, err
	}
	defer rows.Close()
	items := make([]PersonView, 0)
	for rows.Next() {
		item, _, err := scanPersonRow(rows)
		if err != nil {
			return ListResponse[PersonView]{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return ListResponse[PersonView]{}, err
	}
	return ListResponse[PersonView]{Kind: "people", Query: term, Items: items}, nil
}

// ListPersonAliases lists the aliases of a person the policy lets this actor read.
// Each alias follows the rule plan 001 applies everywhere else: an alias taken from
// a research-only source is not shown to an actor who cannot read that source, and
// an alias with no source is a platform record and stays.
//
// The gate is the person policy and nothing more, not the identity write role: the
// alias write reaches the tree owner and the tree collaborator, so the list has to
// as well or they could record a name they cannot read back. An alias is exactly
// what the dictionary already shows for a person the policy lets through, so this
// adds no data to anyone's reach.
func (s *Service) ListPersonAliases(ctx context.Context, personID, actorID string) (ListResponse[PersonAliasView], error) {
	_, _, policy, err := s.scopedRead(ctx, actorID)
	if err != nil {
		return ListResponse[PersonAliasView]{}, err
	}
	personUUID, err := parseID(personID)
	if err != nil {
		return ListResponse[PersonAliasView]{}, err
	}
	access, err := policy.Person(ctx, s.Pool, personUUID)
	if err != nil {
		return ListResponse[PersonAliasView]{}, err
	}
	if !access.Allowed() {
		return ListResponse[PersonAliasView]{}, ErrNotFound
	}
	params := visibility.NewParams()
	personRef := params.Add(personUUID)
	rows, err := s.Pool.Query(ctx, `
		SELECT pa.id, pa.person_id, pa.value_ar, pa.alias_type, pa.source_id, pa.created_at
		FROM person_aliases pa
		WHERE pa.person_id = `+personRef+`
		  AND (pa.source_id IS NULL OR `+policy.SourcePredicate(params, "pa.source_id")+`)
		ORDER BY pa.created_at, pa.id
	`, params.Args()...)
	if err != nil {
		return ListResponse[PersonAliasView]{}, err
	}
	defer rows.Close()
	items := make([]PersonAliasView, 0)
	for rows.Next() {
		var view PersonAliasView
		var id, aliasPersonID, sourceID pgtype.UUID
		var createdAt pgtype.Timestamptz
		if err := rows.Scan(&id, &aliasPersonID, &view.ValueAR, &view.AliasType, &sourceID, &createdAt); err != nil {
			return ListResponse[PersonAliasView]{}, err
		}
		view.ID = uuidValue(id)
		view.PersonID = uuidValue(aliasPersonID)
		view.SourceID = uuidValue(sourceID)
		view.CreatedAt = timestampValue(createdAt)
		items = append(items, view)
	}
	if err := rows.Err(); err != nil {
		return ListResponse[PersonAliasView]{}, err
	}
	return ListResponse[PersonAliasView]{Kind: "person-aliases", Items: items}, nil
}

// CreatePersonAlias records another name for a person.
//
// Authorization is the one in authorizeAliasWrite: the identity write role, or the
// tree-scoped right trees.AddPerson already grants, over a person the actor can
// read. The published check then refuses a person that a published tree version
// rests on, because its dictionary page is public and a new alias would appear
// there the moment it landed. A source_id may only name a source the actor can
// read, so a spelling cannot be attached to a source its own author is not allowed
// to see; an alias with no source is a platform record and is visible to every
// reader of a public person, which is why the choice is explicit rather than
// defaulted.
func (s *Service) CreatePersonAlias(ctx context.Context, personID, actorID string, input PersonAliasInput) (PersonAliasView, error) {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return PersonAliasView{}, err
	}
	personUUID, err := parseID(personID)
	if err != nil {
		return PersonAliasView{}, err
	}
	value, aliasType, source, reason, err := validateAliasInput(input)
	if err != nil {
		return PersonAliasView{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return PersonAliasView{}, err
	}
	defer tx.Rollback(ctx)
	if err := authorizeAliasWrite(ctx, tx, actorUUID, personUUID); err != nil {
		return PersonAliasView{}, err
	}
	if _, _, err := lockPerson(ctx, tx, personUUID); err != nil {
		return PersonAliasView{}, err
	}
	if err := refusePublishedPerson(ctx, tx, personUUID); err != nil {
		return PersonAliasView{}, err
	}
	if source != nil {
		if err := requireReadableSource(ctx, tx, actorID, source.(uuid.UUID)); err != nil {
			return PersonAliasView{}, err
		}
	}
	aliasID := uuid.New()
	if _, err := tx.Exec(ctx, `
		INSERT INTO person_aliases (id, person_id, value_ar, normalized_value_ar, alias_type, source_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, aliasID, personUUID, value, NormalizeArabicName(value), aliasType, source); err != nil {
		return PersonAliasView{}, mapWriteError(err)
	}
	view, err := readAlias(ctx, tx, aliasID)
	if err != nil {
		return PersonAliasView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, actionPersonAliasCreated, entityPersonAlias, aliasID, nil, aliasSnapshot(view), reason); err != nil {
		return PersonAliasView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PersonAliasView{}, err
	}
	return view, nil
}

// UpdatePersonAlias changes the spelling, the type or the source of an alias. The
// published check applies exactly as on create, because the alias is shown on the
// public dictionary page of a person a published version already published.
func (s *Service) UpdatePersonAlias(ctx context.Context, aliasID, actorID string, input PersonAliasInput) (PersonAliasView, error) {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return PersonAliasView{}, err
	}
	aliasUUID, err := parseID(aliasID)
	if err != nil {
		return PersonAliasView{}, err
	}
	value, aliasType, source, reason, err := validateAliasInput(input)
	if err != nil {
		return PersonAliasView{}, err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return PersonAliasView{}, err
	}
	defer tx.Rollback(ctx)
	// The person is resolved before the row is locked, so the authorization
	// decision - and not the existence of the alias - is what a denied actor meets
	// first. The lock then re-reads the row under the same transaction.
	personID, err := aliasOwner(ctx, tx, aliasUUID)
	if err != nil {
		return PersonAliasView{}, err
	}
	if err := authorizeAliasWrite(ctx, tx, actorUUID, personID); err != nil {
		return PersonAliasView{}, err
	}
	if err := refusePublishedPerson(ctx, tx, personID); err != nil {
		return PersonAliasView{}, err
	}
	before, _, err := lockAlias(ctx, tx, aliasUUID)
	if err != nil {
		return PersonAliasView{}, err
	}
	if source != nil {
		if err := requireReadableSource(ctx, tx, actorID, source.(uuid.UUID)); err != nil {
			return PersonAliasView{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE person_aliases
		SET value_ar = $1, normalized_value_ar = $2, alias_type = $3, source_id = $4
		WHERE id = $5
	`, value, NormalizeArabicName(value), aliasType, source, aliasUUID); err != nil {
		return PersonAliasView{}, mapWriteError(err)
	}
	view, err := readAlias(ctx, tx, aliasUUID)
	if err != nil {
		return PersonAliasView{}, err
	}
	if err := writeAudit(ctx, tx, actorUUID, actionPersonAliasUpdated, entityPersonAlias, aliasUUID, aliasSnapshot(before), aliasSnapshot(view), reason); err != nil {
		return PersonAliasView{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PersonAliasView{}, err
	}
	return view, nil
}

// DeletePersonAlias removes one alias. It carries the same published refusal as
// the other alias mutations, because removing an alias also changes what a public
// dictionary page shows.
func (s *Service) DeletePersonAlias(ctx context.Context, aliasID, actorID, reasonAR string) error {
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return err
	}
	aliasUUID, err := parseID(aliasID)
	if err != nil {
		return err
	}
	reason, err := optionalText(reasonAR, maxReasonRunes)
	if err != nil {
		return err
	}
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	personID, err := aliasOwner(ctx, tx, aliasUUID)
	if err != nil {
		return err
	}
	if err := authorizeAliasWrite(ctx, tx, actorUUID, personID); err != nil {
		return err
	}
	if err := refusePublishedPerson(ctx, tx, personID); err != nil {
		return err
	}
	before, _, err := lockAlias(ctx, tx, aliasUUID)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM person_aliases WHERE id = $1`, aliasUUID); err != nil {
		return mapWriteError(err)
	}
	return commitWithAudit(ctx, tx, actorUUID, actionPersonAliasDeleted, entityPersonAlias, aliasUUID, aliasSnapshot(before), nil, reason)
}

// scopedRead resolves the acting user and the visibility policy for a read. The
// policy is the one internal/visibility builds, never a second decision written
// here, so this package cannot answer "may this actor read this row" differently
// from the dictionary, the search service or the claim reads.
func (s *Service) scopedRead(ctx context.Context, actorID string) (string, uuid.UUID, visibility.Policy, error) {
	if err := s.ready(); err != nil {
		return "", uuid.Nil, visibility.Policy{}, err
	}
	actorUUID, err := parseActor(actorID)
	if err != nil {
		return "", uuid.Nil, visibility.Policy{}, err
	}
	policy, err := visibility.Load(ctx, s.Pool, actorID)
	if err != nil {
		return "", uuid.Nil, visibility.Policy{}, err
	}
	return actorID, actorUUID, policy, nil
}

// -------------------------------------------------------------- shared reads -

const personColumns = `p.id, p.canonical_name_ar, p.gender, p.identity_status, p.birth_date_from, p.birth_date_to, p.death_date_from, p.death_date_to, p.notes_ar, p.created_at, p.updated_at, p.merged_into_id`

func scanPersonRow(row pgx.Row) (PersonView, pgtype.UUID, error) {
	var item PersonView
	var id, mergedInto pgtype.UUID
	var gender pgtype.Text
	var birthFrom, birthTo, deathFrom, deathTo pgtype.Date
	var notes pgtype.Text
	var createdAt, updatedAt pgtype.Timestamptz
	if err := row.Scan(&id, &item.NameAR, &gender, &item.IdentityStatus, &birthFrom, &birthTo, &deathFrom, &deathTo, &notes, &createdAt, &updatedAt, &mergedInto); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PersonView{}, pgtype.UUID{}, ErrNotFound
		}
		return PersonView{}, pgtype.UUID{}, err
	}
	item.ID = uuidValue(id)
	item.Kind = entityPerson
	// people.gender is nullable in the schema because the seed writes it sparsely.
	// The API answers "unknown" rather than an empty string, so a caller never has
	// to tell "not recorded" apart from "not returned".
	item.Gender = "unknown"
	if gender.Valid && gender.String != "" {
		item.Gender = gender.String
	}
	item.BirthDateFrom = dateValue(birthFrom)
	item.BirthDateTo = dateValue(birthTo)
	item.DeathDateFrom = dateValue(deathFrom)
	item.DeathDateTo = dateValue(deathTo)
	item.NotesAR = textValue(notes)
	item.CreatedAt = timestampValue(createdAt)
	item.UpdatedAt = timestampValue(updatedAt)
	return item, mergedInto, nil
}

func readPerson(ctx context.Context, q identityReader, personID uuid.UUID) (PersonView, error) {
	item, _, err := scanPersonRow(q.QueryRow(ctx, `SELECT `+personColumns+` FROM people p WHERE p.id = $1`, personID))
	return item, err
}

// lockPerson takes the row lock an update or a delete needs and reports whether
// the row has been merged away. The lock is taken before the published check so
// the decision and the write see the same row.
func lockPerson(ctx context.Context, tx pgx.Tx, personID uuid.UUID) (PersonView, bool, error) {
	item, mergedInto, err := scanPersonRow(tx.QueryRow(ctx, `SELECT `+personColumns+` FROM people p WHERE p.id = $1 FOR UPDATE`, personID))
	if err != nil {
		return PersonView{}, false, err
	}
	return item, mergedInto.Valid, nil
}

// refusePublishedPerson is the one published-interpretation check for people and
// the aliases that hang off them. It is the same shape as plan 001's person grant
// - a node in a published version of a tree - so the write surface and the read
// surface agree on what published means. The tree's own visibility is deliberately
// not part of the test: a published version of a private tree is still a published
// interpretation its readers have already seen.
func refusePublishedPerson(ctx context.Context, q identityReader, personID uuid.UUID) error {
	var published bool
	if err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM tree_nodes vis_node
			JOIN tree_versions vis_version ON vis_version.id = vis_node.tree_version_id
			WHERE vis_node.person_id = $1 AND vis_version.state = 'published'
		)
	`, personID).Scan(&published); err != nil {
		return err
	}
	if published {
		return ErrPublishedInterpretation
	}
	return nil
}

// authorizeAliasWrite is the authorization gate for a person alias, and it is
// deliberately wider than the gate on every other identity write.
//
// An alias is the one global identity record a researcher reaches for from inside a
// tree draft - the person panel in the workspace has no other field - so a gate that
// only a platform role can pass leaves the tree owner and the tree collaborator with
// a form that always fails. The rule therefore mirrors how trees.AddPerson
// authorizes: the actor may write an alias for a person when they hold the identity
// write role, or when they own a tree whose current draft holds that person, or when
// they are an edit-level collaborator on such a tree.
//
// Three things still hold, and each is a separate refusal:
//
//   - A person the actor cannot read answers exactly like a person that does not
//     exist, so a tree-scoped right cannot be aimed at somebody else's private
//     research record. This is the same Person grant the dictionary uses, asked the
//     same way.
//   - A person in a published tree version is still refused with
//     ErrPublishedInterpretation: the alias would appear on a public dictionary page
//     the moment it landed.
//   - An actor with neither right gets ErrForbidden, which says nothing about the
//     row.
//
// The whole decision is taken inside the caller's transaction, before the first
// write, from the same role and collaborator rows every other decision reads.
func authorizeAliasWrite(ctx context.Context, tx pgx.Tx, actorID, personID uuid.UUID) error {
	if err := requireIdentityWrite(ctx, tx, actorID); err == nil {
		return requireReadablePerson(ctx, tx, actorID.String(), personID)
	} else if !errors.Is(err, ErrForbidden) {
		return err
	}
	editable, err := canEditDraftHoldingPerson(ctx, tx, actorID, personID)
	if err != nil {
		return err
	}
	if !editable {
		return ErrForbidden
	}
	return requireReadablePerson(ctx, tx, actorID.String(), personID)
}

// canEditDraftHoldingPerson mirrors trees.canEditDraftTx for one person: the tree's
// owner, or an edit-level collaborator on it, and the person has to be a node in that
// tree's current draft. A person that is only in a published version is not in a
// draft, so a published tree gives its owner no alias right at all - the published
// check refuses that case anyway.
func canEditDraftHoldingPerson(ctx context.Context, q identityReader, actorID, personID uuid.UUID) (bool, error) {
	var allowed bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM trees vis_tree
			JOIN tree_versions vis_version ON vis_version.tree_id = vis_tree.id AND vis_version.state = 'draft'
			JOIN tree_nodes vis_node ON vis_node.tree_version_id = vis_version.id
			WHERE vis_node.person_id = $1
			  AND (
			    vis_tree.owner_id = $2
			    OR EXISTS (
			      SELECT 1 FROM tree_collaborators vis_collaborator
			      WHERE vis_collaborator.tree_id = vis_tree.id
			        AND vis_collaborator.user_id = $2
			        AND vis_collaborator.permission_level = 'edit'
			    )
			  )
		)
	`, personID, actorID).Scan(&allowed)
	return allowed, err
}

// requireReadablePerson asks the central person policy about one person inside the
// caller's transaction. A hidden or missing person answers the same way, so the
// refusal cannot be used to find out which people exist.
func requireReadablePerson(ctx context.Context, q identityReader, actorID string, personID uuid.UUID) error {
	policy, err := visibility.Load(ctx, q, actorID)
	if err != nil {
		return err
	}
	access, err := policy.Person(ctx, q, personID)
	if err != nil {
		return err
	}
	if !access.Allowed() {
		return ErrNotFound
	}
	return nil
}

func personInAnyTreeVersion(ctx context.Context, q identityReader, personID uuid.UUID) (bool, error) {
	var referenced bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tree_nodes vis_node WHERE vis_node.person_id = $1)`, personID).Scan(&referenced)
	return referenced, err
}

// aliasOwner resolves the person an alias belongs to without taking a lock, so the
// authorization decision can be taken before the row is locked for the write.
func aliasOwner(ctx context.Context, q identityReader, aliasID uuid.UUID) (uuid.UUID, error) {
	var personID pgtype.UUID
	if err := q.QueryRow(ctx, `SELECT person_id FROM person_aliases WHERE id = $1`, aliasID).Scan(&personID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrNotFound
		}
		return uuid.Nil, err
	}
	return uuidUUID(personID), nil
}

func lockAlias(ctx context.Context, tx pgx.Tx, aliasID uuid.UUID) (PersonAliasView, uuid.UUID, error) {
	view, personID, err := scanAlias(tx.QueryRow(ctx, aliasSelect+` WHERE id = $1 FOR UPDATE`, aliasID))
	if err != nil {
		return PersonAliasView{}, uuid.Nil, err
	}
	return view, personID, nil
}

func readAlias(ctx context.Context, q identityReader, aliasID uuid.UUID) (PersonAliasView, error) {
	view, _, err := scanAlias(q.QueryRow(ctx, aliasSelect+` WHERE id = $1`, aliasID))
	return view, err
}

const aliasSelect = `SELECT id, person_id, value_ar, alias_type, source_id, created_at FROM person_aliases`

func scanAlias(row pgx.Row) (PersonAliasView, uuid.UUID, error) {
	var view PersonAliasView
	var id, personID, sourceID pgtype.UUID
	var createdAt pgtype.Timestamptz
	if err := row.Scan(&id, &personID, &view.ValueAR, &view.AliasType, &sourceID, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PersonAliasView{}, uuid.Nil, ErrNotFound
		}
		return PersonAliasView{}, uuid.Nil, err
	}
	view.ID = uuidValue(id)
	view.PersonID = uuidValue(personID)
	view.SourceID = uuidValue(sourceID)
	view.CreatedAt = timestampValue(createdAt)
	return view, uuidUUID(personID), nil
}

// requireReadableSource refuses a source the actor may not read. An alias naming a
// research-only source is scoped by that source everywhere it is read, so the
// write starts from the same question: may this actor see what it points at.
func requireReadableSource(ctx context.Context, q identityReader, actorID string, sourceID uuid.UUID) error {
	policy, err := visibility.Load(ctx, q, actorID)
	if err != nil {
		return err
	}
	access, err := policy.Source(ctx, q, sourceID)
	if err != nil {
		return err
	}
	if !access.Allowed() {
		return ErrNotFound
	}
	return nil
}

// ----------------------------------------------------------------- validation -

type personDates struct {
	birthFrom pgtype.Date
	birthTo   pgtype.Date
	deathFrom pgtype.Date
	deathTo   pgtype.Date
}

type personFields struct {
	name   string
	gender string
	notes  string
	dates  personDates
}

func validatePersonInput(name, gender, birthFrom, birthTo, deathFrom, deathTo, notes string) (personFields, error) {
	trimmedName, err := requiredName(name)
	if err != nil {
		return personFields{}, err
	}
	resolvedGender, err := optionalEnum(gender, "unknown", personGenders)
	if err != nil {
		return personFields{}, err
	}
	trimmedNotes, err := optionalText(notes, maxNotesRunes)
	if err != nil {
		return personFields{}, err
	}
	birth, err := parseDateRange(birthFrom, birthTo)
	if err != nil {
		return personFields{}, err
	}
	death, err := parseDateRange(deathFrom, deathTo)
	if err != nil {
		return personFields{}, err
	}
	return personFields{
		name:   trimmedName,
		gender: resolvedGender,
		notes:  trimmedNotes,
		dates:  personDates{birthFrom: birth.From, birthTo: birth.To, deathFrom: death.From, deathTo: death.To},
	}, nil
}

func validateAliasInput(input PersonAliasInput) (string, string, any, string, error) {
	value, err := requiredName(input.ValueAR)
	if err != nil {
		return "", "", nil, "", err
	}
	if len([]rune(value)) > maxAliasRunes {
		return "", "", nil, "", ErrValidation
	}
	aliasType, err := optionalEnum(input.AliasType, "alternative_name", aliasTypes)
	if err != nil {
		return "", "", nil, "", err
	}
	source, err := optionalID(input.SourceID)
	if err != nil {
		return "", "", nil, "", err
	}
	reason, err := optionalText(input.ReasonAR, maxReasonRunes)
	if err != nil {
		return "", "", nil, "", err
	}
	return value, aliasType, source, reason, nil
}

// validateListQuery normalizes the shared list filter. The search term goes
// through the same Arabic normalizer the stored columns are built from, so a
// query for "أبو بكر" finds a row stored as "ابو بكر".
func validateListQuery(search string) (string, error) {
	term := NormalizeArabicName(strings.TrimSpace(search))
	if len([]rune(term)) > maxSearchRunes {
		return "", ErrValidation
	}
	return term, nil
}

func nullText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func mapWriteError(err error) error {
	if isConstraintViolation(err) {
		return ErrConflict
	}
	return err
}

func commitWithAudit(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any, reasonAR string) error {
	if err := writeAudit(ctx, tx, actorID, action, entityType, entityID, before, after, reasonAR); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func personSnapshot(view PersonView) map[string]any {
	return map[string]any{
		"canonical_name_ar": view.NameAR,
		"gender":            view.Gender,
		"identity_status":   view.IdentityStatus,
		"birth_date_from":   view.BirthDateFrom,
		"birth_date_to":     view.BirthDateTo,
		"death_date_from":   view.DeathDateFrom,
		"death_date_to":     view.DeathDateTo,
		"notes_ar":          view.NotesAR,
	}
}

func aliasSnapshot(view PersonAliasView) map[string]any {
	return map[string]any{
		"person_id":  view.PersonID,
		"value_ar":   view.ValueAR,
		"alias_type": view.AliasType,
		"source_id":  view.SourceID,
	}
}
