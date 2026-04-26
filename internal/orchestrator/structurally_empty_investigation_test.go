package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

func testReadGraph() types.TaskGraph {
	return types.TaskGraph{
		Nodes: []types.TaskNode{
			{ID: "evidence", Type: types.NodeEvidence, Objective: "collect evidence"},
			{ID: "finalize", Type: types.NodeFinalize, Objective: "finalize answer"},
		},
		ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 1},
	}
}

func TestHandleStructurallyEmptyInvestigation_RequeuesExploreBeforeFinalize(t *testing.T) {
	graph := testReadGraph()
	state := newGraphState(graph)
	state.markDone("evidence")
	state.markRunning("finalize")

	o := &Orchestrator{
		emit: render.NopEmitter,
		busCtx: &types.BusContext{
			Mutable: func() *types.MutableState {
				mu := types.NewMutableState("where is explore_mid_loop_hint_budget defined")
				mu.SetTurnAArtifacts(types.TurnAArtifacts{UserQuestion: "where is explore_mid_loop_hint_budget defined"})
				return mu
			}(),
			AnalysisIR: &types.AnalysisIR{TaskGraph: graph},
		},
	}

	out, retryMsg, handled := o.handleStructurallyEmptyInvestigation(state, "finalize")
	if !handled {
		t.Fatal("expected structurally empty investigation to be handled")
	}
	if out != nil {
		t.Fatalf("retry path should not synthesize a final answer, got %+v", out)
	}
	if !strings.Contains(retryMsg, "investigation structurally empty") {
		t.Fatalf("retry message = %q, want structurally-empty explanation", retryMsg)
	}
	if state.status["evidence"] != nodeRequeued {
		t.Fatalf("evidence node status = %q, want requeued", state.status["evidence"])
	}
	if state.status["finalize"] != nodeRequeued {
		t.Fatalf("finalize node status = %q, want requeued", state.status["finalize"])
	}
	if state.retryUsed != 1 {
		t.Fatalf("retryUsed = %d, want 1", state.retryUsed)
	}
}

func TestHandleStructurallyEmptyInvestigation_FailsLoudWhenRetryBudgetSpent(t *testing.T) {
	graph := testReadGraph()
	state := newGraphState(graph)
	state.retryUsed = graph.ExecutionPolicy.RetryBudget

	o := &Orchestrator{
		emit: render.NopEmitter,
		busCtx: &types.BusContext{
			Mutable: func() *types.MutableState {
				mu := types.NewMutableState("where is explore_mid_loop_hint_budget defined")
				mu.SetTurnAArtifacts(types.TurnAArtifacts{UserQuestion: "where is explore_mid_loop_hint_budget defined"})
				return mu
			}(),
			AnalysisIR: &types.AnalysisIR{TaskGraph: graph},
		},
	}

	out, retryMsg, handled := o.handleStructurallyEmptyInvestigation(state, "finalize")
	if !handled {
		t.Fatal("expected structurally empty investigation to be handled")
	}
	if retryMsg != "" {
		t.Fatalf("retry message = %q, want empty on fail-loud path", retryMsg)
	}
	if out == nil {
		t.Fatal("expected fail-loud output when retry budget is exhausted")
	}
	if !strings.Contains(out.FinalAnswer, "investigation structurally empty") {
		t.Fatalf("final answer should explain structurally empty investigation, got %q", out.FinalAnswer)
	}
}

func TestHandleStructurallyEmptyInvestigation_IgnoresUnknownState(t *testing.T) {
	graph := testReadGraph()
	state := newGraphState(graph)

	o := &Orchestrator{
		emit: render.NopEmitter,
		busCtx: &types.BusContext{
			Mutable:    types.NewMutableState("where is explore_mid_loop_hint_budget defined"),
			AnalysisIR: &types.AnalysisIR{TaskGraph: graph},
		},
	}

	out, retryMsg, handled := o.handleStructurallyEmptyInvestigation(state, "finalize")
	if handled {
		t.Fatalf("unknown / unobserved investigation state should not be treated as structurally empty: out=%+v retryMsg=%q", out, retryMsg)
	}
}
