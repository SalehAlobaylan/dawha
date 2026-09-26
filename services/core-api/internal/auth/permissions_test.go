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

// A global identity row belongs to no single interpretation, so the identity gate
// reads the platform role and nothing else. The cases below are the matrix the
// identity write surface depends on: a right over one tree must not become a
// right over the shared rows every tree reads, and creating a row must not make
// its creator its editor.
func TestIdentityWriteIgnoresTreeScopedRights(t *testing.T) {
	ownedTree := Resource{Type: "tree", OwnerID: "user-1", Visibility: "private", VersionState: "draft"}
	collaboratedTree := Resource{
		Type: "tree", OwnerID: "user-1", Visibility: "private",
		CollaboratorIDs: []string{"user-2"}, CollaboratorPermission: "edit", VersionState: "draft",
	}
	cases := []struct {
		name    string
		actor   Actor
		allowed bool
	}{
		{name: "anonymous", actor: Actor{}, allowed: false},
		{name: "registered without a global role", actor: Actor{UserID: "user-1", Roles: []Role{RoleRegistered}}, allowed: false},
		{name: "tree owner without a global role", actor: Actor{UserID: "user-1", Roles: []Role{RoleTreeOwner, RoleRegistered}}, allowed: false},
		{name: "edit collaborator without a global role", actor: Actor{UserID: "user-2", Roles: []Role{RoleRegistered}}, allowed: false},
		{name: "platform collaborator", actor: Actor{UserID: "user-2", Roles: []Role{RoleCollaborator}}, allowed: true},
		{name: "researcher", actor: Actor{UserID: "user-3", Roles: []Role{RoleResearcher}}, allowed: true},
		{name: "moderator", actor: Actor{UserID: "user-4", Roles: []Role{RoleModerator}}, allowed: true},
		{name: "admin", actor: Actor{UserID: "user-5", Roles: []Role{RoleAdmin}}, allowed: true},
	}
	for _, testCase := range cases {
		if got := Can(testCase.actor, IdentityWrite, Resource{Type: "person"}); got != testCase.allowed {
			t.Fatalf("%s: Can(IdentityWrite) = %t, want %t", testCase.name, got, testCase.allowed)
		}
	}
	// The two tree-scoped relationships above are the ones the plan 002 lesson is
	// about: they are genuinely granted by the same policy, and genuinely do not
	// reach an identity row.
	if !Can(Actor{UserID: "user-1", Roles: []Role{RoleTreeOwner}}, TreeEdit, ownedTree) {
		t.Fatal("expected the tree owner to hold the tree-scoped edit right the matrix denies it for identity")
	}
	if !Can(Actor{UserID: "user-2", Roles: []Role{RoleCollaborator}}, TreeEdit, collaboratedTree) {
		t.Fatal("expected the tree collaborator to hold the tree-scoped edit right the matrix denies it for identity")
	}
}

// A record's creator gains no extra power over it: the row the next tree reads is
// the same row, so ownership would make the first writer the only one who could
// ever correct a typo.
func TestIdentityWriteIgnoresRecordOwnership(t *testing.T) {
	actor := Actor{UserID: "user-1", Roles: []Role{RoleRegistered}}

	if Can(actor, IdentityWrite, Resource{Type: "person", OwnerID: "user-1"}) {
		t.Fatal("expected creating a person not to make its creator an identity writer")
	}
}
