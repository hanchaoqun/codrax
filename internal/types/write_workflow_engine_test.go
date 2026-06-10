package types

import "testing"

func TestNormalizeWriteWorkflowEngineDefaultsLegacy(t *testing.T) {
	for _, raw := range []string{"", "legacy", "LEGACY", "unknown"} {
		if got := NormalizeWriteWorkflowEngine(raw); got != WriteWorkflowEngineLegacy {
			t.Fatalf("NormalizeWriteWorkflowEngine(%q) = %q, want legacy", raw, got)
		}
	}
	if got := NormalizeWriteWorkflowEngine(" controller "); got != WriteWorkflowEngineController {
		t.Fatalf("NormalizeWriteWorkflowEngine(controller) = %q", got)
	}
}
