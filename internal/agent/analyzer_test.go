package agent

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAnalyzerParseOutputCapturesSummary verifies that the analyzer's
// ParseOutput packs the LLM's free-form text into Data so the trace
// retains the human-readable classification rationale, and that it
// does not touch a non-empty Mutable.TaskList (i.e. respects whatever
// todo_write set during the ReAct loop).
func TestAnalyzerParseOutputCapturesSummary(t *testing.T) {
	e := &analyzerEvaluator{}

	// Pretend a previous todo_write call already populated the list.
	mut := types.NewMutableState(types.TaskList{
		Objective:     "explain the project",
		CurrentTaskID: "t1",
		Tasks: []types.TaskItem{{
			ID: "t1", Title: "explain", Writing: false, Status: types.TaskInProgress,
		}},
	})

	ctx := &types.AgentContext{
		Stage:   types.StageAnalyze,
		Mutable: mut,
	}
	msgs := []llm.Message{
		{Role: "assistant", Content: "Classified as analysis: pure question."},
	}

	out, err := e.ParseOutput(ctx, msgs, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Data should still capture the LLM's natural-language summary.
	var data map[string]string
	if err := json.Unmarshal(out.Data, &data); err != nil {
		t.Fatalf("Data not valid JSON: %v", err)
	}
	if data["result"] == "" {
		t.Error("Data.result should contain the assistant's summary")
	}

	// Mutable.TaskList must be unchanged — analyzer should not clobber
	// what todo_write put there.
	tl := mut.TaskList()
	if len(tl.Tasks) != 1 || tl.Tasks[0].ID != "t1" {
		t.Errorf("Mutable.TaskList was clobbered, got %+v", tl.Tasks)
	}
}

// TestAnalyzerParseOutputFailSafe verifies that when the LLM fails to
// call todo_write — leaving Mutable.TaskList empty — the analyzer
// installs a single Analysis-typed task as a safety net so the
// orchestrator routes to the analysis policy instead of falling
// through into nothing.
func TestAnalyzerParseOutputFailSafe(t *testing.T) {
	e := &analyzerEvaluator{}

	// Empty Mutable: simulates "LLM never called todo_write"
	mut := types.NewMutableState(types.TaskList{Objective: "what does this do?"})

	ctx := &types.AgentContext{
		Stage:   types.StageAnalyze,
		Mutable: mut,
	}
	msgs := []llm.Message{
		{Role: "assistant", Content: "I'm thinking about it..."},
	}

	out, err := e.ParseOutput(ctx, msgs, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}

	tl := mut.TaskList()
	if len(tl.Tasks) != 1 {
		t.Fatalf("fail-safe should install exactly 1 task, got %d", len(tl.Tasks))
	}
	if tl.Tasks[0].Writing {
		t.Errorf("fail-safe task should be read-only (Writing=false), got Writing=true")
	}
	if tl.Tasks[0].HighRisk {
		t.Errorf("fail-safe task should not be high-risk")
	}
	if tl.CurrentTask() == nil {
		t.Error("CurrentTaskID and Tasks must agree")
	}
	if tl.Objective != "what does this do?" {
		t.Errorf("Objective should be preserved, got %q", tl.Objective)
	}
}

// TestAnalyzerParseOutputNoMutable verifies the evaluator does not
// crash if Mutable is nil (defensive — production always has it set,
// but tests should not).
func TestAnalyzerParseOutputNoMutable(t *testing.T) {
	e := &analyzerEvaluator{}
	ctx := &types.AgentContext{Stage: types.StageAnalyze}
	msgs := []llm.Message{
		{Role: "assistant", Content: "ok"},
	}
	out, err := e.ParseOutput(ctx, msgs, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
}
