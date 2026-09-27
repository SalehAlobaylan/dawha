package research

// A RESEARCH-ONLY PERSON MUST NOT ARRIVE THROUGH ITS OWN ALIASES.
//
// The workspace reads a person's alternate spellings with no predicate at all:
// `SELECT value_ar FROM person_aliases WHERE person_id = $1`. Every other read
// path in this repository scopes that same table, and the scope is the plan 001
// privacy class:
//
//   - the PERSON must be readable by the actor, or the aliases are not read at
//     all. A research role is not a blanket bypass over people, so a person
//     nobody has published does not become visible because the reader is a
//     researcher.
//   - the alias's own SOURCE must be readable, or the spelling is not shown. An
//     alias taken from a research-only source is a way to disclose a private
//     source through a workspace that looks like a public one.
//   - an alias carrying NO source is a PLATFORM record and stays. "No source" is
//     not "no permission": it is a row the platform itself wrote.
//
// The third rule is the one a careless fix loses, so it is asserted directly and
// not only as part of a longer list.
//
// The tests drive Workspace rather than the query, because the query is not the
// defect - the missing policy is - and a policy that is correct in isolation can
// still be handed an anonymous scope by the caller.

import (
	"slices"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/google/uuid"
)

// aliasFixture is the shape these tests read: one public person and one
// research-only person, each carrying a platform alias, an alias from a public
// source and an alias from a private source.
type aliasFixture struct {
	questionID          uuid.UUID
	publicPersonID      uuid.UUID
	researchPersonID    uuid.UUID
	researchPersonOwner uuid.UUID
	publicSourceID      uuid.UUID
	privateSourceID     uuid.UUID
	// The three alias values of the PUBLIC person. The private person's carry
	// their own distinct values so a leak is attributable.
	publicPlatformAlias    string
	publicPublicAlias      string
	publicPrivateAlias     string
	researchPlatformAlias  string
	researchPrivateAlias   string
	readerID               uuid.UUID
	researcherID           uuid.UUID
	researcherSourceOwner  uuid.UUID
	collaboratorPersonID   uuid.UUID
	collaboratorTreeID     uuid.UUID
	anonymousUnlinkedCount int
}

