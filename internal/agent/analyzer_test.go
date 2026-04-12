package agent

import (
	"encoding/json"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAnalyzerParseOutputCapturesSummary verifies that the analyzer's
// ParseOutput packs the LLM's free-form text into Data (under "result")
// alongside the structured classification fields (question_kind,
// answer_shape, etc), and that it does not touch a non-empty
// Mutable.TaskList (i.e. respects whatever todo_write set during the
// ReAct loop).
func TestAnalyzerParseOutputCapturesSummary(t *testing.T) {
	e := &analyzerEvaluator{}

	// Pretend a previous todo_write call already populated the list
	// with a full analyzer classification.
	mut := types.NewMutableState(types.TaskList{
		Objective:     "explain the project",
		CurrentTaskID: "t1",
		Tasks: []types.TaskItem{{
			ID:           "t1",
			Title:        "explain",
			Writing:      false,
			Status:       types.TaskInProgress,
			QuestionKind: "mechanism",
			AnswerShape:  "step_list",
			Complexity:   "moderate",
			Entities:     []string{"Orchestrator", "BaseAgent"},
			Keywords:     []string{"orchestrator", "agent", "pipeline"},
		}},
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

	// Data should capture both the natural-language summary and the
	// structured classification. Use map[string]any to accommodate
	// the mixed string/int value types.
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
	// what todo_write put there.
	tl := mut.TaskList()
	if len(tl.Tasks) != 1 || tl.Tasks[0].ID != "t1" {
		t.Errorf("Mutable.TaskList was clobbered, got %+v", tl.Tasks)
	}
	if tl.Tasks[0].QuestionKind != "mechanism" {
		t.Errorf("QuestionKind was clobbered, got %q", tl.Tasks[0].QuestionKind)
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
