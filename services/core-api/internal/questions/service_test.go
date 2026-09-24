package questions

import "testing"

func TestValidateCreateQuestionDefaults(t *testing.T) {
	input, err := validateCreateQuestionInput(CreateQuestionInput{TitleAR: "  سؤال مهم  "})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.TitleAR != "سؤال مهم" || input.Status != "open" || input.Priority != "normal" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
}

func TestValidateQuestionInputRejectsInvalidStatus(t *testing.T) {
	if _, err := validateCreateQuestionInput(CreateQuestionInput{TitleAR: "سؤال", Status: "answered"}); err != ErrValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestValidateDisputeInputDefaults(t *testing.T) {
	input, err := validateDisputeInput(CreateDisputeInput{TitleAR: "  خلاف  "}, true)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if input.TitleAR != "خلاف" || input.Status != "open" {
		t.Fatalf("unexpected normalized input: %+v", input)
	}
}

func TestValidateDisputeResolution(t *testing.T) {
	if _, err := validateDisputeInput(CreateDisputeInput{TitleAR: "خلاف", Status: "resolved"}, true); err != ErrValidation {
		t.Fatalf("expected resolution validation error, got %v", err)
	}
}
