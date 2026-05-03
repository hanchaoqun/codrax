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
		Language:            "en",
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
		Language:            "en",
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
// rest of the write pipeline. WriteAnalysisIR is nil afterwards but
// downstream stages (planner / coder / verifier) still run.
func TestWriteAnalyze_DegradesOnEmitFailure(t *testing.T) {
	ir := dagIR(types.AnswerContract{
		Language:            "en",
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
	// is non-fatal: the pipeline still reaches the plan stage.
	if !bus.TaskState.IsTerminal {
		t.Error("Run should terminate even when write_analyze degrades")
	}
	if bus.Mutable.WriteAnalysisIR() != nil {
		t.Error("WriteAnalysisIR should be nil after a failed emit")
	}
}
