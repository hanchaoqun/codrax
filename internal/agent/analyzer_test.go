package agent

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAnalyzerParseOutputCapturesSummary verifies that the analyzer's
// ParseOutput packs the LLM's free-form text into Data (under "result")
// alongside the structured classification fields derived from an
// emit_analysis-emitted RequestModel, and that it does not touch a
// non-empty Mutable.TaskList.
func TestAnalyzerParseOutputCapturesSummary(t *testing.T) {
	e := &analyzerEvaluator{}

	// Pretend a previous emit_analysis call already populated the
	// RequestModel carrier. Under v3 the analyzer reads its hints
	// from MutableState.RequestModel(), not a separate carrier.
	mut := types.NewMutableState(types.TaskList{
		Objective:     "explain the project",
		CurrentTaskID: "t1",
		Tasks: []types.TaskItem{{
			ID:     "t1",
			Title:  "explain",
			Status: types.TaskInProgress,
		}},
	})
	mut.SetRequestModel(types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		AnalyzerHints: types.AnalyzerHints{
			Kind:     "mechanism",
			Shape:    "step_list",
			Keywords: []string{"orchestrator", "agent", "pipeline"},
			Entities: []string{"Orchestrator", "BaseAgent"},
		},
	})

	ctx := &types.AgentContext{
		Stage:   types.StageAnalyze,
		Mutable: mut,
	}
	msgs := []llm.Message{
		{Role: "assistant", Content: "Classified as mechanism: ordered steps."},
	}

	out, err := e.ParseOutput(ctx, msgs, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(out.Data, &data); err != nil {
		t.Fatalf("Data not valid JSON: %v", err)
	}
	if s, _ := data["result"].(string); s == "" {
		t.Error("Data.result should contain the assistant's summary")
	}
	if got, _ := data["question_kind"].(string); got != "mechanism" {
		t.Errorf("Data.question_kind = %q, want mechanism", got)
	}
	if got, _ := data["answer_shape"].(string); got != "step_list" {
		t.Errorf("Data.answer_shape = %q, want step_list", got)
	}
	// JSON numbers decode as float64 in map[string]any.
	if got, _ := data["entity_count"].(float64); got != 2 {
		t.Errorf("Data.entity_count = %v, want 2", got)
	}
	if got, _ := data["keyword_count"].(float64); got != 3 {
		t.Errorf("Data.keyword_count = %v, want 3", got)
	}

	// Mutable.TaskList must be unchanged — analyzer should not clobber
	// what the bootstrap or a previous dispatch put there.
	tl := mut.TaskList()
	if len(tl.Tasks) != 1 || tl.Tasks[0].ID != "t1" {
		t.Errorf("Mutable.TaskList was clobbered, got %+v", tl.Tasks)
	}
	if rm := mut.RequestModel(); rm == nil || rm.AnalyzerHints.Kind != "mechanism" {
		t.Errorf("RequestModel was clobbered, got %+v", rm)
	}
}

// TestAnalyzerParseOutputFailSafe verifies that when the LLM fails to
// call emit_analysis — leaving Mutable.TaskList empty and no
// RequestModel carrier — the analyzer installs a single task as a
// safety net so the orchestrator routes to the analysis policy
// instead of falling through into nothing, and buildAnalysisIR
// synthesises a read-only RunPolicy from the zero-value RequestModel.
func TestAnalyzerParseOutputFailSafe(t *testing.T) {
	e := &analyzerEvaluator{}

	// Empty Mutable: simulates "LLM never called emit_analysis"
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
	if tl.CurrentTask() == nil {
		t.Error("CurrentTaskID and Tasks must agree")
	}
	if tl.Objective != "what does this do?" {
		t.Errorf("Objective should be preserved, got %q", tl.Objective)
	}
	// Failsafe path: no todo_write → Classification stays zero →
	// buildAnalysisIR derives a read-only RunPolicy.
	if out.AnalysisIR == nil {
		t.Fatal("failsafe must still produce an IR")
	}
	if out.AnalysisIR.RunPolicy.Writing {
		t.Error("fail-safe RunPolicy should be read-only")
	}
	if out.AnalysisIR.RunPolicy.RequireDesignReview || out.AnalysisIR.RunPolicy.RequireCodeReview {
		t.Error("fail-safe RunPolicy should not force reviews")
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
