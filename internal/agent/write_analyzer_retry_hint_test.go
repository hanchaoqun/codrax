package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestWriteAnalyzer_BuildInitialInstruction_RendersHint pins the
// commit 13 #1 fix: SetAnalyzerRetryHint set by runWriteAnalyzePhase
// must surface in the next dispatch's prompt as "## Previous
// attempt rejected".
func TestWriteAnalyzer_BuildInitialInstruction_RendersHint(t *testing.T) {
	mu := types.NewMutableState("test")
	mu.SetAnalyzerRetryHint("missing required field task.summary")
	ctx := &types.AgentContext{Mutable: mu}
	eval := &writeAnalyzerEvaluator{}
	got := eval.BuildInitialInstruction(ctx, nil)
	if !strings.Contains(got, "Previous attempt rejected") {
		t.Errorf("expected '## Previous attempt rejected' header; got %q", got)
	}
	if !strings.Contains(got, "missing required field task.summary") {
		t.Errorf("expected hint text in prompt; got %q", got)
	}
	// Consume-once: a second call should NOT re-render the hint
	// (Mutable.AnalyzerRetryHint was reset by the first call).
	got2 := eval.BuildInitialInstruction(ctx, nil)
	if got2 != "" {
		t.Errorf("second call should produce empty prompt (hint consumed); got %q", got2)
	}
}

// TestWriteAnalyzer_BuildInitialInstruction_NoHintReturnsEmpty
// pins the no-retry path: a fresh dispatch with no prior failure
// produces an empty supplement (skill owns the full prompt).
func TestWriteAnalyzer_BuildInitialInstruction_NoHintReturnsEmpty(t *testing.T) {
	mu := types.NewMutableState("test")
	ctx := &types.AgentContext{Mutable: mu}
	eval := &writeAnalyzerEvaluator{}
	if got := eval.BuildInitialInstruction(ctx, nil); got != "" {
		t.Errorf("no hint should yield empty supplement; got %q", got)
	}
}

// TestWriteAnalyzer_BuildInitialInstruction_NilSafe pins
// defensive paths.
func TestWriteAnalyzer_BuildInitialInstruction_NilSafe(t *testing.T) {
	eval := &writeAnalyzerEvaluator{}
	if got := eval.BuildInitialInstruction(nil, nil); got != "" {
		t.Errorf("nil ctx should yield empty supplement; got %q", got)
	}
	if got := eval.BuildInitialInstruction(&types.AgentContext{}, nil); got != "" {
		t.Errorf("nil mutable should yield empty supplement; got %q", got)
	}
}

func TestNewWriteAnalyzerAgent_CapsMaxIterations(t *testing.T) {
	deps := &Dependencies{MaxIterations: 100}
	a := NewWriteAnalyzerAgent(deps).(*writeAnalyzer)
	if a.base.deps.MaxIterations != writeAnalyzerIterCap {
		t.Fatalf("MaxIterations = %d, want capped %d", a.base.deps.MaxIterations, writeAnalyzerIterCap)
	}
}

func TestNewWriteAnalyzerAgent_PreservesLowerMaxIterations(t *testing.T) {
	deps := &Dependencies{MaxIterations: 3}
	a := NewWriteAnalyzerAgent(deps).(*writeAnalyzer)
	if a.base.deps.MaxIterations != 3 {
		t.Fatalf("MaxIterations = %d, want explicit lower cap 3", a.base.deps.MaxIterations)
	}
}

func TestWriteAnalyzer_FilterToolSchemas_AllowsInitialPrescan(t *testing.T) {
	eval := &writeAnalyzerEvaluator{}
	schemas := writeAnalyzerTestSchemas()

	got := eval.FilterToolSchemas(&types.AgentContext{}, schemas)
	if len(got) != len(schemas) {
		t.Fatalf("initial write_analyzer surface should be unchanged, got %+v", got)
	}
}

func TestWriteAnalyzer_FilterToolSchemas_EmitOnlyAfterPrescanBudget(t *testing.T) {
	eval := &writeAnalyzerEvaluator{}
	eval.ObserveToolResults(nil, LoopObservation{
		CurrentToolResults: []types.ToolResult{
			{ToolName: "repo_map", Success: true},
			{ToolName: "grep", Success: true},
			{ToolName: "read_file", Success: true},
			{ToolName: "list_files", Success: true},
			{ToolName: "emit_write_analysis", Success: false},
			{ToolName: "run_tests", Success: true},
			{ToolName: "repo_map", Success: false},
		},
	})

	got := eval.FilterToolSchemas(&types.AgentContext{}, writeAnalyzerTestSchemas())
	if len(got) != 1 || got[0].Name != "emit_write_analysis" {
		t.Fatalf("prescan budget exhaustion should leave only emit_write_analysis, got %+v", got)
	}
}

func TestWriteAnalyzer_BuildInitialInstruction_ResetsPrescanBudget(t *testing.T) {
	eval := &writeAnalyzerEvaluator{prescanToolCalls: writeAnalyzerPrescanToolBudget}
	eval.BuildInitialInstruction(&types.AgentContext{Mutable: types.NewMutableState("test")}, nil)

	got := eval.FilterToolSchemas(&types.AgentContext{}, writeAnalyzerTestSchemas())
	if len(got) != len(writeAnalyzerTestSchemas()) {
		t.Fatalf("new dispatch should reset write_analyzer prescan budget, got %+v", got)
	}
}

func writeAnalyzerTestSchemas() []llm.ToolSchema {
	return []llm.ToolSchema{
		{Name: "read_file"},
		{Name: "list_files"},
		{Name: "repo_map"},
		{Name: "grep"},
		{Name: "emit_write_analysis"},
	}
}
