package types

import "testing"

// TestDefaultAgentSettings_MaxIterations pins the default ReAct-loop
// ceiling at 20. A brief 15-iter experiment (reverted the same day)
// showed the ceiling is hard-cut — no Fallback S1 rescue when the
// LLM is still actively tool-calling — so shaving iterations off the
// default is a quality risk for single-topic questions (no multi-topic
// scaling backup). If this assertion needs to move, update the
// rationale in config.go + the codrax.yaml.example comment in lockstep.
func TestDefaultAgentSettings_MaxIterations(t *testing.T) {
	if got := DefaultAgentSettings().MaxIterations; got != 20 {
		t.Errorf("MaxIterations default = %d, want 20", got)
	}
}
