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
	resource := Resource{Type: "tree", OwnerID: "user-1", Visibility: "private", CollaboratorIDs: []string{"user-2"}}
	actor := Actor{UserID: "user-2", Roles: []Role{RoleCollaborator}}

	if !Can(actor, TreeEdit, resource) {
		t.Fatal("expected collaborator to edit")
	}
	if Can(actor, TreePublish, resource) {
		t.Fatal("expected collaborator not to publish")
	}
}

func TestResearcherCanReviewClaims(t *testing.T) {
	allowed := Can(Actor{UserID: "researcher-1", Roles: []Role{RoleResearcher}}, ClaimReview, Resource{Type: "claim"})

	if !allowed {
		t.Fatal("expected researcher to review claims")
	}
}
