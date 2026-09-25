package collaboration

import (
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport/actor"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/trees"
	"github.com/jackc/pgx/v5/pgxpool"
)

// sharedTree creates a draft tree owned by the returned owner, which is the
// precondition every invitation journey needs.
func sharedTree(t *testing.T, fixture *testsupport.Fixture, owner actor.Actor) string {
	t.Helper()
	detail, err := trees.NewService(fixture.Pool()).CreateTree(fixture.Ctx(), owner.User.ID, trees.CreateTreeInput{
		Name:       "شجرة " + fixture.Unique("t"),
		Visibility: "public",
		People:     []trees.PersonInput{{CanonicalName: "شخص " + fixture.Tag()}},
	})
	if err != nil {
		t.Fatalf("create tree: %v", err)
	}
	return detail.Tree.ID
}

func newService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool, Trees: trees.NewService(pool)}
}

func TestInvitedCollaboratorCanEditTheDraft(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب")
	guest := actor.Register(t, fixture, "متدعٍ")
	treeID := sharedTree(t, fixture, owner)
	service := newService(fixture.Pool())

	created, err := service.CreateInvitation(fixture.Ctx(), treeID, owner.User.ID, InviteInput{
		InviteeUserID:   guest.User.ID,
		PermissionLevel: "edit",
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if created.Invitation.Status != "pending" {
		t.Fatalf("invitation status = %s, want pending", created.Invitation.Status)
	}
	if created.AcceptPath == "" || created.AcceptToken == "" {
		t.Fatalf("invitation %+v carries no accept path or token", created)
	}
	// The token is handed to the invitee once, and the database keeps only its
	// hash, so the stored row cannot be replayed.
	if got := fixture.Count(`SELECT count(*) FROM tree_invitations WHERE token_hash = $1 AND token_hash <> $2`,
		auth.HashToken(created.AcceptToken), created.AcceptToken); got != 1 {
		t.Fatalf("stored invitation rows = %d, want 1 keyed by a hash", got)
	}

	accepted, err := service.AcceptInvitation(fixture.Ctx(), created.AcceptToken, guest.User.ID)
	if err != nil {
		t.Fatalf("accept invitation: %v", err)
	}
	if accepted.TreeID != treeID || accepted.PermissionLevel != "edit" {
		t.Fatalf("accepted = %+v, want the invited tree at edit level", accepted)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_collaborators WHERE tree_id = $1 AND user_id = $2`, treeID, guest.User.ID); got != 1 {
		t.Fatalf("collaborator rows = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_invitations WHERE id = $1 AND status = 'accepted'`, created.Invitation.ID); got != 1 {
		t.Fatalf("accepted invitation rows = %d, want 1", got)
	}
	// An accepted invitation cannot be replayed into a second membership.
	if _, err := service.AcceptInvitation(fixture.Ctx(), created.AcceptToken, guest.User.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("second accept = %v, want ErrConflict", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_collaborators WHERE tree_id = $1 AND user_id = $2`, treeID, guest.User.ID); got != 1 {
		t.Fatalf("collaborator rows after a replayed accept = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_change_log WHERE tree_id = $1 AND action = 'invitation_accepted'`, treeID); got != 1 {
		t.Fatalf("invitation_accepted change log rows = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM audit_log WHERE action = 'invitation_accepted' AND entity_id = $1`, guest.User.ID); got != 1 {
		t.Fatalf("invitation_accepted audit rows = %d, want 1", got)
	}
}

func TestInvitationByEmailAcceptsOnlyTheAddressedResearcher(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب")
	invitee := actor.Register(t, fixture, "المدعو")
	stranger := actor.Register(t, fixture, "غير المدعو")
	treeID := sharedTree(t, fixture, owner)
	service := newService(fixture.Pool())

	created, err := service.CreateInvitation(fixture.Ctx(), treeID, owner.User.ID, InviteInput{
		InviteeEmail:    invitee.Email,
		PermissionLevel: "view",
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if _, err := service.AcceptInvitation(fixture.Ctx(), created.AcceptToken, stranger.User.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("accept by a stranger = %v, want ErrNotFound", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_collaborators WHERE tree_id = $1`, treeID); got != 0 {
		t.Fatalf("collaborator rows after a refused accept = %d, want 0", got)
	}
	if _, err := service.AcceptInvitation(fixture.Ctx(), created.AcceptToken, invitee.User.ID); err != nil {
		t.Fatalf("accept by the addressed researcher: %v", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_collaborators WHERE tree_id = $1 AND user_id = $2`, treeID, invitee.User.ID); got != 1 {
		t.Fatalf("collaborator rows = %d, want 1", got)
	}
}

func TestRevokedInvitationCannotBeAccepted(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب")
	guest := actor.Register(t, fixture, "متدعٍ")
	treeID := sharedTree(t, fixture, owner)
	service := newService(fixture.Pool())

	created, err := service.CreateInvitation(fixture.Ctx(), treeID, owner.User.ID, InviteInput{
		InviteeUserID:   guest.User.ID,
		PermissionLevel: "edit",
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if err := service.RevokeInvitation(fixture.Ctx(), treeID, created.Invitation.ID, owner.User.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_invitations WHERE id = $1 AND status = 'revoked'`, created.Invitation.ID); got != 1 {
		t.Fatalf("revoked invitation rows = %d, want 1", got)
	}
	if _, err := service.AcceptInvitation(fixture.Ctx(), created.AcceptToken, guest.User.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("accept a revoked invitation = %v, want ErrConflict", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_collaborators WHERE tree_id = $1`, treeID); got != 0 {
		t.Fatalf("collaborator rows = %d, want 0", got)
	}
	// Revoking twice is refused rather than rewriting a decision already made.
	if err := service.RevokeInvitation(fixture.Ctx(), treeID, created.Invitation.ID, owner.User.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("second revoke = %v, want ErrConflict", err)
	}
}

func TestOnlyTheOwnerCanInviteOrRevoke(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب")
	stranger := actor.Register(t, fixture, "غريب")
	guest := actor.Register(t, fixture, "متدعٍ")
	treeID := sharedTree(t, fixture, owner)
	service := newService(fixture.Pool())

	if _, err := service.CreateInvitation(fixture.Ctx(), treeID, stranger.User.ID, InviteInput{
		InviteeUserID: guest.User.ID,
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("invite by a stranger = %v, want ErrForbidden", err)
	}
	created, err := service.CreateInvitation(fixture.Ctx(), treeID, owner.User.ID, InviteInput{
		InviteeUserID: guest.User.ID,
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if err := service.RevokeInvitation(fixture.Ctx(), treeID, created.Invitation.ID, stranger.User.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoke by a stranger = %v, want ErrForbidden", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_invitations WHERE id = $1 AND status = 'pending'`, created.Invitation.ID); got != 1 {
		t.Fatalf("the refused revoke changed the invitation: pending rows = %d, want 1", got)
	}
}

func TestRemovingACollaboratorRevokesTheirAccess(t *testing.T) {
	fixture := testsupport.New(t)
	owner := actor.RegisterAndLogin(t, fixture, "صاحب")
	guest := actor.Register(t, fixture, "متدعٍ")
	treeID := sharedTree(t, fixture, owner)
	service := newService(fixture.Pool())

	created, err := service.CreateInvitation(fixture.Ctx(), treeID, owner.User.ID, InviteInput{
		InviteeUserID:   guest.User.ID,
		PermissionLevel: "review",
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if _, err := service.AcceptInvitation(fixture.Ctx(), created.AcceptToken, guest.User.ID); err != nil {
		t.Fatalf("accept invitation: %v", err)
	}
	response, err := service.ListCollaborators(fixture.Ctx(), treeID, owner.User.ID)
	if err != nil {
		t.Fatalf("list collaborators: %v", err)
	}
	if len(response.Collaborators) != 1 || response.Collaborators[0].UserID != guest.User.ID {
		t.Fatalf("collaborators = %+v, want the accepted guest", response.Collaborators)
	}
	if err := service.RemoveCollaborator(fixture.Ctx(), treeID, guest.User.ID, owner.User.ID); err != nil {
		t.Fatalf("remove collaborator: %v", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_collaborators WHERE tree_id = $1 AND user_id = $2`, treeID, guest.User.ID); got != 0 {
		t.Fatalf("collaborator rows after removal = %d, want 0", got)
	}
	// The tree itself is untouched: removing a member is not deleting the tree.
	if got := fixture.Count(`SELECT count(*) FROM trees WHERE id = $1`, treeID); got != 1 {
		t.Fatalf("tree rows after a collaborator removal = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM tree_change_log WHERE tree_id = $1 AND action = 'collaborator_removed'`, treeID); got != 1 {
		t.Fatalf("collaborator_removed change log rows = %d, want 1", got)
	}
}
