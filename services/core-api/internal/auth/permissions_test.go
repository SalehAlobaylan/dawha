package auth

import "testing"

func TestPublicCanReadPublicTree(t *testing.T) {
	allowed := Can(Actor{}, TreeRead, Resource{Type: "tree", Visibility: "public"})

	if !allowed {
		t.Fatal("expected public visitor to read public tree")
	}
}

func TestUnrelatedUserCannotEditTree(t *testing.T) {
	allowed := Can(
		Actor{UserID: "user-2", Roles: []Role{RoleRegistered}},
		TreeEdit,
		Resource{Type: "tree", OwnerID: "user-1", Visibility: "private", CollaboratorIDs: []string{"user-3"}},
	)

	if allowed {
		t.Fatal("expected unrelated registered user to be denied")
	}
}

func TestCollaboratorCanEditButNotPublish(t *testing.T) {
	resource := Resource{Type: "tree", OwnerID: "user-1", Visibility: "private", CollaboratorIDs: []string{"user-2"}, CollaboratorPermission: "edit", VersionState: "draft"}
	actor := Actor{UserID: "user-2", Roles: []Role{RoleCollaborator}}

	if !Can(actor, TreeEdit, resource) {
		t.Fatal("expected collaborator to edit")
	}
	if Can(actor, TreePublish, resource) {
		t.Fatal("expected collaborator not to publish")
	}
}

func TestViewCollaboratorCannotEdit(t *testing.T) {
	allowed := Can(
		Actor{UserID: "user-2", Roles: []Role{RoleCollaborator}},
		TreeEdit,
		Resource{Type: "tree", OwnerID: "user-1", Visibility: "private", CollaboratorIDs: []string{"user-2"}, CollaboratorPermission: "view", VersionState: "draft"},
	)
	if allowed {
		t.Fatal("expected view collaborator to be denied")
	}
}

func TestPublishedVersionCannotBeEdited(t *testing.T) {
	allowed := Can(
		Actor{UserID: "user-1", Roles: []Role{RoleTreeOwner}},
		TreeEdit,
		Resource{Type: "tree", OwnerID: "user-1", Visibility: "private", VersionState: "published"},
	)
	if allowed {
		t.Fatal("expected published version to be read-only")
	}
}

func TestResearcherDoesNotReceiveImplicitPrivateTreeAccess(t *testing.T) {
	allowed := Can(
		Actor{UserID: "researcher-1", Roles: []Role{RoleResearcher}},
		TreeRead,
		Resource{Type: "tree", OwnerID: "user-1", Visibility: "private"},
	)
	if allowed {
		t.Fatal("expected researcher to be denied private tree access")
	}
}

func TestUnlistedTreeIsNotPubliclyReadable(t *testing.T) {
	allowed := Can(Actor{}, TreeRead, Resource{Type: "tree", Visibility: "unlisted"})
	if allowed {
		t.Fatal("expected unlisted tree to require authorization")
	}
}

func TestResearcherCanReviewClaims(t *testing.T) {
	allowed := Can(Actor{UserID: "researcher-1", Roles: []Role{RoleResearcher}}, ClaimReview, Resource{Type: "claim"})

	if !allowed {
		t.Fatal("expected researcher to review claims")
	}
}
