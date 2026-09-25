package trees

import (
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
)

// twoGenerationTree builds the smallest tree that can be published and forked:
// two people and the parent_of relation between them.
func twoGenerationTree(t *testing.T, fixture *testsupport.Fixture, owner actor.Actor, name string) TreeDetail {
	t.Helper()
	service := NewService(fixture.Pool())
	detail, err := service.CreateTree(fixture.Ctx(), owner.User.ID, CreateTreeInput{
		Name:       name,
		Visibility: "public",
		People: []PersonInput{
			{CanonicalName: "سعد بن عامر " + fixture.Tag(), Gender: "male", BirthDateFrom: "1100-01-01"},
			{CanonicalName: "خالد بن سعد " + fixture.Tag(), Gender: "male", BirthDateFrom: "1140-01-01"},
		},
	})
	if err != nil {
		t.Fatalf("create tree: %v", err)
	}
	if len(detail.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(detail.Nodes))
	}
	updated, err := service.AddRelationship(fixture.Ctx(), detail.Tree.ID, owner.User.ID, AddRelationshipInput{
		SubjectNodeID: detail.Nodes[1].ID,
		ObjectNodeID:  detail.Nodes[0].ID,
		Predicate:     "parent_of",
		Status:        "interpreted",
	})
	if err != nil {
		t.Fatalf("add relationship: %v", err)
	}
	return updated
}

func TestCreateTreeStartsADraftOwnedByTheCreator(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب الشجرة")
	service := NewService(fixture.Pool())

	detail, err := service.CreateTree(fixture.Ctx(), owner.User.ID, CreateTreeInput{
		Name:       "شجرة " + fixture.Unique("t"),
		Visibility: "public",
		People:     []PersonInput{{CanonicalName: "فرد " + fixture.Tag()}},
	})
	if err != nil {
		t.Fatalf("create tree: %v", err)
	}
	if detail.Tree.OwnerID != owner.User.ID {
		t.Fatalf("owner = %s, want %s", detail.Tree.OwnerID, owner.User.ID)
	}
	if detail.SelectedVersion.State != string(Draft) {
		t.Fatalf("initial version state = %s, want draft", detail.SelectedVersion.State)
	}
	if !detail.Permissions.CanEdit || !detail.Permissions.CanPublish {
		t.Fatalf("owner permissions = %+v, want edit and publish", detail.Permissions)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_versions WHERE tree_id = $1 AND state = 'draft'`, detail.Tree.ID); got != 1 {
		t.Fatalf("draft version rows = %d, want 1", got)
	}
	// Every change is written twice over: once to the audit log and once to the
	// tree change log, so a reader can retrace how an interpretation was built.
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'tree_created'`, detail.Tree.ID); got != 1 {
		t.Fatalf("tree_created audit rows = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_change_log WHERE tree_id = $1 AND action = 'tree_created'`, detail.Tree.ID); got != 1 {
		t.Fatalf("tree_created change log rows = %d, want 1", got)
	}
}

func TestPublishClosesTheDraftAndOpensTheNext(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "ناشر")
	detail := twoGenerationTree(t, fixture, owner, "شجرة "+fixture.Unique("t"))
	service := NewService(fixture.Pool())

	// Publishing returns the tree with the freshly opened draft selected, so the
	// published version has to be read out of the version list rather than off
	// the returned selection.
	published, err := service.PublishLatestDraft(fixture.Ctx(), detail.Tree.ID, owner.User.ID, "تفسير أول")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if published.SelectedVersion.State != string(Draft) {
		t.Fatalf("selected version after publishing = %s, want the new draft", published.SelectedVersion.State)
	}
	sealed := publishedVersion(t, published.Versions)
	if sealed.PublishedAt == nil {
		t.Fatal("published version has no published_at")
	}
	if sealed.PublicationNote != "تفسير أول" {
		t.Fatalf("publication note = %q, want the note the publisher gave", sealed.PublicationNote)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_versions WHERE tree_id = $1 AND state = 'published'`, detail.Tree.ID); got != 1 {
		t.Fatalf("published version rows = %d, want 1", got)
	}
	// Publishing opens the next draft so the interpretation stays editable, and
	// the published version stays immutable.
	if got := fixture.Count(`SELECT count(*) FROM tree_versions WHERE tree_id = $1 AND state = 'draft'`, detail.Tree.ID); got != 1 {
		t.Fatalf("draft versions after publishing = %d, want 1", got)
	}
	if _, err := service.AddPerson(fixture.Ctx(), detail.Tree.ID, owner.User.ID, PersonInput{CanonicalName: "لاحق " + fixture.Tag()}); err != nil {
		t.Fatalf("add person to the new draft: %v", err)
	}
	versions, err := service.ListVersions(fixture.Ctx(), detail.Tree.ID, owner.User.ID)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions = %d, want 2", len(versions))
	}
	if versions[0].Number != 2 || versions[0].State != string(Draft) {
		t.Fatalf("latest version = %+v, want draft number 2", versions[0])
	}
	immutable, err := service.GetTreeVersion(fixture.Ctx(), detail.Tree.ID, sealed.ID, owner.User.ID)
	if err != nil {
		t.Fatalf("read the published version: %v", err)
	}
	if len(immutable.Nodes) != 2 {
		t.Fatalf("published version nodes = %d, want the 2 it was published with", len(immutable.Nodes))
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'tree_version_published'`, detail.Tree.ID); got != 1 {
		t.Fatalf("publish audit rows = %d, want 1", got)
	}
}

