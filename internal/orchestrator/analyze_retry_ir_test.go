package orchestrator

import (
	"io"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunAnalyzePhase_PromotesCleanRetryIR(t *testing.T) {
	stale := dagIR(types.AnswerContract{Language: "en"})
	stale.RequestModel.Intent = types.IntentExplain
	stale.TaskGraph.Nodes[0].Objective = "stale graph from rejected attempt"

	clean := dagIR(types.AnswerContract{Language: "en"})
	clean.RequestModel.Intent = types.IntentRootCause
	clean.TaskGraph.Nodes[0].Objective = "clean graph from accepted retry"

	var calls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			calls++
			if calls == 1 {
				return &agent.StageOutput{
					Error:      "analyzer quality gate rejected: stale test IR",
					AnalysisIR: stale,
				}, nil
			}
			return &agent.StageOutput{AnalysisIR: clean}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 2}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable: types.NewMutableState("which graph survives retry?"),
	}

	used, err := o.runAnalyzePhase()
	if err != nil {
		t.Fatalf("runAnalyzePhase: %v", err)
	}
	if used != 2 {
		t.Fatalf("used attempts = %d, want 2", used)
	}
	if calls != 2 {
		t.Fatalf("analyzer calls = %d, want 2", calls)
	}
	if o.busCtx.AnalysisIR != clean {
		t.Fatalf("busCtx.AnalysisIR was not promoted to the clean retry IR")
	}
	if got := o.busCtx.AnalysisIR.TaskGraph.Nodes[0].Objective; got != "clean graph from accepted retry" {
		t.Fatalf("TaskGraph came from %q, want clean retry graph", got)
	}
	if got := o.busCtx.AnalysisIR.RequestModel.Intent; got != types.IntentRootCause {
		t.Fatalf("RequestModel.Intent = %q, want clean retry intent", got)
	}
}

func TestRunAnalyzePhase_TransientRetryDoesNotConsumeSemanticBudget(t *testing.T) {
	clean := dagIR(types.AnswerContract{Language: "en"})
	clean.RequestModel.Intent = types.IntentRootCause

	var calls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			calls++
			if calls == 1 {
				return nil, io.ErrUnexpectedEOF
			}
			return &agent.StageOutput{AnalysisIR: clean}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 1}, ar, sr, sar)
	o.SetTransientRetryBudget(1)
	o.busCtx = &types.BusContext{
		Mode:    types.ModeRead,
		Mutable: types.NewMutableState("transient analyze retry"),
	}

	used, err := o.runAnalyzePhase()
	if err != nil {
		t.Fatalf("runAnalyzePhase: %v", err)
	}
	if used != 1 {
		t.Fatalf("semantic attempts used = %d, want 1", used)
	}
	if calls != 2 {
		t.Fatalf("analyzer dispatches = %d, want 2 (one transient + one semantic)", calls)
	}
	if o.busCtx.AnalysisIR != clean {
		t.Fatalf("busCtx.AnalysisIR was not promoted after transient retry")
	}
}

func TestRun_AnalyzeTransientExhaustionDoesNotDegradeToRecoveryIR(t *testing.T) {
	var analyzerCalls, explorerCalls, finalizerCalls int
	agentFns := map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentAnalyzer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			analyzerCalls++
			return nil, io.ErrUnexpectedEOF
		},
		types.AgentExplorer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			explorerCalls++
			return &agent.StageOutput{EvidenceItems: []types.EvidenceItem{{ID: "ev", Source: "src.go", LineStart: 1}}}, nil
		},
		types.AgentFinalizer: func(ctx *types.AgentContext, sk *skill.Config) (*agent.StageOutput, error) {
			finalizerCalls++
			return &agent.StageOutput{FinalAnswer: "should not be reached"}, nil
		},
	}
	ar, sr, sar := buildRegistries(agentFns)
	o := New(types.PipelineSettings{MaxRetriesPerStage: 3}, ar, sr, sar)
	o.SetTransientRetryBudget(1)
	o.SetMaxSteps(20)

	busCtx, err := o.Run("explain X", "/tmp/repo", "main")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if analyzerCalls != 2 {
		t.Fatalf("analyzer dispatches = %d, want 2 (initial + one transient retry)", analyzerCalls)
	}
	if explorerCalls != 0 || finalizerCalls != 0 {
		t.Fatalf("transport-failed analyze must not degrade into explore/finalize; explorer=%d finalizer=%d", explorerCalls, finalizerCalls)
	}
	if busCtx.AnalysisIR != nil {
		t.Fatalf("transport-failed analyze must not install degraded semantic IR")
	}
	if busCtx.TaskState.LastError == "" {
		t.Fatalf("LastError must explain the terminal transport failure")
	}
	if !strings.Contains(strings.ToLower(busCtx.TaskState.LastError), "connection") {
		t.Fatalf("LastError should be user-facing transport wording, got %q", busCtx.TaskState.LastError)
	}
	if busCtx.TaskState.SoftAnalyzerError == "" {
		t.Fatalf("SoftAnalyzerError should retain operator transport detail")
	}
}
