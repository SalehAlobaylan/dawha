package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/visibility"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Every test here runs against its own migrated schema, so the refusals below are
// proved against the real constraints, the real visibility policy and the real
// permission function rather than against a stand-in.

// installWriteFault makes the database reject one write, identified by a sentinel
// value in one column, so a mutation can be interrupted at a chosen step and its
// rollback observed. The trigger fires only for the sentinel and is dropped with
// the test, so every other writer in this schema is unaffected. It is the same
// fault-injection style the evidence service uses for its provenance boundary: a
// real database rule, and no hook in production code.
func installWriteFault(t *testing.T, pool *pgxpool.Pool, table, column, sentinel string) {
	t.Helper()
	ctx := context.Background()
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	function := fmt.Sprintf("dawha_identity_fault_fn_%s", suffix)
	trigger := fmt.Sprintf("dawha_identity_fault_trg_%s", suffix)
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $body$
		BEGIN
			RAISE EXCEPTION 'injected write fault on %s.%s';
		END;
		$body$
	`, function, table, column)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE TRIGGER %s BEFORE INSERT ON %s FOR EACH ROW
		WHEN (NEW.%s::text = '%s') EXECUTE FUNCTION %s()
	`, trigger, table, column, strings.ReplaceAll(sentinel, "'", "''"), function)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		if _, err := pool.Exec(cleanupCtx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON %s`, trigger, table)); err != nil {
			t.Errorf("dropping the fault trigger failed: %v", err)
		}
		if _, err := pool.Exec(cleanupCtx, fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, function)); err != nil {
			t.Errorf("dropping the fault function failed: %v", err)
		}
	})
}

func newService(t *testing.T, fixture *testsupport.Fixture) *Service {
	t.Helper()
	return NewService(fixture.Pool())
}

// publishedPerson builds a public tree with one published version holding one
// node, and returns the person that version rests on. The rows are written
// directly because what matters here is the fact the guard reads - a node in a
// published version - and not how the tree service produces one; internal/trees
// cannot be imported from a test in this package because it depends on it.
func publishedPerson(t *testing.T, fixture *testsupport.Fixture, owner actor.Actor) publishedIDs {
	t.Helper()
	treeID := uuid.New()
	versionID := uuid.New()
	personID := uuid.New()
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, $2, $2, $3)`,
		personID, "شخص منشور "+fixture.Tag(), owner.User.ID)
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, $2, 'public', $3)`,
		treeID, "شجرة منشورة "+fixture.Unique("tree"), owner.User.ID)
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', $3, now())`,
		versionID, treeID, owner.User.ID)
	fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, $4, 0)`,
		uuid.New(), versionID, personID, "شخص منشور "+fixture.Tag())
	return publishedIDs{person: personID.String(), tree: treeID.String()}
}

// publishedIDs names the rows a published tree fixture produced.
type publishedIDs struct {
	person string
	tree   string
}

// TestCreatedPersonIsResearchOnlyByConstruction is the service half of the
// guarantee the acceptance gate exists for: a person written through this API
// belongs to no published tree version, so the central visibility policy hides it
// from an anonymous reader, and it arrives unreviewed and unmerged so nothing
// downstream can treat it as settled.
func TestCreatedPersonIsResearchOnlyByConstruction(t *testing.T) {
	fixture := testsupport.New(t)
	writer := actor.RegisterAndLogin(t, fixture, "باحث")
	fixture.GrantRole(writer.User.ID, "researcher")
	service := newService(t, fixture)
	ctx := fixture.Ctx()

	person, err := service.CreatePerson(ctx, writer.User.ID, CreatePersonInput{
		CanonicalNameAR: "سعيد بن قيس " + fixture.Tag(),
		Gender:          "male",
		BirthDateFrom:   "0700-01-01",
		BirthDateTo:     "0710-01-01",
	})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	if person.IdentityStatus != "unreviewed" {
		t.Fatalf("identity status = %q, want unreviewed", person.IdentityStatus)
	}
	if got := fixture.Count(`SELECT count(*) FROM people WHERE id = $1 AND merged_into_id IS NULL`, person.ID); got != 1 {
		t.Fatalf("a created person carries a merge, or is missing: %d rows", got)
	}
	// No tree version was written, which is the whole reason the row stays private.
	if got := fixture.Count(`SELECT count(*) FROM tree_nodes WHERE person_id = $1`, person.ID); got != 0 {
		t.Fatalf("the created person appears in %d tree nodes, want 0", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_versions WHERE state = 'published' AND id IN (SELECT tree_version_id FROM tree_nodes WHERE person_id = $1)`, person.ID); got != 0 {
		t.Fatalf("the created person is inside %d published versions, want 0", got)
	}

	alias, err := service.CreatePersonAlias(ctx, person.ID, writer.User.ID, PersonAliasInput{ValueAR: "أبو سعيد", AliasType: "kunyah"})
	if err != nil {
		t.Fatalf("create alias: %v", err)
	}
	// An alias with no source is a platform record, which is the only kind plan 001
	// lets a public reader see - and this person is not public, so it is not seen.
	if alias.SourceID != "" {
		t.Fatalf("alias source = %q, want none for a platform record", alias.SourceID)
	}

	// The central policy, asked the way the dictionary asks, answers Hidden for an
	// anonymous caller even though the writer created the row. This is the single
	// decision every public read path makes, and this service does not get to make
	// a friendlier one.
	anonymous, err := visibility.Load(ctx, fixture.Pool(), "")
	if err != nil {
		t.Fatal(err)
	}
	access, err := anonymous.Person(ctx, fixture.Pool(), uuid.MustParse(person.ID))
	if err != nil {
		t.Fatal(err)
	}
	if access.Allowed() {
		t.Fatalf("an anonymous reader was granted %q access to a person created here", access)
	}

	// The creator is the one actor plan 001 lets see it, and it is still a
	// research-only view rather than a published one.
	writerPolicy, err := visibility.Load(ctx, fixture.Pool(), writer.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	ownAccess, err := writerPolicy.Person(ctx, fixture.Pool(), uuid.MustParse(person.ID))
	if err != nil {
		t.Fatal(err)
	}
	if ownAccess != visibility.AccessOwner {
		t.Fatalf("the creator's access = %q, want %q", ownAccess, visibility.AccessOwner)
	}

	// The alias list the writer sees is scoped exactly as the dictionary scopes it.
	aliases, err := service.ListPersonAliases(ctx, person.ID, writer.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases.Items) != 1 || aliases.Items[0].ID != alias.ID {
		t.Fatalf("aliases = %+v, want the one just created", aliases.Items)
	}
	// And the list of people the writer may read includes the row, because the
	// creator owns it - the same grant the dictionary index uses.
	people, err := service.ListPeople(ctx, writer.User.ID, person.NameAR)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range people.Items {
		if item.ID == person.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("the creator's own person list does not include the row they created: %+v", people.Items)
	}
	// An unrelated actor sees nothing, which is the same answer the dictionary gives.
	stranger := actor.Register(t, fixture, "غريب")
	anonymousList, err := service.ListPeople(ctx, stranger.User.ID, person.NameAR)
	if err == nil {
		t.Fatalf("a registered account with no identity role listed people: %+v", anonymousList.Items)
	}
}

