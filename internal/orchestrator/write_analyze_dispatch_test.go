package orchestrator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestWriteAnalyze_DispatchedInPlanMode pins that ModePlan triggers
// a StageWriteAnalyze dispatch. The mock write_analyzer pretends the
// LLM successfully emitted by writing a synthetic IR onto Mutable;
// the test asserts the IR survives onto the BusContext after Run.
func TestWriteAnalyze_DispatchedInPlanMode(t *testing.T) {
	dispatchCount := 0
	ir := dagIR(types.AnswerContract{
		Language: "en",
	})
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentWriteAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			dispatchCount++
			ctx.Mutable.SetWriteAnalysisIR(&types.WriteAnalysisIR{
				Request: types.WriteRequestModel{
					RawRequest: "test request",
					Task: types.WriteTask{
						Kind:    types.WriteTaskFeature,
						Scope:   types.ScopeMicro,
						Summary: "synthesised by mock",
					},
					Risk: types.WriteRiskProfile{Overall: types.RiskBandLow},
				},
				PhaseProposal: types.PhaseProposal{Split: "single"},
			})
			return &agent.StageOutput{StageReport: "synth"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModePlan)
	o.SetAutoInitRepo(true)
	o.SetScaffoldEnabled(true)
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bus, err := o.Run("add a feature", repoRoot, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if dispatchCount != 1 {
		t.Errorf("write_analyzer dispatch count = %d; want 1", dispatchCount)
	}
	got := bus.Mutable.WriteAnalysisIR()
	if got == nil {
		t.Fatal("WriteAnalysisIR not on Mutable after plan-mode Run")
	}
	if got.Request.Task.Summary != "synthesised by mock" {
		t.Errorf("IR not preserved; got %+v", got.Request.Task)
	}
}

// TestWriteAnalyze_NotDispatchedInReadMode pins the L1 red line:
// read-mode Runs must not trigger StageWriteAnalyze. Even with a
// write_analyzer agent registered (the test fixture always
// registers all agents), the read path must skip it.
func TestWriteAnalyze_NotDispatchedInReadMode(t *testing.T) {
	dispatchCount := 0
	ir := dagIR(types.AnswerContract{
		Language: "en",
	})
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{MissingPiece: types.MissingFacts}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{
				MissingPiece: types.MissingNone,
				FinalAnswer:  "- `Foo` (file.go:1)",
			}, nil
		},
		types.AgentWriteAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			dispatchCount++
			t.Errorf("write_analyzer must not dispatch in ModeRead; sentinel fired")
			return &agent.StageOutput{}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	// Default mode (no SetMode call) coerces to ModeRead.
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bus, err := o.Run("explain X", repoRoot, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if bus.Mode != types.ModeRead {
		t.Errorf("Mode coerced to %q; want ModeRead", bus.Mode)
	}
	if dispatchCount != 0 {
		t.Errorf("write_analyzer dispatch count = %d in ModeRead; want 0", dispatchCount)
	}
	if bus.Mutable.WriteAnalysisIR() != nil {
		t.Error("WriteAnalysisIR should stay nil in read-mode Run")
	}
}

// TestWriteAnalyze_DegradesOnEmitFailure pins the non-fatal contract:
// when write_analyzer returns an error, the Run continues into the
// rest of the write pipeline. A conservative fallback WriteAnalysisIR
// is installed so the planner still sees write-task framing instead
// of inheriting read-analyzer diagnostic/root-cause framing.
func TestWriteAnalyze_DegradesOnEmitFailure(t *testing.T) {
	ir := dagIR(types.AnswerContract{
		Language: "en",
	})
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: dagAnalyzerFn(ir),
		types.AgentWriteAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			return &agent.StageOutput{Error: "synthetic emit failure"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.SetMaxSteps(20)
	o.SetMode(types.ModePlan)
	o.SetAutoInitRepo(true)
	o.SetScaffoldEnabled(true)
	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	bus, err := o.Run("add feature", repoRoot, "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Run must terminate cleanly; LastError gets populated by the
	// downstream planner stub (no ChangePlan produced) — that's
	// expected. The point of this test is that write_analyze failure
	// is non-fatal and still leaves typed write framing for planner.
	if !bus.TaskState.IsTerminal {
		t.Error("Run should terminate even when write_analyze degrades")
	}
	got := bus.Mutable.WriteAnalysisIR()
	if got == nil {
		t.Fatal("fallback WriteAnalysisIR should be installed after a failed emit")
	}
	if got.Request.Task.Kind != types.WriteTaskMisc {
		t.Fatalf("fallback kind = %q, want misc", got.Request.Task.Kind)
	}
	if got.Request.Task.Scope != types.ScopeMicro {
		t.Fatalf("fallback scope = %q, want micro", got.Request.Task.Scope)
	}
	if got.Request.RawRequest != "add feature" {
		t.Fatalf("fallback raw request = %q", got.Request.RawRequest)
	}
	if len(got.Request.Constraints) == 0 || got.Request.Constraints[0].Kind != "preserve_user_request" {
		t.Fatalf("fallback should carry a preservation constraint, got %+v", got.Request.Constraints)
	}
}

func TestFallbackWriteAnalysisIR_ProjectsTypedReadIRScope(t *testing.T) {
	readIR := dagIR(types.AnswerContract{Language: "en"})
	readIR.RequestModel.Intent = types.IntentRootCause
	readIR.RequestModel.Predicates.IsDiagnosticQuestion = true
	readIR.EvidencePlan.RequiredFiles = []string{
		"seaborn/_core.py",
		"./tests/../seaborn/_core.py",
		"/tmp/outside.py",
		"../escape.py",
	}
	readIR.RequestModel.AnalyzerHints.RequiredFileHints = []types.RequiredFileHint{
		{Path: "seaborn/_oldcore.py", Confidence: 0.9},
		{Path: "low_confidence.py", Confidence: 0.1},
	}

	got := fallbackWriteAnalysisIR("fix the crash", readIR, nil)
	if got.Request.Task.Kind != types.WriteTaskBugfix {
		t.Fatalf("Kind = %q, want bugfix", got.Request.Task.Kind)
	}
	if got.Request.Task.Scope != types.ScopePackage {
		t.Fatalf("Scope = %q, want package so controller can explore before planning", got.Request.Task.Scope)
	}
	wantAnchors := []string{"seaborn/_core.py", "seaborn/_oldcore.py"}
	if len(got.Request.ScopeAnchors) != len(wantAnchors) {
		t.Fatalf("ScopeAnchors = %+v, want %+v", got.Request.ScopeAnchors, wantAnchors)
	}
	for i, want := range wantAnchors {
		if got.Request.ScopeAnchors[i] != want {
			t.Fatalf("ScopeAnchors[%d] = %q, want %q (all=%+v)", i, got.Request.ScopeAnchors[i], want, got.Request.ScopeAnchors)
		}
	}
}

func TestRunWriteAnalyzePhase_FallbackReturnsNilAndClearsStageError(t *testing.T) {
	readIR := dagIR(types.AnswerContract{Language: "en"})
	readIR.RequestModel.Intent = types.IntentRootCause
	readIR.EvidencePlan.RequiredFiles = []string{"src/_pytest/assertion/rewrite.py"}
	dispatchCount := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			dispatchCount++
			return &agent.StageOutput{Error: "write_analyzer did not call emit_write_analysis within the ReAct loop"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	mu := types.NewMutableState("fix assertion rewrite")
	o.busCtx = &types.BusContext{
		Mode:       types.ModeApply,
		Mutable:    mu,
		AnalysisIR: readIR,
	}

	used, err := o.runWriteAnalyzePhase()
	if err != nil {
		t.Fatalf("fallback write analysis should recover nil error, got %v", err)
	}
	if used != 1 {
		t.Fatalf("used steps = %d, want 1 no-emit attempt before fallback", used)
	}
	if dispatchCount != 1 {
		t.Fatalf("dispatch count = %d, want 1 no-emit attempt before fallback", dispatchCount)
	}
	if got := o.busCtx.TaskState.LastError; got != "" {
		t.Fatalf("fallback should clear stage LastError, got %q", got)
	}
	got := mu.WriteAnalysisIR()
	if got == nil {
		t.Fatal("fallback WriteAnalysisIR should be installed")
	}
	if got.Request.Task.Kind != types.WriteTaskBugfix {
		t.Fatalf("fallback kind = %q, want bugfix", got.Request.Task.Kind)
	}
	if len(got.Request.ScopeAnchors) != 1 || got.Request.ScopeAnchors[0] != "src/_pytest/assertion/rewrite.py" {
		t.Fatalf("fallback anchors = %+v", got.Request.ScopeAnchors)
	}
}

func TestRunWriteAnalyzePhase_RetriesEmitRejectionBeforeFallback(t *testing.T) {
	readIR := dagIR(types.AnswerContract{Language: "en"})
	dispatchCount := 0
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			dispatchCount++
			return &agent.StageOutput{Error: "write_analyzer emit rejected: emit_write_analysis rejected: task.summary is empty"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	mu := types.NewMutableState("add option")
	o.busCtx = &types.BusContext{
		Mode:       types.ModeApply,
		Mutable:    mu,
		AnalysisIR: readIR,
	}

	used, err := o.runWriteAnalyzePhase()
	if err != nil {
		t.Fatalf("fallback write analysis should recover nil error, got %v", err)
	}
	if used != 2 {
		t.Fatalf("used steps = %d, want 2 attempts for emit rejection", used)
	}
	if dispatchCount != 2 {
		t.Fatalf("dispatch count = %d, want 2 attempts for emit rejection", dispatchCount)
	}
	if got := mu.WriteAnalysisIR(); got == nil {
		t.Fatal("fallback WriteAnalysisIR should be installed")
	}
}
