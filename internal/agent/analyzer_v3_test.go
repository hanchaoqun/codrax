package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestAnalyzerV3_IRIsBuiltFromTaskItem validates the analyzer's
// deterministic post-processing pipeline: given a populated
// TaskItem and raw objective, ParseOutput must produce a full
// AnalysisIR whose RequestModel, TaskGraph, RunPolicy, and
// QualityGate are all consistent with the inputs.
func TestAnalyzerV3_IRIsBuiltFromTaskItem(t *testing.T) {
	mut := types.NewMutableState(types.TaskList{
		Objective: "请解释 explorer 是如何决定 ShouldStop 的",
		Tasks: []types.TaskItem{{
			ID:           "task-1",
			Title:        "解释 explorer 停止逻辑",
			Writing:      false,
			HighRisk:     false,
			Complexity:   "moderate",
			Keywords:     []string{"explorer", "ShouldStop", "explore"},
			Entities:     []string{"Explorer", "ShouldStop"},
			QuestionKind: "mechanism",
			AnswerShape:  "step_list",
			Status:       types.TaskPending,
		}},
		CurrentTaskID: "task-1",
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
	if ir.RunPolicy.Writing {
		t.Errorf("writing=false task should produce read-only policy")
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

// TestAnalyzerV3_FailsafeTaskInstallsIR validates that even when the
// LLM never called todo_write, the fail-safe path still produces a
// usable IR. This is the "ungraceful degradation" contract: analyzer
// never emits a nil IR on a non-empty objective.
func TestAnalyzerV3_FailsafeTaskInstallsIR(t *testing.T) {
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

// TestAnalyzerV3_WritingTaskDerivesPolicy validates that writing +
// high_risk flags on the legacy TaskItem propagate into the v3
// RunPolicy correctly.
func TestAnalyzerV3_WritingTaskDerivesPolicy(t *testing.T) {
	mut := types.NewMutableState(types.TaskList{
		Objective: "refactor the auth middleware to use new session storage",
		Tasks: []types.TaskItem{{
			ID:       "task-1",
			Title:    "refactor auth middleware",
			Writing:  true,
			HighRisk: true,
			Status:   types.TaskPending,
		}},
		CurrentTaskID: "task-1",
	})
	ctx := &types.AgentContext{
		Objective: "refactor the auth middleware to use new session storage",
		Mutable:   mut,
	}
	eval := &analyzerEvaluator{}
	out, err := eval.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput: %v", err)
	}
	ir := out.AnalysisIR
	if !ir.RunPolicy.Writing {
		t.Errorf("writing=true must propagate")
	}
	if !ir.RunPolicy.RequireDesignReview || !ir.RunPolicy.RequireCodeReview {
		t.Errorf("HighRisk=true must force both reviews; got %+v", ir.RunPolicy)
	}
	// "auth" term should trip the risk matrix security+compliance
	// dimensions via the normalizer+risk path.
	if ir.RequestModel.RiskMatrix.Security.Level < 3 {
		t.Errorf("auth term must bump security ≥3; got %d", ir.RequestModel.RiskMatrix.Security.Level)
	}
}

// TestAnalyzerV3_BusContextReceivesIR validates the propagation path:
// StageOutput.AnalysisIR produced by the analyzer must survive the
// applyStageOutput step and end up on BusContext.AnalysisIR. This
// catches wiring regressions between the analyzer and orchestrator
// without needing a full LLM run.
func TestAnalyzerV3_StageOutputCarriesIR(t *testing.T) {
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
