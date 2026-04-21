package types

import "testing"

// TestDefaultAgentSettings_MaxIterations pins the default ReAct-loop
// ceiling. 20 was the session-3 default; session 23 T2.2 dropped to
// 15 after field data showed explorer routinely completes in 6–13
// iterations and iter>15 activity is almost always redundant re-emit.
// If this assertion needs to move, update the rationale + the
// codrax.yaml.example comment in lockstep.
func TestDefaultAgentSettings_MaxIterations(t *testing.T) {
	if got := DefaultAgentSettings().MaxIterations; got != 15 {
		t.Errorf("MaxIterations default = %d, want 15", got)
	}
}
