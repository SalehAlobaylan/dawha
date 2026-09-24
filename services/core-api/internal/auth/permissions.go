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
)

type Actor struct {
	UserID string
	Roles  []Role
}

type Resource struct {
	Type            string
	OwnerID         string
	Visibility      string
	CollaboratorIDs []string
	CanEditResearch bool
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
	isPublic := resource.Visibility == "public" || resource.Visibility == "unlisted"
	hasRegisteredRole := actor.hasRole(RoleRegistered) || actor.hasRole(RoleTreeOwner) || actor.hasRole(RoleCollaborator) || actor.hasRole(RoleResearcher) || actor.hasRole(RoleModerator) || actor.hasRole(RoleAdmin)
	hasResearchRole := actor.hasRole(RoleResearcher) || actor.hasRole(RoleModerator) || actor.hasRole(RoleAdmin)
	hasModerationRole := actor.hasRole(RoleModerator) || actor.hasRole(RoleAdmin)

	if permission == TreeRead {
		return isPublic || isOwner || isCollaborator || hasResearchRole
	}
	if permission == TreeCreate || permission == SourceCreate || permission == ClaimCreate {
		return hasRegisteredRole
	}
	if permission == TreeEdit {
		return isOwner || isCollaborator || hasResearchRole
	}
	if permission == TreePublish || permission == TreeInvite {
		return isOwner
	}
	if permission == ClaimReview || permission == SourceReview {
		return hasResearchRole || (resource.CanEditResearch && (isOwner || isCollaborator))
	}
	if permission == QuestionManage {
		return hasResearchRole || isOwner || isCollaborator
	}
	if permission == ModerationReview {
		return hasModerationRole
	}
	return false
}
