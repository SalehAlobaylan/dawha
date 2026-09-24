package research

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestValidateWorkspaceInput(t *testing.T) {
	questionID := "80000000-0000-0000-0000-000000000001"
	personID := "10000000-0000-0000-0000-000000000001"
	value, err := validateWorkspaceInput(WorkspaceInput{QuestionID: questionID, EntityID: personID})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if value.EntityType != "person" {
		t.Fatalf("entity type = %q", value.EntityType)
	}
	for _, input := range []WorkspaceInput{
		{QuestionID: "not-a-uuid"},
		{QuestionID: questionID, EntityType: "person", EntityID: "not-a-uuid"},
		{QuestionID: questionID, EntityType: "person"},
		{QuestionID: questionID, EntityType: "planet", EntityID: personID},
	} {
		if _, err := validateWorkspaceInput(input); !errors.Is(err, ErrValidation) {
			t.Fatalf("expected validation error for %+v, got %v", input, err)
		}
	}
}

func TestBuildWorkspacePermissions(t *testing.T) {
	actorID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	creatorID := actorID
	permissions := buildWorkspacePermissions(actorID, creatorID, true, true)
	if !permissions.CanRunResearch || !permissions.CanCreateClaim || !permissions.CanManageSelectedQuestion || !permissions.CanReviewFinding || !permissions.CanMergeIdentity {
		t.Fatalf("unexpected permissions: %+v", permissions)
	}
	publicPermissions := buildWorkspacePermissions(uuid.Nil, uuid.Nil, false, false)
	if !publicPermissions.CanRunResearch || publicPermissions.CanCreateClaim || publicPermissions.CanReviewFinding {
		t.Fatalf("unexpected public permissions: %+v", publicPermissions)
	}
}

func TestWorkspaceRequiresDatabase(t *testing.T) {
	if _, err := (&Service{}).Workspace(context.Background(), WorkspaceInput{}, ""); !errors.Is(err, ErrDatabaseUnavailable) {
		t.Fatalf("expected database error, got %v", err)
	}
}
