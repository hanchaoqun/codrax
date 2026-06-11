package repl

import (
	"strings"
	"testing"
)

// The "/mode plan" pointer on artifact-generation panels is gated on
// three typed turn-policy fields. These pins keep the gate precise:
// it must fire only on confidently classified file-artifact turns,
// and the line itself must stay advisory (no mode switching).
func TestOperationWritePipelineGuidanceGate(t *testing.T) {
	firing := TurnPolicy{
		OperationKind: "artifact_generation",
		TargetSurface: "file_artifact",
		Confidence:    0.85,
	}

	for _, lang := range []string{"zh", "en"} {
		got := operationWritePipelineGuidance(lang, firing)
		if got == "" {
			t.Fatalf("lang=%s: guidance empty for canonical firing policy", lang)
		}
		if !strings.Contains(got, "/mode plan") {
			t.Fatalf("lang=%s: guidance missing /mode plan pointer: %q", lang, got)
		}
	}

	zh := operationWritePipelineGuidance("zh", firing)
	if !strings.Contains(zh, "写管线") {
		t.Fatalf("zh guidance not localized: %q", zh)
	}
	en := operationWritePipelineGuidance("en", firing)
	if !strings.Contains(en, "write pipeline") {
		t.Fatalf("en guidance not localized: %q", en)
	}

	quiet := []struct {
		name   string
		policy TurnPolicy
	}{
		{"wrong kind", TurnPolicy{OperationKind: "computer_operation", TargetSurface: "file_artifact", Confidence: 0.9}},
		{"wrong surface", TurnPolicy{OperationKind: "artifact_generation", TargetSurface: "browser", Confidence: 0.9}},
		{"hesitant", TurnPolicy{OperationKind: "artifact_generation", TargetSurface: "file_artifact", Confidence: 0.5}},
		{"confidence unset", TurnPolicy{OperationKind: "artifact_generation", TargetSurface: "file_artifact"}},
		{"empty policy", TurnPolicy{}},
	}
	for _, tc := range quiet {
		if got := operationWritePipelineGuidance("zh", tc.policy); got != "" {
			t.Errorf("%s: expected quiet gate, got %q", tc.name, got)
		}
	}
}

// At exactly the floor the gate fires — the floor is inclusive so the
// taught canonical confidence (≈0.85) keeps a margin above it.
func TestOperationWritePipelineGuidanceFloorInclusive(t *testing.T) {
	p := TurnPolicy{
		OperationKind: "artifact_generation",
		TargetSurface: "file_artifact",
		Confidence:    operationWriteGuidanceConfidenceFloor,
	}
	if operationWritePipelineGuidance("en", p) == "" {
		t.Fatalf("gate must fire at the floor (inclusive)")
	}
}
