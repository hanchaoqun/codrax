package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFreezeRunPolicyFromAnalysis(t *testing.T) {
	cfg := defaultResolvedConfig()
	ar, sr, sar := buildRegistries(nil)
	o := New(cfg, ar, sr, sar)
	o.busCtx = &types.BusContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RiskMatrix: types.RiskMatrix{Security: types.RiskDimension{Level: 4}},
			},
		},
		Policy: types.PolicyContext{},
	}

	o.freezeRunPolicyFromAnalysis()
	if !o.busCtx.Policy.PolicyLocked {
		t.Fatal("expected policy to be locked")
	}
	if !o.busCtx.Policy.RequireReview || !o.busCtx.Policy.RequireVerification {
		t.Fatalf("unexpected frozen policy: %#v", o.busCtx.Policy)
	}
}

func TestFilterByRunPolicy_BlocksFinalizeBeforeMandatoryStages(t *testing.T) {
	cfg := defaultResolvedConfig()
	ar, sr, sar := buildRegistries(nil)
	o := New(cfg, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Policy: types.PolicyContext{
			RunPolicy: types.RunPolicy{MandatoryStages: []types.PipelineStage{types.StagePlan}},
		},
		Signals: types.ExecutionSignals{},
	}
	o.stageVisits = map[types.PipelineStage]int{}

	in := []types.Transition{
		{From: types.StageExplore, To: types.StageFinalize, Priority: 100},
		{From: types.StageExplore, To: types.StageExplore, Priority: 50},
	}
	out := o.filterByRunPolicy(in)
	if len(out) != 1 || out[0].To != types.StageExplore {
		t.Fatalf("expected finalize to be filtered, got %#v", out)
	}
}
