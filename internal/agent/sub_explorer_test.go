package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSubExplorerFactoryDoesNotStoreSharedBaseAgent(t *testing.T) {
	sub := NewSubExplorer(&Dependencies{})
	if sub == nil || sub.deps == nil {
		t.Fatal("NewSubExplorer should keep dependencies for per-run BaseAgent construction")
	}
	if _, ok := reflect.TypeOf(*sub).FieldByName("base"); ok {
		t.Fatal("SubExplorer must not store a shared BaseAgent; Run needs a fresh evaluator per request")
	}
}

// TestSubExplorer_CrossRunResetOnObjectiveChange locks the F2-style
// evaluator reset behavior for callers that exercise the evaluator directly. See
// memory/project_followups_from_repl_audit.md item #1 and
// memory/project_repl_equivalence_audit.md for the original explorer
// fix this mirrors. Each SubAgentRequest is an independent scoped
// investigation, so state from a previous Run() must not leak into
// the next one's notes / evidence. The old test also checked that
// idleStreak / lastToolCount counters were reset on cross-run —
// those counters moved to LoopPolicy and are rebuilt per dispatch,
// so the evaluator no longer owns them.
func TestSubExplorer_CrossRunResetOnObjectiveChange(t *testing.T) {
	eval := &subExplorerEvaluator{
		objective:          "find registration points",
		investigationNotes: []string{"[DIRECT] stale note from prior run"},
		structuredEvidence: []types.EvidenceItem{{Summary: "stale"}},
		flowFindings:       []types.FlowFindingDigest{{ID: "stale-flow"}},
	}

	ctx := &types.AgentContext{
		Objective:   "trace config loader",
		Constraints: []string{"internal/config"},
	}
	prompt := eval.BuildInitialInstruction(ctx, nil)

	if len(eval.investigationNotes) != 0 {
		t.Errorf("investigationNotes not reset: %v", eval.investigationNotes)
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
// objective is seen twice in a row by one evaluator instance, accumulated
// notes are NOT wiped. Run() now creates a fresh evaluator per request, but the
// evaluator contract stays useful for direct tests and any future scoped
// single-investigation reuse. structuredEvidence and flowFindings are still
// cleared unconditionally (pre-existing behaviour — they're rebuilt from tool
// results each Execute).
func TestSubExplorer_SameObjectiveKeepsState(t *testing.T) {
	eval := &subExplorerEvaluator{
		objective:          "trace config loader",
		investigationNotes: []string{"[DIRECT] keep me"},
	}

	ctx := &types.AgentContext{
		Objective:   "trace config loader",
		Constraints: []string{"internal/config"},
	}
	_ = eval.BuildInitialInstruction(ctx, nil)

	if len(eval.investigationNotes) != 1 {
		t.Errorf("notes wiped on same-objective call: %v", eval.investigationNotes)
	}
}

func TestSubExplorerAdvisoryNotes_BoundsAndLabelsNoToolProse(t *testing.T) {
	req := &types.SubAgentRequest{
		ID:        "trace-st-1",
		SubAgent:  "explorer",
		Objective: "inventory scoped package",
		Scope:     []string{"pkg/a", "pkg/b"},
	}
	long := strings.Repeat("x", subExplorerAdvisoryMaxNoteBytes+32)
	got := subExplorerAdvisoryNotes(req, []string{
		"old note",
		"rich JSON inventory",
		long,
	}, "rich JSON inventory")

	if len(got) != 2 {
		t.Fatalf("notes len=%d, want 2 after recent-window dedupe: %+v", len(got), got)
	}
	if !strings.Contains(got[0], "Sub-agent advisory result (not a citation)") ||
		!strings.Contains(got[0], "request_id=trace-st-1") ||
		!strings.Contains(got[0], "rich JSON inventory") {
		t.Fatalf("note missing advisory metadata/content:\n%s", got[0])
	}
	if strings.Count(strings.Join(got, "\n"), "rich JSON inventory") != 1 {
		t.Fatalf("duplicate stage report was not deduped: %+v", got)
	}
	if !strings.Contains(got[len(got)-1], "[advisory note truncated]") {
		t.Fatalf("long advisory note was not explicitly truncated")
	}
}

func TestSubAgentReducer_CarriesBoundedAdvisoryNotes(t *testing.T) {
	results := []*types.SubAgentResult{
		{RequestID: "a", InvestigationNotes: []string{"note a", "note b"}},
		{RequestID: "b", InvestigationNotes: []string{"note b", "note c"}},
	}

	out := (&SubAgentReducer{}).Reduce(results)
	if got, want := len(out.InvestigationNotes), 3; got != want {
		t.Fatalf("InvestigationNotes len=%d, want %d: %+v", got, want, out.InvestigationNotes)
	}
	if strings.Join(out.InvestigationNotes, "|") != "note a|note b|note c" {
		t.Fatalf("InvestigationNotes order/dedupe mismatch: %+v", out.InvestigationNotes)
	}
}
