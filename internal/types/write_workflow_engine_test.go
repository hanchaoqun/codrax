package types

import "testing"

func TestNormalizeWriteWorkflowEngineAlwaysController(t *testing.T) {
	for _, raw := range []string{"", "legacy", "LEGACY", "unknown", " controller "} {
		if got := NormalizeWriteWorkflowEngine(raw); got != WriteWorkflowEngineController {
			t.Fatalf("NormalizeWriteWorkflowEngine(%q) = %q, want controller", raw, got)
		}
	}
}
