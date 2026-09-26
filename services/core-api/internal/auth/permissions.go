package auth

type Role string

const (
	RolePublic       Role = "public"
	RoleRegistered   Role = "registered"
	RoleTreeOwner    Role = "tree_owner"
	RoleCollaborator Role = "collaborator"
	RoleResearcher   Role = "researcher"
	RoleModerator    Role = "moderator"
	RoleAdmin        Role = "admin"
)

type Permission string

const (
	TreeRead         Permission = "tree.read"
	TreeCreate       Permission = "tree.create"
	TreeEdit         Permission = "tree.edit"
	TreePublish      Permission = "tree.publish"
	TreeInvite       Permission = "tree.invite"
	ClaimCreate      Permission = "claim.create"
	ClaimReview      Permission = "claim.review"
	SourceCreate     Permission = "source.create"
	SourceReview     Permission = "source.review"
	QuestionManage   Permission = "question.manage"
	ModerationReview Permission = "moderation.review"
	// IdentityWrite gates every read and every write of a global identity row:
	// people, person aliases, families, tribes, branches, places and generic
	// entity relationships. The rows are shared by every tree, so the gate is a
	// platform role rather than a relationship to one interpretation. It is also
	// the gate on the reads, so the identity API cannot become a new public read
	// path for rows the dictionary only publishes selectively.
	IdentityWrite Permission = "identity.write"
)

type Actor struct {
	UserID string
	Roles  []Role
}

type Resource struct {
	Type                   string
	OwnerID                string
	Visibility             string
	CollaboratorIDs        []string
	CollaboratorPermission string
	VersionState           string
	CanEditResearch        bool
}

func (a Actor) hasRole(role Role) bool {
	for _, current := range a.Roles {
		if current == role {
			return true
		}
	}
	return false
}

func (a Actor) isCollaborator(resource Resource) bool {
	if a.UserID == "" {
		return false
	}
	for _, collaboratorID := range resource.CollaboratorIDs {
		if collaboratorID == a.UserID {
			return true
		}
	}
	return false
}

func Can(actor Actor, permission Permission, resource Resource) bool {
	isOwner := actor.UserID != "" && actor.UserID == resource.OwnerID
	isCollaborator := actor.isCollaborator(resource)
	isPublic := resource.Visibility == "public"
	isDraft := resource.VersionState == "draft"
	canEditAsCollaborator := isCollaborator && resource.CollaboratorPermission == "edit" && isDraft
	canReviewAsCollaborator := isCollaborator && (resource.CollaboratorPermission == "edit" || resource.CollaboratorPermission == "review")
	hasRegisteredRole := actor.hasRole(RoleRegistered) || actor.hasRole(RoleTreeOwner) || actor.hasRole(RoleCollaborator) || actor.hasRole(RoleResearcher) || actor.hasRole(RoleModerator) || actor.hasRole(RoleAdmin)
	hasResearchRole := actor.hasRole(RoleResearcher) || actor.hasRole(RoleModerator) || actor.hasRole(RoleAdmin)
	hasModerationRole := actor.hasRole(RoleModerator) || actor.hasRole(RoleAdmin)

	if permission == TreeRead {
		return isPublic || isOwner || isCollaborator
	}
	if permission == TreeCreate || permission == SourceCreate || permission == ClaimCreate {
		return hasRegisteredRole
	}
	if permission == TreeEdit {
		return isDraft && (isOwner || canEditAsCollaborator)
	}
	if permission == TreePublish {
		return isDraft && isOwner
	}
	if permission == TreeInvite {
		return isOwner
	}
	if permission == ClaimReview || permission == SourceReview {
		return hasResearchRole || (resource.CanEditResearch && (isOwner || canReviewAsCollaborator))
	}
	if permission == QuestionManage {
		return hasResearchRole || isOwner || canReviewAsCollaborator
	}
	if permission == ModerationReview {
		return hasModerationRole
	}
	if permission == IdentityWrite {
		// A global identity row belongs to no single interpretation. Owning a tree,
		// or being a collaborator on one, is a right over that tree's draft and
		// says nothing about the shared rows every other tree reads, so neither
		// ownership nor collaboration reaches this gate. The role set is exactly the
		// one the suggestions change set already requires for the same tables
		// (person aliases and entity relationships), so this API cannot open a write
		// path that service would refuse. A research role is not a blanket bypass
		// either: it authorises the write, never the publication of a person, which
		// stays with a published tree version.
		return actor.hasRole(RoleCollaborator) || hasResearchRole
	}
	return false
}