func seedWorkspaceAliasFixture(t *testing.T, fixture *testsupport.Fixture) aliasFixture {
	t.Helper()
	unique := fixture.Unique("alias")

	reader := uuid.New()
	researcher := uuid.New()
	researcherSourceOwner := uuid.New()
	researchPersonOwner := uuid.New()
	fixture.Exec(`INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'قارئ'), ($3, $4, 'باحث'), ($5, $6, 'صاحب المصدر الخاص'), ($7, $8, 'صاحب الشخص البحثي')`,
		reader, unique+"-reader@example.test", researcher, unique+"-researcher@example.test",
		researcherSourceOwner, unique+"-sourceowner@example.test", researchPersonOwner, unique+"-personowner@example.test")
	// Only the researcher holds a role. The reader is a registered user with
	// none, which is the interesting case: a session that can reach the
	// workspace and must still not see a private spelling.
	fixture.Exec(`INSERT INTO user_roles (user_id, role) VALUES ($1, 'registered'), ($2, 'researcher'), ($3, 'registered')`,
		reader, researcher, researcherSourceOwner)

	questionID := uuid.New()
	if err := fixture.QueryRow(`INSERT INTO open_questions (id, title_ar, status, priority, created_by) VALUES ($1, $2, 'open', 'high', $3) RETURNING id`,
		questionID, "سؤال الأسماء المستعارة "+unique, reader).Scan(&questionID); err != nil {
		t.Fatalf("insert open question: %v", err)
	}

	// A public source, a private source, and a person who owns neither.
	publicSourceID := uuid.New()
	privateSourceID := uuid.New()
	fixture.Exec(`INSERT INTO sources (id, title_ar, source_type, visibility, created_by) VALUES ($1, $2, 'book', 'public', $3), ($4, $5, 'book', 'private', $6)`,
		publicSourceID, "مصدر عام "+unique, researcher, privateSourceID, "مصدر خاص "+unique, researcherSourceOwner)

	// The PUBLIC person: published in a published version of a public tree, which
	// is what the people policy calls public. Nobody owns it.
	publicPersonID := uuid.New()
	treeID, versionID := uuid.New(), uuid.New()
	fixture.Exec(`INSERT INTO trees (id, name_ar, visibility, owner_id) VALUES ($1, $2, 'public', $3)`, treeID, "شجرة "+unique, reader)
	fixture.Exec(`INSERT INTO tree_versions (id, tree_id, version_number, state, published_by, published_at) VALUES ($1, $2, 1, 'published', $3, now())`, versionID, treeID, reader)
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status) VALUES ($1, $2, $3, 'reviewed')`,
		publicPersonID, "عبد الله العام "+unique, "عبد الله العام")
	fixture.Exec(`INSERT INTO tree_nodes (id, tree_version_id, person_id, display_name_ar, sort_order) VALUES ($1, $2, $3, $4, 1)`, uuid.New(), versionID, publicPersonID, "عبد الله العام")

	// The RESEARCH-ONLY person: created by somebody, in no published tree, in no
	// tree at all. This is the row whose aliases must not surface.
	researchPersonID := uuid.New()
	fixture.Exec(`INSERT INTO people (id, canonical_name_ar, normalized_name_ar, identity_status, created_by) VALUES ($1, $2, $3, 'reviewed', $4)`,
		researchPersonID, "سعد الخاص "+unique, "سعد الخاص", researchPersonOwner)

	publicPlatformAlias := "أبو بكر " + unique
	publicPublicAlias := "عبد الله العام " + unique + "-منشور"
	publicPrivateAlias := "عبد الله العام " + unique + "-خاص"
	researchPlatformAlias := "أبو سعد " + unique
	researchPrivateAlias := "سعد الخاص " + unique + "-خاص"
	for _, alias := range []struct {
		personID, sourceID uuid.UUID
		value              string
		aliasType          string
	}{
		{publicPersonID, uuid.Nil, publicPlatformAlias, "kunyah"},
		{publicPersonID, publicSourceID, publicPublicAlias, "source_spelling"},
		{publicPersonID, privateSourceID, publicPrivateAlias, "source_spelling"},
		{researchPersonID, uuid.Nil, researchPlatformAlias, "kunyah"},
		{researchPersonID, privateSourceID, researchPrivateAlias, "source_spelling"},
	} {
		var sourceArg any
		if alias.sourceID != uuid.Nil {
			sourceArg = alias.sourceID
		}
		fixture.Exec(`INSERT INTO person_aliases (person_id, value_ar, normalized_value_ar, alias_type, source_id) VALUES ($1, $2, $3, $4, $5)`,
			alias.personID, alias.value, alias.value, alias.aliasType, sourceArg)
	}

	return aliasFixture{
		questionID: questionID, publicPersonID: publicPersonID, researchPersonID: researchPersonID,
		researchPersonOwner: researchPersonOwner, publicSourceID: publicSourceID, privateSourceID: privateSourceID,
		publicPlatformAlias: publicPlatformAlias, publicPublicAlias: publicPublicAlias, publicPrivateAlias: publicPrivateAlias,
		researchPlatformAlias: researchPlatformAlias, researchPrivateAlias: researchPrivateAlias,
		readerID: reader, researcherID: researcher, researcherSourceOwner: researcherSourceOwner,
	}
}

// aliasesFor reads the workspace as one actor would and returns the alias list
// for one person.
func aliasesFor(t *testing.T, fixture *testsupport.Fixture, seeded aliasFixture, actorID string, personID uuid.UUID) []string {
	t.Helper()
	snapshot, err := NewService(fixture.Pool(), nil).Workspace(fixture.Ctx(), WorkspaceInput{
		QuestionID: seeded.questionID.String(),
		EntityType: "person",
		EntityID:   personID.String(),
	}, actorID)
	if err != nil {
		t.Fatalf("workspace as %s: %v", actorID, err)
	}
	return snapshot.Context.EntityAliasesAR
}

func hasAlias(values []string, want string) bool { return slices.Contains(values, want) }

// TestTheWorkspaceHidesAnAliasTakenFromAPrivateSource is the first of the two
// directions, on a person the actor may read.
//
// A registered reader with no roles reads a workspace about a published person.
// The person is public, so the reader may read the person; the alias taken from a
// private source is still not theirs, because on this page that spelling IS the
// disclosure of a source the reader cannot open.
func TestTheWorkspaceHidesAnAliasTakenFromAPrivateSource(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedWorkspaceAliasFixture(t, fixture)

	seen := aliasesFor(t, fixture, seeded, seeded.readerID.String(), seeded.publicPersonID)

	if hasAlias(seen, seeded.publicPrivateAlias) {
		t.Fatalf("the workspace showed the alias taken from a private source to a reader with no roles: %v", seen)
	}
	// The two that are legitimately readable, and in particular the platform
	// record: an alias with no source is a row the platform wrote, and losing it
	// here would be a different defect.
	if !hasAlias(seen, seeded.publicPlatformAlias) {
		t.Fatalf("the workspace dropped the platform's own alias, which carries no source and is not a private disclosure: %v", seen)
	}
	if !hasAlias(seen, seeded.publicPublicAlias) {
		t.Fatalf("the workspace dropped an alias taken from a PUBLIC source on a public person: %v", seen)
	}
	if len(seen) != 2 {
		t.Fatalf("the workspace returned %v, want exactly the platform alias and the public-source alias", seen)
	}
}

// TestTheWorkspaceHidesAResearchOnlyPersonsAliases is the second direction.
//
// The subject here is a person no role can read: not published, not owned by the
// actor, not on a tree the actor collaborates on. A research role is explicitly
// NOT a blanket bypass over the people table, so its aliases must not appear -
// and a person the actor may not read does not become readable because the
// reader lists its other spellings.
//
// The owner of the person is the control: the same person, read by the actor who
// created it, which the person policy does grant.
func TestTheWorkspaceHidesAResearchOnlyPersonsAliases(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedWorkspaceAliasFixture(t, fixture)

	// A registered reader with no role, a researcher, and the source's owner:
	// three actors, none of whom created the person.
	for name, actorID := range map[string]uuid.UUID{
		"a reader with no role":      seeded.readerID,
		"a researcher":               seeded.researcherID,
		"the private source's owner": seeded.researcherSourceOwner,
	} {
		seen := aliasesFor(t, fixture, seeded, actorID.String(), seeded.researchPersonID)
		if len(seen) != 0 {
			t.Fatalf("%s read the aliases of a research-only person: %v. A person the actor may not read must not arrive through its other spellings", name, seen)
		}
	}

	// The control: the person the actor created is readable, and its aliases come
	// with it. Without this the test above would also pass if the alias read were
	// simply broken.
	ownerView := aliasesFor(t, fixture, seeded, seeded.researchPersonOwner.String(), seeded.researchPersonID)
	if !hasAlias(ownerView, seeded.researchPlatformAlias) {
		t.Fatalf("the owner of the person did not get its platform alias, so the alias read is broken rather than scoped: %v", ownerView)
	}
	// The owner of the person is not the owner of the private source, so the
	// spelling taken from that source stays out of even this view.
	if hasAlias(ownerView, seeded.researchPrivateAlias) {
		t.Fatalf("the person owner was shown a spelling taken from a source they do not own: %v", ownerView)
	}
}

// TestAResearchRoleKeepsTheViewOfTheAliasesItMayRead is the "unchanged" half.
//
// The fix is scoped, so it must not take anything away from the role that was
// supposed to have this view in the first place. A research role's source grants
// include a blanket pass, so a researcher reading a workspace about a PUBLIC
// person sees every spelling of it, including the one from the private source -
// exactly as they did before the predicates were added. What a researcher does
// lose is the aliases of a person they may not read, which is the other test.
func TestAResearchRoleKeepsTheViewOfTheAliasesItMayRead(t *testing.T) {
	fixture := testsupport.New(t)
	seeded := seedWorkspaceAliasFixture(t, fixture)

	seen := aliasesFor(t, fixture, seeded, seeded.researcherID.String(), seeded.publicPersonID)

	for _, want := range []string{seeded.publicPlatformAlias, seeded.publicPublicAlias, seeded.publicPrivateAlias} {
		if !hasAlias(seen, want) {
			t.Fatalf("a research role lost %q from a public person's aliases, which it may read: %v. The scoping must not take away the view this role had", want, seen)
		}
	}
	if len(seen) != 3 {
		t.Fatalf("a research role read %v, want all three spellings of a person it may read", seen)
	}
}

// TestAFamilySubjectsAliasesAreUnscopedByDesign says out loud what the person
// gates do not apply to.
//
// family_aliases carries no source column and no alias_type column, so there is
// nothing for the alias policy to scope and the rows follow their family. This
// is internal/dictionary's own stated reason, and it is here so a later reader
// who sees the asymmetry with person_aliases knows it was checked rather than
// missed.
func TestAFamilySubjectsAliasesAreUnscopedByDesign(t *testing.T) {
	fixture := testsupport.New(t)
	unique := fixture.Unique("familyalias")
	familyID := uuid.New()
	fixture.Exec(`INSERT INTO families (id, canonical_name_ar, normalized_name_ar, visibility) VALUES ($1, $2, $3, 'public')`,
		familyID, "بيت "+unique, "بيت "+unique)
	fixture.Exec(`INSERT INTO family_aliases (family_id, value_ar, normalized_value_ar) VALUES ($1, $2, $3)`, familyID, "بيت آخر "+unique, "بيت آخر")

	// An actor with no roles, which is the scope this assertion is about: a
	// family whose aliases carry no source are readable by anyone who can reach
	// the workspace, exactly as they were before the person policy was added.
	reader := uuid.New()
	fixture.Exec(`INSERT INTO users (id, email, display_name_ar) VALUES ($1, $2, 'قارئ عائلة')`, reader, unique+"-familyreader@example.test")
	fixture.Exec(`INSERT INTO user_roles (user_id, role) VALUES ($1, 'registered')`, reader)

	snapshot, err := NewService(fixture.Pool(), nil).Workspace(fixture.Ctx(), WorkspaceInput{
		QuestionID: seedWorkspaceQuestionFor(t, fixture, unique),
		EntityType: "family",
		EntityID:   familyID.String(),
	}, reader.String())
	if err != nil {
		t.Fatalf("workspace for a family: %v", err)
	}
	if !hasAlias(snapshot.Context.EntityAliasesAR, "بيت آخر "+unique) {
		t.Fatalf("a family's alias was scoped away, and family_aliases has no source column to scope: %v", snapshot.Context.EntityAliasesAR)
	}
}

func seedWorkspaceQuestionFor(t *testing.T, fixture *testsupport.Fixture, unique string) string {
	t.Helper()
	questionID := uuid.New()
	if err := fixture.QueryRow(`INSERT INTO open_questions (id, title_ar, status, priority) VALUES ($1, $2, 'open', 'normal') RETURNING id`,
		questionID, "سؤال عائلة "+unique).Scan(&questionID); err != nil {
		t.Fatalf("insert open question: %v", err)
	}
	return questionID.String()
}
