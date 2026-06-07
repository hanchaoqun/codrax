package dataworkflow

import "testing"

func TestGuardResultErrorTextPrefersMessage(t *testing.T) {
	result := NewGuardResult(
		"unfinished_validation_stage",
		"error",
		RepairNeedsTypedAction,
		"emit the next typed action",
		WorkflowViolation{Reason: "repair graph state"},
	)
	if result.Empty() {
		t.Fatal("guard result should not be empty")
	}
	if got, want := result.ErrorText(), "emit the next typed action"; got != want {
		t.Fatalf("ErrorText=%q, want %q", got, want)
	}
	if got, want := result.Reason, "emit the next typed action"; got != want {
		t.Fatalf("Reason=%q, want %q", got, want)
	}
}
