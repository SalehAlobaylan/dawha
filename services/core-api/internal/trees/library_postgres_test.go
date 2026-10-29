package trees

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
)

func TestWorkspaceAndPublicTreeCollectionsHaveDifferentBoundaries(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب العمل")
	other := actor.RegisterAndLogin(t, fixture, "ناشر آخر")
	service := NewService(fixture.Pool())
	baseline, err := service.ListPublicTrees(fixture.Ctx())
	if err != nil {
		t.Fatal(err)
	}
	create := func(user actor.Actor, visibility string) TreeDetail {
		t.Helper()
		detail, err := service.CreateTree(fixture.Ctx(), user.User.ID, CreateTreeInput{Name: fixture.Unique("شجرة"), Visibility: visibility})
		if err != nil {
			t.Fatal(err)
		}
		return detail
	}
	private := create(owner, "private")
	shared := create(other, "private")
	unlisted := create(other, "unlisted")
	publicDraft := create(other, "public")
	public := twoGenerationTree(t, fixture, other, fixture.Unique("منشورة"))
	sealedVersionID := public.SelectedVersion.ID
	if _, err := service.PublishLatestDraft(fixture.Ctx(), public.Tree.ID, other.User.ID, "نشر"); err != nil {
		t.Fatal(err)
	}
	fixture.Exec(`INSERT INTO tree_collaborators (tree_id, user_id, permission_level, invited_by) VALUES ($1, $2, 'view', $3)`, shared.Tree.ID, owner.User.ID, other.User.ID)

	workspace, err := service.ListWorkspaceTrees(fixture.Ctx(), owner.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(workspace) != 2 {
		t.Fatalf("workspace = %+v, want owned and explicitly shared trees", workspace)
	}
	for _, tree := range workspace {
		if tree.ID != private.Tree.ID && tree.ID != shared.Tree.ID {
			t.Fatalf("platform tree leaked into workspace: %+v", tree)
		}
	}
	anonymous, err := service.ListWorkspaceTrees(fixture.Ctx(), "")
	if err != nil || len(anonymous) != 0 {
		t.Fatalf("anonymous workspace = %+v, %v; want empty", anonymous, err)
	}
	published, err := service.ListPublicTrees(fixture.Ctx())
	if err != nil {
		t.Fatal(err)
	}
	if len(published) != len(baseline)+1 {
		t.Fatalf("public list = %+v; want exactly one additional published tree", published)
	}
	found := false
	for _, tree := range published {
		if tree.ID == private.Tree.ID || tree.ID == shared.Tree.ID || tree.ID == unlisted.Tree.ID || tree.ID == publicDraft.Tree.ID {
			t.Fatalf("non-public or unpublished tree entered discovery: %+v", tree)
		}
		if tree.ID == public.Tree.ID {
			found = true
			if tree.LatestVersionID != sealedVersionID || tree.LatestState != "published" {
				t.Fatalf("discovery selected a draft: %+v", tree)
			}
		}
	}
	if !found {
		t.Fatal("published tree was not discoverable")
	}
	// A publisher also has a newer draft. Workspace opens that draft; discovery
	// keeps pointing at the sealed version even for the publisher.
	publisherWorkspace, err := service.ListWorkspaceTrees(fixture.Ctx(), other.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, tree := range publisherWorkspace {
		if tree.ID == public.Tree.ID && (tree.LatestState != "draft" || tree.LatestVersionID == sealedVersionID) {
			t.Fatalf("publisher workspace selected the published version: %+v", tree)
		}
	}
	accessible, err := service.ListAccessibleTrees(fixture.Ctx(), owner.User.ID)
	if err != nil || len(accessible) != len(baseline)+3 {
		t.Fatalf("legacy accessible list = %+v, %v; want owned, shared, and public", accessible, err)
	}
}