// TestPublishedPersonResistsEveryMutation proves the plan's STOP condition: a
// person a published tree version already rests on is not changed and not removed,
// through any of the mutations this API offers.
func TestPublishedPersonResistsEveryMutation(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب الشجرة")
	writer := actor.RegisterAndLogin(t, fixture, "باحث")
	fixture.GrantRole(writer.User.ID, "researcher")
	service := newService(t, fixture)
	published := publishedPerson(t, fixture, owner)
	personID := published.person

	// A case that needs a research record to aim at writes it in setup, so the
	// audit baseline is taken after the setup and only the refused step is measured.
	cases := []struct {
		name   string
		setup  func()
		mutate func() error
		want   error
	}{
		{
			name: "update the person",
			mutate: func() error {
				_, err := service.UpdatePerson(fixture.Ctx(), personID, writer.User.ID, UpdatePersonInput{CanonicalNameAR: "اسم جديد", Gender: "male", ReasonAR: "تصحيح"})
				return err
			},
			want: ErrPublishedInterpretation,
		},
		{
			name:   "delete the person",
			mutate: func() error { return service.DeletePerson(fixture.Ctx(), personID, writer.User.ID, "حذف") },
			want:   ErrReferencedByInterpretation,
		},
		{
			name: "add an alias",
			mutate: func() error {
				_, err := service.CreatePersonAlias(fixture.Ctx(), personID, writer.User.ID, PersonAliasInput{ValueAR: "أبو محمد", AliasType: "kunyah"})
				return err
			},
			want: ErrPublishedInterpretation,
		},
		{
			name:  "change a relationship that points at the published person",
			setup: func() { aimRelationshipAt(t, fixture, service, writer, personID, "father_of") },
			mutate: func() error {
				_, err := service.UpdateEntityRelationship(fixture.Ctx(), relationshipIDFrom(t, fixture, personID), writer.User.ID, UpdateEntityRelationshipInput{
					Predicate: "spouse_of", Status: "unresolved", ReasonAR: "تغيير",
				})
				return err
			},
			want: ErrPublishedInterpretation,
		},
		{
			name:  "delete a relationship that points at the published person",
			setup: func() { aimRelationshipAt(t, fixture, service, writer, personID, "sibling_of") },
			mutate: func() error {
				return service.DeleteEntityRelationship(fixture.Ctx(), relationshipIDFrom(t, fixture, personID), writer.User.ID, "حذف")
			},
			want: ErrPublishedInterpretation,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.setup != nil {
				testCase.setup()
			}
			before := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID)
			err := testCase.mutate()
			if !errors.Is(err, testCase.want) {
				t.Fatalf("error = %v, want %v", err, testCase.want)
			}
			// A refusal writes nothing, so it leaves no audit event behind either.
			if after := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID); after != before {
				t.Fatalf("a refused mutation wrote %d audit events, want none", after-before)
			}
		})
	}

	// The published person is exactly as the tree published it.
	if got := fixture.Count(`SELECT count(*) FROM people WHERE id = $1 AND canonical_name_ar = $2`, personID, "شخص منشور "+fixture.Tag()); got != 1 {
		t.Fatal("the published person's name changed through a refused mutation")
	}
	if got := fixture.Count(`SELECT count(*) FROM person_aliases WHERE person_id = $1`, personID); got != 0 {
		t.Fatalf("the published person gained %d aliases through refused mutations", got)
	}
	// The relationships the refused cases tried to remove are still there, still
	// unresolved: the guard stopped the edit without touching the research record.
	if got := fixture.Count(`SELECT count(*) FROM entity_relationships WHERE subject_id = $1`, personID); got != 2 {
		t.Fatalf("relationships touching the published person = %d, want the 2 the refusals tried to change", got)
	}
}

