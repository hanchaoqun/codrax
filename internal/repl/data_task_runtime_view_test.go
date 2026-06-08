package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/dataworkflow"
)

func TestDataTaskWorkflowRuntimeViewPrefersRuntimeSnapshot(t *testing.T) {
	fallbackRecord := dataTaskWorkflowRecord{Err: "fallback"}
	fallbackCurrent := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "fallback_current", Kind: dataquery.DataActionInspectMaterial}}}
	fallbackDeferred := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "fallback_deferred", Kind: dataquery.DataActionInspectMaterial}}}

	rt := dataworkflow.NewWorkflowRuntime(dataquery.TaskPlan{})
	current := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "runtime_current", Kind: dataquery.DataActionExtractRecords}}}
	rt.SwitchCurrentPlan(3, "continue", current, "runtime current")
	rt.EnqueueDeferred(3, dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "runtime_deferred", Kind: dataquery.DataActionJoinRecords}}}, "next rank")
	rt.AppendRecord(dataTaskWorkflowRecord{Plan: current, Err: "runtime"})
	rt.SetRounds(4, 2)

	view := dataTaskWorkflowRuntimeViewFrom(rt, []dataTaskWorkflowRecord{fallbackRecord}, fallbackCurrent, fallbackDeferred, 1, 1)
	if len(view.Records) != 1 || view.Records[0].Err != "runtime" {
		t.Fatalf("Records=%+v, want runtime records", view.Records)
	}
	if len(view.CurrentPlan.Actions) != 1 || view.CurrentPlan.Actions[0].ID != "runtime_current" {
		t.Fatalf("CurrentPlan=%+v, want runtime current", view.CurrentPlan)
	}
	if len(view.DeferredPlan.Actions) != 1 || view.DeferredPlan.Actions[0].ID != "runtime_deferred" || len(view.DeferredQueue.Plan.Actions) != 1 {
		t.Fatalf("Deferred=%+v queue=%+v, want runtime deferred queue", view.DeferredPlan, view.DeferredQueue)
	}
	if view.DataRounds != 4 || view.RepairRounds != 2 {
		t.Fatalf("rounds=%d/%d, want runtime rounds 4/2", view.DataRounds, view.RepairRounds)
	}
}

func TestDataTaskWorkflowRuntimeViewKeepsFallbackCurrentWhenRuntimeCurrentEmpty(t *testing.T) {
	rt := dataworkflow.NewWorkflowRuntime(dataquery.TaskPlan{})
	fallbackCurrent := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "fallback_current", Kind: dataquery.DataActionComputeContribs}}}
	fallbackRecords := []dataTaskWorkflowRecord{{Err: "fallback"}}

	view := dataTaskWorkflowRuntimeViewFrom(rt, fallbackRecords, fallbackCurrent, dataquery.TaskPlan{}, 1, 1)
	if len(view.CurrentPlan.Actions) != 1 || view.CurrentPlan.Actions[0].ID != "fallback_current" {
		t.Fatalf("CurrentPlan=%+v, want fallback current", view.CurrentPlan)
	}
	if len(view.Records) != 1 || view.Records[0].Err != "fallback" {
		t.Fatalf("Records=%+v, want fallback records", view.Records)
	}
	if view.DataRounds != 1 || view.RepairRounds != 1 {
		t.Fatalf("rounds=%d/%d, want fallback rounds 1/1", view.DataRounds, view.RepairRounds)
	}
}

func TestDataTaskWorkflowStateFromRuntimeViewCarriesDeferredQueue(t *testing.T) {
	view := dataTaskWorkflowRuntimeView{
		Records: []dataTaskWorkflowRecord{{
			Result: &dataquery.Result{ConsumedPaths: []string{"records.csv"}},
		}},
		CurrentPlan: dataquery.TaskPlan{
			CoverageContract: dataquery.CoverageContract{ContributionLedgerRequired: true},
			Actions:          []dataquery.DataAction{{ID: "filter", Kind: dataquery.DataActionFilterRecords}},
		},
		DeferredQueue: dataworkflow.NewDeferredQueue(dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "compute",
			Kind: dataquery.DataActionComputeContribs,
		}}}),
	}

	state := dataTaskWorkflowStateFromRuntimeView(view)
	if len(state.ActionGraph.Ready) != 1 || state.ActionGraph.Ready[0].ID != "filter" {
		t.Fatalf("ready=%+v, want current runtime action", state.ActionGraph.Ready)
	}
	if state.ActionGraph.DeferredQueue.Actions != 1 || len(state.ActionGraph.Deferred) != 1 || state.ActionGraph.Deferred[0].ID != "compute" {
		t.Fatalf("deferred graph=%+v, want deferred queue action", state.ActionGraph)
	}
}

func TestMarshalDataTaskWorkflowStateFromRuntimeViewCarriesActionGraph(t *testing.T) {
	view := dataTaskWorkflowRuntimeView{
		Records: []dataTaskWorkflowRecord{{
			Result: &dataquery.Result{ConsumedPaths: []string{"records.csv"}},
		}},
		CurrentPlan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "derive",
			Kind: dataquery.DataActionDeriveFields,
		}}},
		DeferredQueue: dataworkflow.NewDeferredQueue(dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:   "join",
			Kind: dataquery.DataActionJoinRecords,
		}}}),
	}

	raw := marshalDataTaskWorkflowStateFromRuntimeView(view)
	for _, want := range []string{`"action_graph"`, `"derive"`, `"join"`, `"deferred_queue"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("workflow state json=%s, missing %q", raw, want)
		}
	}
}
