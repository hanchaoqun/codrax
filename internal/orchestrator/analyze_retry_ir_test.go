package orchestrator

import (
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