// aimRelationshipAt records a research relationship whose subject is the published
// person. Creating one is allowed: a new research assertion about a published
// identity is invisible to every public reader. Only changing or removing it later
// is refused.
func aimRelationshipAt(t *testing.T, fixture *testsupport.Fixture, service *Service, writer actor.Actor, personID, predicate string) {
	t.Helper()
	other, err := service.CreatePerson(fixture.Ctx(), writer.User.ID, CreatePersonInput{CanonicalNameAR: "طرف آخر " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateEntityRelationship(fixture.Ctx(), writer.User.ID, CreateEntityRelationshipInput{
		SubjectType: "person", SubjectID: personID, Predicate: predicate, ObjectType: "person", ObjectID: other.ID,
	}); err != nil {
		t.Fatal(err)
	}
}

func relationshipIDFrom(t *testing.T, fixture *testsupport.Fixture, personID string) string {
	t.Helper()
	var id string
	if err := fixture.QueryRow(`SELECT id FROM entity_relationships WHERE subject_id = $1 ORDER BY created_at DESC LIMIT 1`, personID).Scan(&id); err != nil {
		t.Fatalf("read the relationship under test: %v", err)
	}
	return id
}

// TestReferenceFamiliesRefuseUpdateAndDelete covers the four reference families.
// A row written through this API is research-only, so its update and delete are
// refused with ErrResearchReference, and once it is published they are refused with
// ErrPublishedReference - the same refusal with the other reason, because neither
// state has a version to supersede the change. Each refusal is proved to be a real
// refusal rather than a route that happens to work: the row is still there,
// unchanged, and no audit event was written.
func TestReferenceFamiliesRefuseUpdateAndDelete(t *testing.T) {
	fixture := testsupport.New(t)
	writer := actor.RegisterAndLogin(t, fixture, "باحث")
	fixture.GrantRole(writer.User.ID, "researcher")
	service := newService(t, fixture)

	family, err := service.CreateFamily(fixture.Ctx(), writer.User.ID, CreateFamilyInput{CanonicalNameAR: "بيت " + fixture.Tag(), DescriptionAR: "وصف"})
	if err != nil {
		t.Fatalf("create family: %v", err)
	}
	tribe, err := service.CreateTribe(fixture.Ctx(), writer.User.ID, CreateTribeInput{CanonicalNameAR: "قبيلة " + fixture.Tag()})
	if err != nil {
		t.Fatalf("create tribe: %v", err)
	}
	branch, err := service.CreateBranch(fixture.Ctx(), writer.User.ID, CreateBranchInput{
		FamilyID: family.ID, CanonicalNameAR: "فرع " + fixture.Tag(), ValidFrom: "0100-01-01",
	})
	if err != nil {
		t.Fatalf("create branch: %v", err)
	}
	place, err := service.CreatePlace(fixture.Ctx(), writer.User.ID, CreatePlaceInput{CanonicalNameAR: "موضع " + fixture.Tag(), PlaceType: "village"})
	if err != nil {
		t.Fatalf("create place: %v", err)
	}

	cases := []struct {
		name   string
		mutate func() error
	}{
		{
			name: "family",
			mutate: func() error {
				_, err := service.UpdateFamily(fixture.Ctx(), family.ID, writer.User.ID, UpdateFamilyInput{CanonicalNameAR: "اسم آخر"})
				return err
			},
		},
		{
			name:   "family delete",
			mutate: func() error { return service.DeleteFamily(fixture.Ctx(), family.ID, writer.User.ID, "حذف") },
		},
		{
			name: "tribe",
			mutate: func() error {
				_, err := service.UpdateTribe(fixture.Ctx(), tribe.ID, writer.User.ID, UpdateTribeInput{CanonicalNameAR: "اسم آخر"})
				return err
			},
		},
		{
			name:   "tribe delete",
			mutate: func() error { return service.DeleteTribe(fixture.Ctx(), tribe.ID, writer.User.ID, "حذف") },
		},
		{
			name: "branch",
			mutate: func() error {
				_, err := service.UpdateBranch(fixture.Ctx(), branch.ID, writer.User.ID, UpdateBranchInput{CanonicalNameAR: "اسم آخر"})
				return err
			},
		},
		{
			name:   "branch delete",
			mutate: func() error { return service.DeleteBranch(fixture.Ctx(), branch.ID, writer.User.ID, "حذف") },
		},
		{
			name: "place",
			mutate: func() error {
				_, err := service.UpdatePlace(fixture.Ctx(), place.ID, writer.User.ID, UpdatePlaceInput{CanonicalNameAR: "اسم آخر", PlaceType: "city"})
				return err
			},
		},
		{
			name:   "place delete",
			mutate: func() error { return service.DeletePlace(fixture.Ctx(), place.ID, writer.User.ID, "حذف") },
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			before := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID)
			if err := testCase.mutate(); !errors.Is(err, ErrResearchReference) {
				t.Fatalf("error = %v, want ErrResearchReference", err)
			}
			if after := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID); after != before {
				t.Fatalf("a refused %s wrote %d audit events, want none", testCase.name, after-before)
			}
		})
	}

	// Publishing a reference row does not make it editable: it has no version either.
	publicFamilyID := uuid.New()
	fixture.Exec(`INSERT INTO families (id, canonical_name_ar, normalized_name_ar, description_ar, created_by, visibility) VALUES ($1, $2, $2, 'منشورة', $3, 'public')`,
		publicFamilyID, "بيت منشور "+fixture.Tag(), writer.User.ID)
	beforePublish := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID)
	if _, err := service.UpdateFamily(fixture.Ctx(), publicFamilyID.String(), writer.User.ID, UpdateFamilyInput{CanonicalNameAR: "اسم آخر"}); !errors.Is(err, ErrPublishedReference) {
		t.Fatalf("updating a published family = %v, want ErrPublishedReference", err)
	}
	if err := service.DeleteFamily(fixture.Ctx(), publicFamilyID.String(), writer.User.ID, "حذف"); !errors.Is(err, ErrPublishedReference) {
		t.Fatalf("deleting a published family = %v, want ErrPublishedReference", err)
	}
	if after := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID); after != beforePublish {
		t.Fatalf("a refused published-family mutation wrote %d audit events, want none", after-beforePublish)
	}
	if got := fixture.Count(`SELECT count(*) FROM families WHERE id = $1 AND canonical_name_ar = $2`, publicFamilyID, "بيت منشور "+fixture.Tag()); got != 1 {
		t.Fatal("the published family changed through a refused mutation")
	}

	for _, check := range []struct{ label, query string }{
		{"family", `SELECT count(*) FROM families WHERE id = $1 AND canonical_name_ar = $2`},
		{"tribe", `SELECT count(*) FROM tribes WHERE id = $1 AND canonical_name_ar = $2`},
		{"branch", `SELECT count(*) FROM branches WHERE id = $1 AND canonical_name_ar = $2`},
		{"place", `SELECT count(*) FROM places WHERE id = $1 AND canonical_name_ar = $2`},
	} {
		id := map[string]string{"family": family.ID, "tribe": tribe.ID, "branch": branch.ID, "place": place.ID}[check.label]
		name := map[string]string{"family": "بيت ", "tribe": "قبيلة ", "branch": "فرع ", "place": "موضع "}[check.label] + fixture.Tag()
		if got := fixture.Count(check.query, id, name); got != 1 {
			t.Fatalf("the %s row was changed or removed by a refused mutation", check.label)
		}
	}

	// A refusal for a row that does not exist answers not-found, so the refusal
	// cannot be used to learn which public rows exist.
	if _, err := service.UpdatePlace(fixture.Ctx(), uuid.NewString(), writer.User.ID, UpdatePlaceInput{CanonicalNameAR: "x", PlaceType: "city"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("updating a missing place = %v, want ErrNotFound", err)
	}
}

