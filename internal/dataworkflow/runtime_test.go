package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestWorkflowRuntimeClonesCurrentPlan(t *testing.T) {
	plan := dataquery.TaskPlan{
		Status:     "ready",
		InputPaths: []string{"input.csv"},
		Actions: []dataquery.DataAction{{
			ID:         "filter",
			Kind:       dataquery.DataActionFilterRecords,
			InputPaths: []string{"input.csv"},
			Params: map[string]string{
				"filters": `[{"field":"status"}]`,
			},
		}},
		CoverageContract: dataquery.CoverageContract{
			RequiredMaterials: []dataquery.CoverageMaterial{{
				Path:           "rules.txt",
				DistilledNotes: []string{"keep active records"},
			}},
		},
	}
	rt := NewWorkflowRuntime(plan)
	plan.Actions[0].Params["filters"] = "mutated"
	plan.CoverageContract.RequiredMaterials[0].DistilledNotes[0] = "mutated"

	got := rt.CurrentPlan()
	if got.Actions[0].Params["filters"] != `[{"field":"status"}]` {
		t.Fatalf("runtime current plan params mutated: %#v", got.Actions[0].Params)
	}
	if got.CoverageContract.RequiredMaterials[0].DistilledNotes[0] != "keep active records" {
		t.Fatalf("runtime coverage notes mutated: %#v", got.CoverageContract.RequiredMaterials[0].DistilledNotes)
	}

	got.Actions[0].Params["filters"] = "second mutation"
	got.CoverageContract.RequiredMaterials[0].DistilledNotes[0] = "second mutation"
	again := rt.CurrentPlan()
	if again.Actions[0].Params["filters"] != `[{"field":"status"}]` {
		t.Fatalf("runtime current plan leaked returned params: %#v", again.Actions[0].Params)
	}
	if again.CoverageContract.RequiredMaterials[0].DistilledNotes[0] != "keep active records" {
		t.Fatalf("runtime current plan leaked returned notes: %#v", again.CoverageContract.RequiredMaterials[0].DistilledNotes)
	}
}

func TestWorkflowRuntimeOwnsDeferredQueueAndAdmission(t *testing.T) {
	deferred := dataquery.TaskPlan{
		Status: "ready",
		Actions: []dataquery.DataAction{{
			ID:     "join",
			Kind:   dataquery.DataActionJoinRecords,
			Params: map[string]string{"join_fields": "id"},
		}},
	}
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.EnqueueDeferred(2, deferred, "split DAG rank")
	deferred.Actions[0].Params["join_fields"] = "mutated"

	gotDeferred := rt.DeferredPlan()
	if gotDeferred.Actions[0].Params["join_fields"] != "id" {
		t.Fatalf("runtime deferred plan mutated: %#v", gotDeferred.Actions[0].Params)
	}
	if len(rt.DeferredQueue().Events) != 1 || rt.DeferredQueue().Events[0].Action != DeferredQueueTransitionEnqueue {
		t.Fatalf("deferred events not recorded: %#v", rt.DeferredQueue().Events)
	}

	admission := ActionDAGAdmissionDecision{
		Plan: deferred,
		Guard: NewGuardResult("field_contract", "error", RepairNeedsTypedAction, "missing field", WorkflowViolation{
			Code:          "field_contract",
			MissingFields: []string{"id"},
		}),
	}
	rt.SetAdmission(admission)
	admission.Plan.Actions[0].Params["join_fields"] = "mutated again"
	admission.Guard.Violations[0].MissingFields[0] = "mutated"

	gotAdmission := rt.Admission()
	if gotAdmission.Plan.Actions[0].Params["join_fields"] != "mutated" {
		t.Fatalf("runtime admission plan mutated: %#v", gotAdmission.Plan.Actions[0].Params)
	}
	if gotAdmission.Guard.Violations[0].MissingFields[0] != "id" {
		t.Fatalf("runtime admission violation mutated: %#v", gotAdmission.Guard.Violations)
	}
}
