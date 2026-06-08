package repl

import (
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