// TestReferenceRowsAreResearchOnlyByConstruction is the reference-family half of
// the guarantee the acceptance gate exists for. A family, tribe, branch or place
// created through this API binds visibility to 'private' and belongs to no version,
// so the central reference policy hides it from an anonymous reader while the role
// that wrote it still sees it.
func TestReferenceRowsAreResearchOnlyByConstruction(t *testing.T) {
	fixture := testsupport.New(t)
	writer := actor.RegisterAndLogin(t, fixture, "باحث")
	fixture.GrantRole(writer.User.ID, "researcher")
	service := newService(t, fixture)
	ctx := fixture.Ctx()

	created := []struct {
		kind  string
		table string
		id    string
	}{
		{kind: "family", table: "families"},
		{kind: "tribe", table: "tribes"},
		{kind: "place", table: "places"},
	}
	family, err := service.CreateFamily(ctx, writer.User.ID, CreateFamilyInput{CanonicalNameAR: "بيت " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}
	created[0].id = family.ID
	tribe, err := service.CreateTribe(ctx, writer.User.ID, CreateTribeInput{CanonicalNameAR: "قبيلة " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}
	created[1].id = tribe.ID
	place, err := service.CreatePlace(ctx, writer.User.ID, CreatePlaceInput{CanonicalNameAR: "موضع " + fixture.Tag(), PlaceType: "city"})
	if err != nil {
		t.Fatal(err)
	}
	created[2].id = place.ID
	branch, err := service.CreateBranch(ctx, writer.User.ID, CreateBranchInput{FamilyID: family.ID, CanonicalNameAR: "فرع " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}
	created = append(created, struct {
		kind  string
		table string
		id    string
	}{kind: "branch", table: "branches", id: branch.ID})

	for _, row := range created {
		if got := fixture.Count(`SELECT count(*) FROM `+row.table+` WHERE id = $1 AND visibility = 'private'`, row.id); got != 1 {
			t.Fatalf("the created %s is not research-only: %d rows are private", row.kind, got)
		}
	}
	// The column default says the same thing, so a writer that forgets the argument
	// gets the safe value rather than publishing by accident.
	fixture.Exec(`INSERT INTO places (id, canonical_name_ar, normalized_name_ar, place_type) VALUES ($1, $2, $2, 'region')`, uuid.New(), "موضع افتراضي")
	if got := fixture.Count(`SELECT count(*) FROM places WHERE canonical_name_ar = $1 AND visibility = 'private'`, "موضع افتراضي"); got != 1 {
		t.Fatal("the column default is not research-only")
	}

	// The central policy, asked the way the dictionary asks, hides every one of them
	// from an anonymous reader and shows them to a role holder.
	anonymous, err := visibility.Load(ctx, fixture.Pool(), "")
	if err != nil {
		t.Fatal(err)
	}
	privileged, err := visibility.Load(ctx, fixture.Pool(), writer.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !privileged.CanWriteReferences() {
		t.Fatal("a researcher does not hold the identity write role set the reference policy uses")
	}
	if anonymous.CanWriteReferences() {
		t.Fatal("an anonymous reader holds the identity write role set")
	}
	for _, row := range created {
		access, err := anonymous.Reference(ctx, fixture.Pool(), row.kind, uuid.MustParse(row.id))
		if err != nil {
			t.Fatal(err)
		}
		if access.Allowed() {
			t.Fatalf("an anonymous reader was granted %q access to a research-only %s", access, row.kind)
		}
		own, err := privileged.Reference(ctx, fixture.Pool(), row.kind, uuid.MustParse(row.id))
		if err != nil {
			t.Fatal(err)
		}
		if !own.Allowed() {
			t.Fatalf("the role that wrote the %s cannot read it back: %q", row.kind, own)
		}
	}
	// A registered account with no role is the anonymous reader as far as the
	// reference families are concerned.
	stranger := actor.Register(t, fixture, "غريب")
	strangerPolicy, err := visibility.Load(ctx, fixture.Pool(), stranger.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	access, err := strangerPolicy.Reference(ctx, fixture.Pool(), "place", uuid.MustParse(place.ID))
	if err != nil {
		t.Fatal(err)
	}
	if access.Allowed() {
		t.Fatalf("a registered account with no role was granted %q access to a research-only place", access)
	}
	// An unknown family is refused rather than guessed at, and its predicate reads
	// as nothing rather than as everything.
	if _, known := visibility.ReferenceTable("source"); known {
		t.Fatal("the reference table map accepted a table that is not a reference family")
	}
	if got := privileged.ReferencePredicate(visibility.NewParams(), "source", "x.id"); got != "FALSE" {
		t.Fatalf("an unknown family's predicate = %q, want FALSE", got)
	}
	missing, err := privileged.Reference(ctx, fixture.Pool(), "place", uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	if missing != visibility.AccessMissing {
		t.Fatalf("a missing reference row = %q, want %q", missing, visibility.AccessMissing)
	}
}

// TestMergedPersonIsNotEditable: a row an audited merge froze is not edited here.
// Unmerging belongs to the merge path that set the field.
func TestMergedPersonIsNotEditable(t *testing.T) {
	fixture := testsupport.New(t)
	writer := actor.RegisterAndLogin(t, fixture, "باحث")
	fixture.GrantRole(writer.User.ID, "researcher")
	service := newService(t, fixture)

	survivor, err := service.CreatePerson(fixture.Ctx(), writer.User.ID, CreatePersonInput{CanonicalNameAR: "الباقي " + fixture.Tag()})
	if err != nil {
		t.Fatalf("create survivor: %v", err)
	}
	merged, err := service.CreatePerson(fixture.Ctx(), writer.User.ID, CreatePersonInput{CanonicalNameAR: "الذاب " + fixture.Tag()})
	if err != nil {
		t.Fatalf("create merged: %v", err)
	}
	fixture.Exec(`UPDATE people SET identity_status = 'merged', merged_into_id = $1 WHERE id = $2`, survivor.ID, merged.ID)

	before := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID)
	if _, err := service.UpdatePerson(fixture.Ctx(), merged.ID, writer.User.ID, UpdatePersonInput{CanonicalNameAR: "اسم جديد", Gender: "male", ReasonAR: "تصحيح"}); !errors.Is(err, ErrMergedIdentity) {
		t.Fatalf("updating a merged person = %v, want ErrMergedIdentity", err)
	}
	if err := service.DeletePerson(fixture.Ctx(), merged.ID, writer.User.ID, "حذف"); !errors.Is(err, ErrMergedIdentity) {
		t.Fatalf("deleting a merged person = %v, want ErrMergedIdentity", err)
	}
	if after := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID); after != before {
		t.Fatalf("a refused merged-person mutation wrote %d audit events, want none", after-before)
	}
	if got := fixture.Count(`SELECT count(*) FROM people WHERE id = $1 AND canonical_name_ar = $2`, merged.ID, "الذاب "+fixture.Tag()); got != 1 {
		t.Fatal("the merged person's name changed through a refused mutation")
	}
	// The survivor is an ordinary research row and stays editable, so the guard is
	// about the merge and not about the pair.
	if _, err := service.UpdatePerson(fixture.Ctx(), survivor.ID, writer.User.ID, UpdatePersonInput{CanonicalNameAR: "الباقي المصحح " + fixture.Tag(), Gender: "male", ReasonAR: "تصحيح"}); err != nil {
		t.Fatalf("updating the survivor: %v", err)
	}
}

// TestResolvedRelationshipIsNotEditable: a relationship whose status is no longer
// unresolved records a human decision, and this API may not reverse it.
func TestResolvedRelationshipIsNotEditable(t *testing.T) {
	fixture := testsupport.New(t)
	writer := actor.RegisterAndLogin(t, fixture, "باحث")
	fixture.GrantRole(writer.User.ID, "researcher")
	service := newService(t, fixture)

	subject, err := service.CreatePerson(fixture.Ctx(), writer.User.ID, CreatePersonInput{CanonicalNameAR: "الموضوع " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}
	object, err := service.CreatePerson(fixture.Ctx(), writer.User.ID, CreatePersonInput{CanonicalNameAR: "الموضوع الآخر " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}
	unresolved, err := service.CreateEntityRelationship(fixture.Ctx(), writer.User.ID, CreateEntityRelationshipInput{
		SubjectType: "person", SubjectID: subject.ID, Predicate: "sibling_of", ObjectType: "person", ObjectID: object.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if unresolved.Status != "unresolved" {
		t.Fatalf("a created relationship has status %q, want unresolved", unresolved.Status)
	}
	// Resolving it is allowed, and that is what makes the next two refusals mean
	// something: the row is frozen from the moment it stops being unresolved.
	if _, err := service.UpdateEntityRelationship(fixture.Ctx(), unresolved.ID, writer.User.ID, UpdateEntityRelationshipInput{
		Predicate: "sibling_of", Status: "supported", ReasonAR: "أدلة",
	}); err != nil {
		t.Fatalf("resolve relationship: %v", err)
	}
	before := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID)
	if _, err := service.UpdateEntityRelationship(fixture.Ctx(), unresolved.ID, writer.User.ID, UpdateEntityRelationshipInput{
		Predicate: "father_of", Status: "disputed", ReasonAR: "تراجع",
	}); !errors.Is(err, ErrSettledInterpretation) {
		t.Fatalf("updating a resolved relationship = %v, want ErrSettledInterpretation", err)
	}
	if err := service.DeleteEntityRelationship(fixture.Ctx(), unresolved.ID, writer.User.ID, "حذف"); !errors.Is(err, ErrSettledInterpretation) {
		t.Fatalf("deleting a resolved relationship = %v, want ErrSettledInterpretation", err)
	}
	if after := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID); after != before {
		t.Fatalf("a refused resolved-relationship mutation wrote %d audit events, want none", after-before)
	}
	if got := fixture.Count(`SELECT count(*) FROM entity_relationships WHERE id = $1 AND status = 'supported' AND predicate = 'sibling_of'`, unresolved.ID); got != 1 {
		t.Fatal("the resolved relationship was changed through a refused mutation")
	}
}

// TestIdentityAuthorizationMatrix is guardrail three: the decision is the platform
// role, taken inside the transaction, and a right over one tree does not reach a
// global identity row. Every entity family is in the table, because a gate that
// was only proved for people would leave the other five untested.
func TestIdentityAuthorizationMatrix(t *testing.T) {
	fixture := testsupport.New(t)
	service := newService(t, fixture)

	treeOwner := actor.RegisterAndLogin(t, fixture, "صاحب شجرة")
	treeCollaborator := actor.RegisterAndLogin(t, fixture, "متعاون")
	registered := actor.RegisterAndLogin(t, fixture, "مسجل")
	collaborator := actor.RegisterAndLogin(t, fixture, "متعامل")
	researcher := actor.RegisterAndLogin(t, fixture, "باحث")
	moderator := actor.RegisterAndLogin(t, fixture, "مشرف")
	administrator := actor.RegisterAndLogin(t, fixture, "مدير")
	actor.Register(t, fixture, "مشارك في شجرة فقط")
	for _, id := range []string{collaborator.User.ID, researcher.User.ID, moderator.User.ID, administrator.User.ID} {
		fixture.GrantRole(id, "researcher")
	}
	fixture.GrantRole(collaborator.User.ID, "collaborator")
	fixture.GrantRole(moderator.User.ID, "moderator")
	fixture.GrantRole(administrator.User.ID, "admin")
	// A collaborator row on a tree the two of them share. The platform roles above
	// are what these accounts hold; the tree relationship is what the matrix says
	// must not matter.
	matrixTreeID := uuid.New()
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, $2, 'private', $3)`,
		matrixTreeID, "شجرة "+fixture.Unique("matrix"), treeOwner.User.ID)
	for _, collaboratorID := range []string{treeCollaborator.User.ID, registered.User.ID} {
		fixture.Exec(`INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'edit', $3)`,
			matrixTreeID, collaboratorID, treeOwner.User.ID)
	}

	// The shared prerequisites are written by a researcher, so each cell below is
	// testing the actor under test and not the setup.
	family, err := service.CreateFamily(fixture.Ctx(), researcher.User.ID, CreateFamilyInput{CanonicalNameAR: "بيت " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}
	subject, err := service.CreatePerson(fixture.Ctx(), researcher.User.ID, CreatePersonInput{CanonicalNameAR: "موضوع " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}
	object, err := service.CreatePerson(fixture.Ctx(), researcher.User.ID, CreatePersonInput{CanonicalNameAR: "موضوع آخر " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}

	families := []struct {
		name    string
		allowed bool
		mutate  func(actorID string) error
	}{
		{
			name:    "anonymous",
			allowed: false,
			mutate: func(string) error {
				_, err := service.CreatePerson(fixture.Ctx(), "", CreatePersonInput{CanonicalNameAR: "بلا جلسة"})
				return err
			},
		},
		{
			name:    "registered without a global role",
			allowed: false,
			mutate: func(actorID string) error {
				_, err := service.CreatePerson(fixture.Ctx(), actorID, CreatePersonInput{CanonicalNameAR: "مسجل"})
				return err
			},
		},
		{
			name:    "tree owner without a global role",
			allowed: false,
			mutate: func(actorID string) error {
				_, err := service.CreateFamily(fixture.Ctx(), actorID, CreateFamilyInput{CanonicalNameAR: "بيت صاحب الشجرة"})
				return err
			},
		},
		{
			name:    "edit collaborator on a tree without a global role",
			allowed: false,
			mutate: func(actorID string) error {
				_, err := service.CreateTribe(fixture.Ctx(), actorID, CreateTribeInput{CanonicalNameAR: "قبيلة متعاون"})
				return err
			},
		},
		{
			name:    "platform collaborator",
			allowed: true,
			mutate: func(actorID string) error {
				_, err := service.CreateTribe(fixture.Ctx(), actorID, CreateTribeInput{CanonicalNameAR: "قبيلة " + fixture.Tag()})
				return err
			},
		},
		{
			name:    "researcher",
			allowed: true,
			mutate: func(actorID string) error {
				_, err := service.CreateBranch(fixture.Ctx(), actorID, CreateBranchInput{FamilyID: family.ID, CanonicalNameAR: "فرع " + fixture.Tag()})
				return err
			},
		},
		{
			name:    "moderator",
			allowed: true,
			mutate: func(actorID string) error {
				_, err := service.CreatePlace(fixture.Ctx(), actorID, CreatePlaceInput{CanonicalNameAR: "موضع " + fixture.Tag(), PlaceType: "region"})
				return err
			},
		},
		{
			name:    "admin",
			allowed: true,
			mutate: func(actorID string) error {
				_, err := service.CreateEntityRelationship(fixture.Ctx(), actorID, CreateEntityRelationshipInput{
					SubjectType: "person", SubjectID: subject.ID, Predicate: "sibling_of", ObjectType: "person", ObjectID: object.ID,
				})
				return err
			},
		},
	}
	byName := map[string]string{
		"anonymous":                                         "",
		"registered without a global role":                  registered.User.ID,
		"tree owner without a global role":                  treeOwner.User.ID,
		"edit collaborator on a tree without a global role": treeCollaborator.User.ID,
		"platform collaborator":                             collaborator.User.ID,
		"researcher":                                        researcher.User.ID,
		"moderator":                                         moderator.User.ID,
		"admin":                                             administrator.User.ID,
	}
	for _, family := range families {
		actorID, ok := byName[family.name]
		if !ok {
			t.Fatalf("no actor is wired for the %q case", family.name)
		}
		before := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id::text = $1`, actorID)
		err := family.mutate(actorID)
		if family.allowed && err != nil {
			t.Fatalf("%s was refused: %v", family.name, err)
		}
		if !family.allowed {
			if !errors.Is(err, ErrForbidden) {
				t.Fatalf("%s = %v, want ErrForbidden", family.name, err)
			}
			if after := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id::text = $1`, actorID); after != before {
				t.Fatalf("%s wrote %d audit events before it was refused", family.name, after-before)
			}
		}
	}
	// The alias family gets the same matrix, asserted separately because its
	// prerequisite person has to exist before the alias can be attempted.
	// An alias write needs the role AND reachability to the person, so the allowed
	// cases create their own person first. The denied cases aim at a person the
	// researcher made, which is the shape that separates the two refusals: an
	// account with no right at all is refused before the person is considered.
	for _, allowed := range []struct {
		name    string
		actorID string
	}{
		{"platform collaborator", collaborator.User.ID},
		{"moderator", moderator.User.ID},
	} {
		person, err := service.CreatePerson(fixture.Ctx(), allowed.actorID, CreatePersonInput{CanonicalNameAR: "شخص الألقاب " + fixture.Tag()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreatePersonAlias(fixture.Ctx(), person.ID, allowed.actorID, PersonAliasInput{ValueAR: "لقب " + fixture.Tag()}); err != nil {
			t.Fatalf("%s could not write an alias on their own person: %v", allowed.name, err)
		}
	}
	// The role alone is not enough: a holder of the identity role who cannot read
	// the person is refused, and the refusal is the one that does not leak existence.
	collaboratorsPerson, err := service.CreatePerson(fixture.Ctx(), researcher.User.ID, CreatePersonInput{CanonicalNameAR: "شخص الباحث " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreatePersonAlias(fixture.Ctx(), collaboratorsPerson.ID, collaborator.User.ID, PersonAliasInput{ValueAR: "لقب " + fixture.Tag()}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("the platform collaborator aliased a person they cannot read = %v, want ErrNotFound", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM person_aliases WHERE person_id = $1`, collaboratorsPerson.ID); got != 0 {
		t.Fatalf("the refused alias left %d rows behind", got)
	}
	for _, denied := range []struct {
		name    string
		actorID string
	}{
		{"tree owner without a global role", treeOwner.User.ID},
		{"edit collaborator on a tree without a global role", treeCollaborator.User.ID},
		{"registered without a global role", registered.User.ID},
	} {
		person, err := service.CreatePerson(fixture.Ctx(), researcher.User.ID, CreatePersonInput{CanonicalNameAR: "شخص الألقاب " + fixture.Tag()})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.CreatePersonAlias(fixture.Ctx(), person.ID, denied.actorID, PersonAliasInput{ValueAR: "لقب " + fixture.Tag()}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("%s wrote an alias = %v, want ErrForbidden", denied.name, err)
		}
		if got := fixture.Count(`SELECT count(*) FROM person_aliases WHERE person_id = $1`, person.ID); got != 0 {
			t.Fatalf("%s left %d aliases behind", denied.name, got)
		}
	}
}

// TestMutationsRollBackWithTheirAuditEvent is guardrail four. The fault is injected
// by a database trigger, not by a hook in production code, and it fires on the
// audit insert, so the row that the audit was supposed to explain must disappear
// with it.
func TestMutationsRollBackWithTheirAuditEvent(t *testing.T) {
	fixture := testsupport.New(t)
	writer := actor.RegisterAndLogin(t, fixture, "باحث")
	fixture.GrantRole(writer.User.ID, "researcher")
	service := newService(t, fixture)
	ctx := fixture.Ctx()

	// A healthy write of each kind first, so the rollback assertion below is about
	// the injected fault and not about a mutation that never worked.
	healthyPerson, err := service.CreatePerson(ctx, writer.User.ID, CreatePersonInput{CanonicalNameAR: "سليم " + fixture.Tag()})
	if err != nil {
		t.Fatalf("create person: %v", err)
	}
	healthyAlias, err := service.CreatePersonAlias(ctx, healthyPerson.ID, writer.User.ID, PersonAliasInput{ValueAR: "لقب سليم " + fixture.Tag()})
	if err != nil {
		t.Fatalf("create alias: %v", err)
	}
	second, err := service.CreatePerson(ctx, writer.User.ID, CreatePersonInput{CanonicalNameAR: "شخص ثانٍ " + fixture.Tag()})
	if err != nil {
		t.Fatalf("create second person: %v", err)
	}
	healthyRelationship, err := service.CreateEntityRelationship(ctx, writer.User.ID, CreateEntityRelationshipInput{
		SubjectType: "person", SubjectID: healthyPerson.ID, Predicate: "sibling_of", ObjectType: "person", ObjectID: second.ID,
	})
	if err != nil {
		t.Fatalf("create relationship: %v", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID); got != 4 {
		t.Fatalf("healthy audit events = %d, want 4", got)
	}

	installWriteFault(t, fixture.Pool(), "audit_log", "action", actionPersonCreated)
	if _, err := service.CreatePerson(ctx, writer.User.ID, CreatePersonInput{CanonicalNameAR: "مرفوض " + fixture.Tag()}); err == nil {
		t.Fatal("a person whose audit insert fails was created anyway")
	}
	if got := fixture.Count(`SELECT count(*) FROM people WHERE canonical_name_ar = $1`, "مرفوض "+fixture.Tag()); got != 0 {
		t.Fatal("a person survived a failed audit write")
	}

	installWriteFault(t, fixture.Pool(), "audit_log", "action", actionPersonAliasCreated)
	if _, err := service.CreatePersonAlias(ctx, healthyPerson.ID, writer.User.ID, PersonAliasInput{ValueAR: "لقب مرفوض " + fixture.Tag()}); err == nil {
		t.Fatal("an alias whose audit insert fails was created anyway")
	}
	if got := fixture.Count(`SELECT count(*) FROM person_aliases WHERE value_ar = $1`, "لقب مرفوض "+fixture.Tag()); got != 0 {
		t.Fatal("an alias survived a failed audit write")
	}

	installWriteFault(t, fixture.Pool(), "audit_log", "action", actionRelationshipCreated)
	if _, err := service.CreateEntityRelationship(ctx, writer.User.ID, CreateEntityRelationshipInput{
		SubjectType: "person", SubjectID: healthyPerson.ID, Predicate: "spouse_of", ObjectType: "person", ObjectID: second.ID,
	}); err == nil {
		t.Fatal("a relationship whose audit insert fails was created anyway")
	}
	if got := fixture.Count(`SELECT count(*) FROM entity_relationships WHERE predicate = 'spouse_of'`); got != 0 {
		t.Fatal("a relationship survived a failed audit write")
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID); got != 4 {
		t.Fatalf("audit events after the injected faults = %d, want the 4 healthy ones", got)
	}

	// The same boundary in the other direction: a failing primary write must not
	// leave an audit event describing a change that never happened.
	faultyName := "يفشل الإدراج " + fixture.Tag()
	installWriteFault(t, fixture.Pool(), "people", "canonical_name_ar", faultyName)
	auditBefore := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID)
	if _, err := service.CreatePerson(ctx, writer.User.ID, CreatePersonInput{CanonicalNameAR: faultyName}); err == nil {
		t.Fatal("a person whose own insert fails was created anyway")
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE actor_id = $1`, writer.User.ID); got != auditBefore {
		t.Fatalf("a failed person write left %d audit events behind", got-auditBefore)
	}

	// The healthy rows are still there and still readable, so the rollback removed
	// only the rejected change.
	aliases, err := service.ListPersonAliases(ctx, healthyPerson.ID, writer.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases.Items) != 1 || aliases.Items[0].ID != healthyAlias.ID {
		t.Fatalf("aliases = %+v, want only the healthy one", aliases.Items)
	}
	if got := fixture.Count(`SELECT count(*) FROM entity_relationships WHERE id = $1`, healthyRelationship.ID); got != 1 {
		t.Fatal("the healthy relationship did not survive the injected faults")
	}
}

// TestAliasUniquenessAndSourceVisibility covers the two ways an alias can be
// refused that are not about publication: the unique index, and a source the
// actor may not read.
func TestAliasUniquenessAndSourceVisibility(t *testing.T) {
	fixture := testsupport.New(t)
	collaborator := actor.RegisterAndLogin(t, fixture, "متعامل")
	fixture.GrantRole(collaborator.User.ID, "collaborator")
	researcher := actor.RegisterAndLogin(t, fixture, "باحث")
	fixture.GrantRole(researcher.User.ID, "researcher")
	service := newService(t, fixture)
	ctx := fixture.Ctx()

	person, err := service.CreatePerson(ctx, collaborator.User.ID, CreatePersonInput{CanonicalNameAR: "صاحب الألقاب " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreatePersonAlias(ctx, person.ID, collaborator.User.ID, PersonAliasInput{ValueAR: "أبو_clean", AliasType: "kunyah"}); err != nil {
		t.Fatalf("create alias: %v", err)
	}
	// The unique index is on the normalized value, so a different spelling of the
	// same Arabic name is still the same alias.
	if _, err := service.CreatePersonAlias(ctx, person.ID, collaborator.User.ID, PersonAliasInput{ValueAR: "أبو clean", AliasType: "kunyah"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("a duplicate normalized alias = %v, want ErrConflict", err)
	}
	// A different type is a different alias.
	if _, err := service.CreatePersonAlias(ctx, person.ID, collaborator.User.ID, PersonAliasInput{ValueAR: "أبو clean", AliasType: "laqab"}); err != nil {
		t.Fatalf("the same spelling under a different type: %v", err)
	}

	// A source the actor cannot read cannot be attached. The collaborator role is
	// not a research role, so the private source of the researcher is invisible to
	// it, while a researcher may read every source. Each actor aliases a person it
	// can read, so the source is the only thing the case is about.
	privateSourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, 'مصدر خاص', 'book', 'private', $2)`, privateSourceID, researcher.User.ID)
	researcherPerson, err := service.CreatePerson(ctx, researcher.User.ID, CreatePersonInput{CanonicalNameAR: "شخص الباحث " + fixture.Tag()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreatePersonAlias(ctx, person.ID, collaborator.User.ID, PersonAliasInput{
		ValueAR: "لقب من مصدر", AliasType: "source_spelling", SourceID: privateSourceID.String(),
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("attaching an unreadable source = %v, want ErrNotFound", err)
	}
	if _, err := service.CreatePersonAlias(ctx, researcherPerson.ID, researcher.User.ID, PersonAliasInput{
		ValueAR: "لقب من مصدر", AliasType: "source_spelling", SourceID: privateSourceID.String(),
	}); err != nil {
		t.Fatalf("a researcher could not attach a source they may read: %v", err)
	}
	// A malformed source is a validation error, never a silent NULL that would turn
	// a source spelling into a public platform record.
	if _, err := service.CreatePersonAlias(ctx, researcherPerson.ID, researcher.User.ID, PersonAliasInput{
		ValueAR: "لقب تالف", AliasType: "source_spelling", SourceID: "not-a-uuid",
	}); !errors.Is(err, ErrValidation) {
		t.Fatalf("a malformed source = %v, want ErrValidation", err)
	}
}

// TestPlaceGeometryIsOptional: the point is built by PostGIS from two validated
// scalars, an absent point stays NULL rather than becoming the origin, and a place
// can nest inside another place.
func TestPlaceGeometryIsOptional(t *testing.T) {
	fixture := testsupport.New(t)
	writer := actor.RegisterAndLogin(t, fixture, "باحث")
	fixture.GrantRole(writer.User.ID, "researcher")
	service := newService(t, fixture)
	ctx := fixture.Ctx()

	region, err := service.CreatePlace(ctx, writer.User.ID, CreatePlaceInput{CanonicalNameAR: "إقليم " + fixture.Tag(), PlaceType: "region"})
	if err != nil {
		t.Fatal(err)
	}
	if region.Longitude != nil || region.Latitude != nil {
		t.Fatalf("a place with no point reports (%v, %v)", region.Longitude, region.Latitude)
	}
	if got := fixture.Count(`SELECT count(*) FROM places WHERE id = $1 AND geometry IS NULL`, region.ID); got != 1 {
		t.Fatal("an absent point was stored as a geometry")
	}
	longitude, latitude := 39.2, 21.5
	village, err := service.CreatePlace(ctx, writer.User.ID, CreatePlaceInput{
		CanonicalNameAR: "قرية " + fixture.Tag(), PlaceType: "village", ParentPlaceID: region.ID, Longitude: &longitude, Latitude: &latitude,
	})
	if err != nil {
		t.Fatal(err)
	}
	if village.ParentPlaceID != region.ID {
		t.Fatalf("parent place = %q, want %q", village.ParentPlaceID, region.ID)
	}
	if village.Longitude == nil || village.Latitude == nil {
		t.Fatal("a place with a point reports no coordinates")
	}
	if got := fixture.Count(`SELECT count(*) FROM places WHERE id = $1 AND ST_SRID(geometry) = 4326 AND ST_X(geometry) = $2 AND ST_Y(geometry) = $3`,
		village.ID, longitude, latitude); got != 1 {
		t.Fatal("the stored point is not the validated WGS84 pair")
	}
	// A parent that does not exist is refused rather than stored as a dangling id.
	if _, err := service.CreatePlace(ctx, writer.User.ID, CreatePlaceInput{
		CanonicalNameAR: "قرية يتيمة " + fixture.Tag(), PlaceType: "village", ParentPlaceID: uuid.NewString(),
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a place under a missing parent = %v, want ErrNotFound", err)
	}
}

// draftTree builds a draft tree owned by owner and returns its id, the person in
// its current draft, and a publish function. It is written with direct statements
// because this package cannot import internal/trees, which depends on it.
type draftTree struct {
	treeID    string
	personID  string
	published func(t *testing.T, fixture *testsupport.Fixture)
}

func newDraftTree(t *testing.T, fixture *testsupport.Fixture, owner actor.Actor) draftTree {
	t.Helper()
	treeID := uuid.New()
	versionID := uuid.New()
	personID := uuid.New()
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, created_by) VALUES ($1, $2, $2, $3)`,
		personID, "شخص المسودة "+fixture.Tag(), owner.User.ID)
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, $2, 'private', $3)`,
		treeID, "شجرة "+fixture.Unique("draft"), owner.User.ID)
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 1, 'draft', $3)`,
		versionID, treeID, owner.User.ID)
	fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, $4, 0)`,
		uuid.New(), versionID, personID, "شخص المسودة")
	return draftTree{
		treeID:   treeID.String(),
		personID: personID.String(),
		published: func(t *testing.T, fixture *testsupport.Fixture) {
			t.Helper()
			// Publishing seals the draft and opens a new one that carries a copy of
			// every node, so the person is in both a published version and the new
			// current draft: the tree owner keeps a tree right and still meets the
			// published refusal.
			fixture.Exec(`UPDATE tree_versions SET state = 'published', published_by = $2, published_at = now() WHERE id = $1`, versionID, owner.User.ID)
			nextID := uuid.New()
			fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, created_by) VALUES ($1, $2, 2, 'draft', $3)`, nextID, treeID, owner.User.ID)
			fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) SELECT gen_random_uuid(), $1, person_id, display_name_ar, sort_order FROM tree_nodes WHERE tree_version_id = $2`, nextID, versionID)
		},
	}
}

// TestAliasWriteReachesTreeScopedRights pins the authorization rule for a person
// alias. The alias form lives in the tree draft editor, so the three accounts that
// see it are the three that have to be able to use it - and the published refusal
// has to survive that widening, or the widening is a hole.
func TestAliasWriteReachesTreeScopedRights(t *testing.T) {
	fixture := testsupport.New(t)
	service := newService(t, fixture)
	ctx := fixture.Ctx()

	owner := actor.RegisterAndLogin(t, fixture, "صاحب المسودة")
	collaborator := actor.RegisterAndLogin(t, fixture, "متعاون")
	viewer := actor.RegisterAndLogin(t, fixture, "مشاهد")
	registered := actor.RegisterAndLogin(t, fixture, "مسجل")
	researcher := actor.RegisterAndLogin(t, fixture, "باحث")
	fixture.GrantRole(researcher.User.ID, "researcher")

	tree := newDraftTree(t, fixture, owner)
	fixture.Exec(`INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'edit', $3)`, tree.treeID, collaborator.User.ID, owner.User.ID)
	fixture.Exec(`INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'view', $3)`, tree.treeID, viewer.User.ID, owner.User.ID)

	// The registered owner of the draft can record an alias on their own person.
	if _, err := service.CreatePersonAlias(ctx, tree.personID, owner.User.ID, PersonAliasInput{ValueAR: "أبو " + fixture.Tag(), AliasType: "kunyah"}); err != nil {
		t.Fatalf("the draft owner could not write an alias: %v", err)
	}
	// So can an edit-level collaborator on the same draft, which is exactly the
	// right trees.AddPerson already grants them.
	if _, err := service.CreatePersonAlias(ctx, tree.personID, collaborator.User.ID, PersonAliasInput{ValueAR: "ابن " + fixture.Tag(), AliasType: "nisbah"}); err != nil {
		t.Fatalf("the draft collaborator could not write an alias: %v", err)
	}
	// A view-level collaborator may read the draft and not write to it.
	if _, err := service.CreatePersonAlias(ctx, tree.personID, viewer.User.ID, PersonAliasInput{ValueAR: "لقب " + fixture.Tag()}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a view collaborator wrote an alias = %v, want ErrForbidden", err)
	}
	// A registered account with no relationship to the tree has no alias right.
	if _, err := service.CreatePersonAlias(ctx, tree.personID, registered.User.ID, PersonAliasInput{ValueAR: "لقب " + fixture.Tag()}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("an unrelated registered account wrote an alias = %v, want ErrForbidden", err)
	}
	// The identity write role still works, including for a person this actor did not
	// create, as long as the person is one the actor can read.
	other := newDraftTree(t, fixture, owner)
	fixture.Exec(`INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'edit', $3)`, other.treeID, researcher.User.ID, owner.User.ID)
	if _, err := service.CreatePersonAlias(ctx, other.personID, researcher.User.ID, PersonAliasInput{ValueAR: "لقع " + fixture.Tag()}); err != nil {
		t.Fatalf("a researcher with the identity role could not write an alias: %v", err)
	}

	// The tree-scoped right is aimed at a draft, not at a tree: once the version is
	// published the person is in a published interpretation and the refusal stands,
	// even for the owner who still has a draft node and a tree right.
	tree.published(t, fixture)
	if _, err := service.CreatePersonAlias(ctx, tree.personID, owner.User.ID, PersonAliasInput{ValueAR: "لقب منشور " + fixture.Tag()}); !errors.Is(err, ErrPublishedInterpretation) {
		t.Fatalf("the draft owner aliased a published person = %v, want ErrPublishedInterpretation", err)
	}
	if _, err := service.CreatePersonAlias(ctx, tree.personID, collaborator.User.ID, PersonAliasInput{ValueAR: "لقب منشور " + fixture.Tag()}); !errors.Is(err, ErrPublishedInterpretation) {
		t.Fatalf("the draft collaborator aliased a published person = %v, want ErrPublishedInterpretation", err)
	}
	// The existing aliases are just as frozen as the person is.
	aliases, err := service.ListPersonAliases(ctx, tree.personID, owner.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases.Items) != 2 {
		t.Fatalf("aliases on the published person = %d, want the 2 written before publication", len(aliases.Items))
	}

	// A private person the actor cannot reach stays unreachable. A view-level
	// collaborator on another account's draft can read that draft, so a person who
	// is in neither their tree nor their own creations is the case that matters:
	// the actor holds a tree right somewhere, and the person is not in it.
	hidden := newDraftTree(t, fixture, owner)
	if _, err := service.CreatePersonAlias(ctx, hidden.personID, researcher.User.ID, PersonAliasInput{ValueAR: "لقب مخفي " + fixture.Tag()}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a researcher aliased a person they cannot read = %v, want ErrNotFound", err)
	}
	// And the collaborator with a view right on a tree they do not hold edit on
	// cannot reach a person outside it either.
	if _, err := service.CreatePersonAlias(ctx, hidden.personID, viewer.User.ID, PersonAliasInput{ValueAR: "لقب مخفي " + fixture.Tag()}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("a view collaborator aliased a person outside the tree = %v, want ErrForbidden", err)
	}
	// A registered account with no tree right is refused before the person is even
	// considered, so the two refusals stay distinguishable.
	if _, err := service.CreatePersonAlias(ctx, hidden.personID, registered.User.ID, PersonAliasInput{ValueAR: "لقب " + fixture.Tag()}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("an unrelated account aliased a hidden person = %v, want ErrForbidden", err)
	}
}