func TestPublishRefusesAnUnrelatedUser(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب")
	stranger := actor.RegisterAndLogin(t, fixture, "غريب")
	detail := twoGenerationTree(t, fixture, owner, "شجرة "+fixture.Unique("t"))
	service := NewService(fixture.Pool())

	if _, err := service.PublishLatestDraft(fixture.Ctx(), detail.Tree.ID, stranger.User.ID, "محاولة"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("publish by a stranger = %v, want ErrForbidden", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_versions WHERE tree_id = $1 AND state = 'published'`, detail.Tree.ID); got != 0 {
		t.Fatalf("published versions = %d, want 0 after a refused publish", got)
	}
}

func TestForkPublishedVersionCopiesInterpretationAndProvenance(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب الأصل")
	forker := actor.RegisterAndLogin(t, fixture, "مفريع")
	service := NewService(fixture.Pool())
	detail := twoGenerationTree(t, fixture, owner, "شجرة "+fixture.Unique("t"))
	afterPublish, err := service.PublishLatestDraft(fixture.Ctx(), detail.Tree.ID, owner.User.ID, "تفسير مرجعي")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	published := publishedVersion(t, afterPublish.Versions)

	forkName := "تفريع " + fixture.Unique("t")
	forked, err := service.ForkPublishedVersion(fixture.Ctx(), detail.Tree.ID, forker.User.ID, ForkTreeInput{
		VersionID:  published.ID,
		Name:       forkName,
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if forked.Tree.OwnerID != forker.User.ID {
		t.Fatalf("fork owner = %s, want %s", forked.Tree.OwnerID, forker.User.ID)
	}
	if forked.Tree.ParentTreeID != detail.Tree.ID || forked.Tree.ParentVersionID != published.ID {
		t.Fatalf("fork provenance = parent %s/%s, want %s/%s",
			forked.Tree.ParentTreeID, forked.Tree.ParentVersionID, detail.Tree.ID, published.ID)
	}
	if len(forked.Nodes) != len(detail.Nodes) || len(forked.Relationships) != len(detail.Relationships) {
		t.Fatalf("fork copied %d nodes and %d relationships, want %d and %d",
			len(forked.Nodes), len(forked.Relationships), len(detail.Nodes), len(detail.Relationships))
	}
	// The fork owns its own nodes, so editing the fork cannot reach back into
	// the tree it came from.
	if got := fixture.Count(`
		SELECT count(*) FROM tree_nodes original
		JOIN tree_nodes copy ON copy.id IN (
			SELECT id FROM tree_nodes WHERE tree_version_id = $2
		)
		WHERE original.tree_version_id = $1 AND original.id = copy.id`, published.ID, forked.SelectedVersion.ID); got != 0 {
		t.Fatalf("the fork shares %d node row(s) with its source", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_forks WHERE tree_id = $1 AND parent_version_id = $2`, forked.Tree.ID, published.ID); got != 1 {
		t.Fatalf("tree_forks rows = %d, want 1", got)
	}
	// A second fork of the same source version is refused rather than silently
	// producing a duplicate interpretation.
	if _, err := service.ForkPublishedVersion(fixture.Ctx(), detail.Tree.ID, forker.User.ID, ForkTreeInput{
		VersionID:  published.ID,
		Name:       "تفريع مكرر " + fixture.Unique("t"),
		Visibility: "private",
	}); !errors.Is(err, ErrForkConflict) {
		t.Fatalf("duplicate fork = %v, want ErrForkConflict", err)
	}
}

func TestForkRefusesAnUnpublishedVersion(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب")
	forker := actor.RegisterAndLogin(t, fixture, "مفريع")
	detail := twoGenerationTree(t, fixture, owner, "شجرة "+fixture.Unique("t"))
	service := NewService(fixture.Pool())

	if _, err := service.ForkPublishedVersion(fixture.Ctx(), detail.Tree.ID, forker.User.ID, ForkTreeInput{
		VersionID:  detail.SelectedVersion.ID,
		Name:       "تفريع مبكر " + fixture.Unique("t"),
		Visibility: "private",
	}); !errors.Is(err, ErrForkSourceNotPublished) {
		t.Fatalf("fork of a draft = %v, want ErrForkSourceNotPublished", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_forks WHERE parent_version_id = $1`, detail.SelectedVersion.ID); got != 0 {
		t.Fatalf("tree_forks rows = %d, want 0", got)
	}
}

// publishedVersion picks the one published version out of a version list. A
// tree has at most one, and a journey that cannot find it means publishing
// stopped working.
func publishedVersion(t *testing.T, versions []TreeVersionView) TreeVersionView {
	t.Helper()
	for _, version := range versions {
		if version.State == string(Published) {
			return version
		}
	}
	t.Fatalf("no published version in %+v", versions)
	return TreeVersionView{}
}
