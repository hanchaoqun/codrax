package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestSubExplorer_CrossRunResetOnObjectiveChange locks the F2-style
// fix for the sub_explorer singleton. See
// memory/project_followups_from_repl_audit.md item #1 and
// memory/project_repl_equivalence_audit.md for the original explorer
// fix this mirrors. Each SubAgentRequest is an independent scoped
// investigation, so state from a previous Run() must not leak into
// the next one's notes / idle counters / evidence.
func TestSubExplorer_CrossRunResetOnObjectiveChange(t *testing.T) {
	eval := &subExplorerEvaluator{
		objective:          "find registration points",
		investigationNotes: []string{"[DIRECT] stale note from prior run"},
		idleStreak:         3,
		lastToolCount:      7,
		structuredEvidence: []types.EvidenceItem{{Summary: "stale"}},
		flowFindings:       []types.FlowFindingDigest{{ID: "stale-flow"}},
	}

	ctx := &types.AgentContext{
		Objective:   "trace config loader",
		Constraints: []string{"internal/config"},
	}
	prompt := eval.BuildInitialPrompt(ctx, nil)

	if len(eval.investigationNotes) != 0 {
		t.Errorf("investigationNotes not reset: %v", eval.investigationNotes)
	}
	if eval.idleStreak != 0 {
		t.Errorf("idleStreak not reset: %d", eval.idleStreak)
	}
	if eval.lastToolCount != 0 {
		t.Errorf("lastToolCount not reset: %d", eval.lastToolCount)
	}
	if len(eval.structuredEvidence) != 0 {
		t.Errorf("structuredEvidence not reset: %v", eval.structuredEvidence)
	}
	if len(eval.flowFindings) != 0 {
		t.Errorf("flowFindings not reset: %v", eval.flowFindings)
	}
	if eval.objective != "trace config loader" {
		t.Errorf("objective not updated: %q", eval.objective)
	}
	if !strings.Contains(prompt, "trace config loader") {
		t.Errorf("prompt missing new objective: %q", prompt)
	}
}

// TestSubExplorer_SameObjectiveKeepsState verifies that when the same
// SubAgentRequest objective is seen twice in a row (hypothetical
// intra-Run reuse), accumulated notes are NOT wiped. This preserves
// the existing contract for any caller that might reuse the evaluator
// within a single logical investigation. structuredEvidence and
// flowFindings are still cleared unconditionally (pre-existing
// behaviour — they're rebuilt from tool results each Execute).
func TestSubExplorer_SameObjectiveKeepsState(t *testing.T) {
	eval := &subExplorerEvaluator{
		objective:          "trace config loader",
		investigationNotes: []string{"[DIRECT] keep me"},
		idleStreak:         2,
		lastToolCount:      5,
	}

	ctx := &types.AgentContext{
		Objective:   "trace config loader",
		Constraints: []string{"internal/config"},
	}
	_ = eval.BuildInitialPrompt(ctx, nil)

	if len(eval.investigationNotes) != 1 {
		t.Errorf("notes wiped on same-objective call: %v", eval.investigationNotes)
	}
	if eval.idleStreak != 2 || eval.lastToolCount != 5 {
		t.Errorf("counters wiped on same-objective call: idle=%d lastTool=%d",
			eval.idleStreak, eval.lastToolCount)
	}
}
