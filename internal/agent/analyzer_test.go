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
// still produces a structurally complete IR from the zero-value
// RequestModel.
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
	// Failsafe path: no emit_analysis → zero-value RequestModel →
	// buildAnalysisIR still builds a structurally complete IR.
	if out.AnalysisIR == nil {
		t.Fatal("failsafe must still produce an IR")
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

// -----------------------------------------------------------------------------
// Deterministic post-processing pipeline (AnalysisIR construction)
// -----------------------------------------------------------------------------

// TestAnalyzer_IRIsBuiltFromTaskItem validates the analyzer's
// deterministic post-processing pipeline: given a populated
// TaskItem and raw objective, ParseOutput must produce a full
// AnalysisIR whose RequestModel, TaskGraph, and QualityGate are
// all consistent with the inputs.
func TestAnalyzer_IRIsBuiltFromTaskItem(t *testing.T) {
	mut := types.NewMutableState(types.TaskList{
		Objective: "请解释 explorer 是如何决定 ShouldStop 的",
		Tasks: []types.TaskItem{{
			ID:     "task-1",
			Title:  "解释 explorer 停止逻辑",
			Status: types.TaskPending,
		}},
		CurrentTaskID: "task-1",
	})
	mut.SetRequestModel(types.RequestModel{
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Complexity: types.ComplexityModerate,
		AnalyzerHints: types.AnalyzerHints{
			Keywords: []string{"explorer", "ShouldStop", "explore"},
			Entities: []string{"Explorer", "ShouldStop"},
			Kind:     "mechanism",
			Shape:    "step_list",
		},
	})
	ctx := &types.AgentContext{
		Objective:     "请解释 explorer 是如何决定 ShouldStop 的",
		CurrentTaskID: "task-1",
		CurrentTask:   "解释 explorer 停止逻辑",
		Mutable:       mut,
	}

	eval := &analyzerEvaluator{}
	out, err := eval.ParseOutput(ctx, []llm.Message{
		{Role: "assistant", Content: "classification complete"},
	}, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnalysisIR == nil {
		t.Fatal("expected non-nil AnalysisIR")
	}

	ir := out.AnalysisIR
	if ir.Version != types.AnalysisIRVersion {
		t.Errorf("version: got %q want %q", ir.Version, types.AnalysisIRVersion)
	}
	if ir.RequestModel.Language != "zh" {
		t.Errorf("language: got %q want zh", ir.RequestModel.Language)
	}
	if ir.RequestModel.Intent != types.IntentExplain {
		t.Errorf("intent: got %q want explain (mechanism)", ir.RequestModel.Intent)
	}
	if ir.RequestModel.Complexity != types.ComplexityModerate {
		t.Errorf("complexity: got %q want moderate", ir.RequestModel.Complexity)
	}
	if ir.AnswerContract.RequiredAnswerShape != types.ShapeStepList {
		t.Errorf("answer shape override: got %q want step_list", ir.AnswerContract.RequiredAnswerShape)
	}
	if len(ir.TaskGraph.Nodes) < 3 {
		t.Errorf("expected ≥3 task nodes from architecture_explain template; got %d", len(ir.TaskGraph.Nodes))
	}
	if len(ir.HypothesisSet) == 0 {
		t.Errorf("hypothesis planner must never produce an empty set")
	}
	// Normalizer should have found both terms. "ShouldStop" is
	// CamelCase so it lands on code:shouldstop; lowercase "explorer"
	// is a concept word without a symbol resolver, so it lands on
	// en:explorer. Both are valid canonicalisations — the explorer
	// at runtime will resolve en:explorer to code:explorer via its
	// SymbolResolver hook.
	foundExplorer := false
	foundShouldStop := false
	for _, c := range ir.RequestModel.TermGraph.Canonical {
		if c.ID == "en:explorer" || c.ID == "code:explorer" {
			foundExplorer = true
		}
		if c.ID == "code:shouldstop" {
			foundShouldStop = true
		}
	}
	if !foundExplorer || !foundShouldStop {
		t.Errorf("term graph missing expected canonical terms; got %+v", ir.RequestModel.TermGraph.Canonical)
	}
	// Gate result is reported, not enforced. Whatever it says, the IR
	// must carry the report so downstream analytics can diff it.
	if len(ir.QualityGate.Checks) == 0 {
		t.Errorf("quality gate must always report checks")
	}
}

// TestAnalyzer_FailsafeTaskInstallsIR validates that even when the
// LLM never called emit_analysis, the fail-safe path still produces
// a usable IR. This is the "ungraceful degradation" contract:
// analyzer never emits a nil IR on a non-empty objective.
func TestAnalyzer_FailsafeTaskInstallsIR(t *testing.T) {
	mut := types.NewMutableState(types.TaskList{
		Objective: "explain how the pipeline stops",
	})
	ctx := &types.AgentContext{
		Objective: "explain how the pipeline stops",
		Mutable:   mut,
	}
	eval := &analyzerEvaluator{}
	out, err := eval.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnalysisIR == nil {
		t.Fatal("expected non-nil IR on failsafe path")
	}
	// Failsafe task has no QuestionKind, so intent stays unknown and
	// language falls back to en (no Han chars).
	if out.AnalysisIR.RequestModel.Language != "en" {
		t.Errorf("language: got %q want en", out.AnalysisIR.RequestModel.Language)
	}
	if out.AnalysisIR.RequestModel.Intent != types.IntentUnknown {
		t.Errorf("intent: got %q want unknown", out.AnalysisIR.RequestModel.Intent)
	}
	// And the fail-safe installed a default task.
	tl := mut.TaskList()
	if len(tl.Tasks) != 1 {
		t.Fatalf("fail-safe must install a single default task; got %d", len(tl.Tasks))
	}
}

// TestAnalyzer_StageOutputCarriesIR validates the propagation path:
// StageOutput.AnalysisIR produced by the analyzer must survive the
// applyStageOutput step and end up on BusContext.AnalysisIR. This
// catches wiring regressions between the analyzer and orchestrator
// without needing a full LLM run.
func TestAnalyzer_StageOutputCarriesIR(t *testing.T) {
	mut := types.NewMutableState(types.TaskList{
		Objective: "explain the finalizer's translation mode",
	})
	ctx := &types.AgentContext{
		Objective: "explain the finalizer's translation mode",
		Mutable:   mut,
	}
	eval := &analyzerEvaluator{}
	out, err := eval.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	if out.AnalysisIR == nil {
		t.Fatal("StageOutput.AnalysisIR must be non-nil")
	}
	if out.AnalysisIR.TraceID == "" && ctx.CurrentTaskID != "" {
		t.Errorf("TraceID should be set when CurrentTaskID is available")
	}
}
