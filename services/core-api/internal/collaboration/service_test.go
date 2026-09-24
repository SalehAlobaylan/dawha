package collaboration

import "testing"

func TestValidateInviteInputNormalizesTargetAndPermission(t *testing.T) {
	input, err := validateInviteInput(InviteInput{
		InviteeEmail:    "  Researcher@Example.com ",
		PermissionLevel: " edit ",
	})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.InviteeEmail != "researcher@example.com" || input.PermissionLevel != "edit" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
}

func TestValidateInviteInputRequiresExactlyOneTarget(t *testing.T) {
	cases := []InviteInput{
		{},
		{InviteeUserID: "00000000-0000-0000-0000-000000000001", InviteeEmail: "researcher@example.com"},
	}
	for _, input := range cases {
		if _, err := validateInviteInput(input); err != ErrValidation {
			t.Fatalf("expected validation error for %+v, got %v", input, err)
		}
	}
}

func TestValidateInviteInputRejectsInvalidPermissionAndEmail(t *testing.T) {
	if _, err := validateInviteInput(InviteInput{InviteeEmail: "researcher@example.com", PermissionLevel: "owner"}); err != ErrValidation {
		t.Fatalf("expected permission validation error, got %v", err)
	}
	if _, err := validateInviteInput(InviteInput{InviteeEmail: "not-an-email"}); err != ErrValidation {
		t.Fatalf("expected email validation error, got %v", err)
	}
}

func TestValidEmailRejectsWhitespace(t *testing.T) {
	if validEmail("researcher@example.com") != true {
		t.Fatal("expected valid email")
	}
	if validEmail("researcher@example.com ") {
		t.Fatal("expected whitespace email to be rejected")
	}
}
