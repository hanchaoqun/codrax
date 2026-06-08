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

func TestWorkflowRuntimeRecordsPlanTransitions(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:     "compute",
		Kind:   dataquery.DataActionComputeContribs,
		Params: map[string]string{"value_field": "duration"},
	}}}
	current := rt.SwitchCurrentPlan(3, "continue", plan, "next typed stage")
	plan.Actions[0].Params["value_field"] = "mutated"
	current.Actions[0].Params["value_field"] = "mutated again"

	got := rt.CurrentPlan()
	if got.Actions[0].Params["value_field"] != "duration" {
		t.Fatalf("runtime current plan mutated through transition: %#v", got.Actions[0].Params)
	}
	transitions := rt.PlanTransitions()
	if len(transitions) != 1 {
		t.Fatalf("transitions len=%d, want 1: %#v", len(transitions), transitions)
	}
	if transitions[0].Source != "continue" || transitions[0].Round != 3 || transitions[0].FirstActionID != "compute" || transitions[0].FirstActionKind != string(dataquery.DataActionComputeContribs) {
		t.Fatalf("transition=%#v", transitions[0])
	}
	transitions[0].Source = "mutated"
	if rt.PlanTransitions()[0].Source != "continue" {
		t.Fatalf("runtime transitions leaked returned slice: %#v", rt.PlanTransitions())
	}
}

func TestWorkflowRuntimeOwnsRecords(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	record := WorkflowRecord{
		Plan: dataquery.TaskPlan{Actions: []dataquery.DataAction{{
			ID:     "filter",
			Kind:   dataquery.DataActionFilterRecords,
			Params: map[string]string{"filter_field": "status"},
		}}},
		Result: &dataquery.Result{
			Artifacts: []dataquery.DataArtifact{{
				ID:     "records",
				Fields: map[string]string{"status": "valid"},
				Children: []dataquery.DataArtifact{{
					ID:      "child",
					Headers: []string{"status"},
				}},
			}},
			Rows: []dataquery.RowDecision{{
				RowID:            "r1",
				NormalizedFields: map[string]string{"status": "valid"},
				EvidenceRef:      []string{"records.csv:2"},
			}},
			Contributions: []dataquery.ContributionRecord{{
				ItemID:       "r1",
				EvidenceRefs: []string{"records.csv:2"},
			}},
			EntityResolutions: []dataquery.EntityResolutionRecord{{
				ItemID: "r1",
				Candidates: []dataquery.EntityCandidate{{
					ID: "candidate",
				}},
			}},
			Reconcile: &dataquery.ReconcileReport{
				Differences: []string{"none"},
				Groups:      []dataquery.ReconcileGroup{{GroupKey: "g1"}},
			},
			ResultPatches: []dataquery.DataResultPatch{{
				Target: "result",
				Value:  []byte(`"patched"`),
			}},
		},
		Evaluation: &dataquery.Evaluation{MissingInputs: []string{"amount"}},
		Admission: &ActionDAGAdmissionDecision{
			Guard: NewGuardResult("field_contract", "error", RepairNeedsTypedAction, "missing field", WorkflowViolation{
				MissingFields: []string{"amount"},
			}),
		},
	}
	records := rt.AppendRecord(record)
	record.Plan.Actions[0].Params["filter_field"] = "mutated"
	record.Result.Artifacts[0].Fields["status"] = "mutated"
	record.Result.Artifacts[0].Children[0].Headers[0] = "mutated"
	record.Result.Rows[0].NormalizedFields["status"] = "mutated"
	record.Result.Rows[0].EvidenceRef[0] = "mutated"
	record.Result.Contributions[0].EvidenceRefs[0] = "mutated"
	record.Result.EntityResolutions[0].Candidates[0].ID = "mutated"
	record.Result.Reconcile.Differences[0] = "mutated"
	record.Result.Reconcile.Groups[0].GroupKey = "mutated"
	record.Result.ResultPatches[0].Value[0] = 'x'
	record.Evaluation.MissingInputs[0] = "mutated"
	record.Admission.Guard.Violations[0].MissingFields[0] = "mutated"
	records[0].Result.Rows[0].NormalizedFields["status"] = "mutated again"

	got := rt.Records()
	if got[0].Plan.Actions[0].Params["filter_field"] != "status" {
		t.Fatalf("plan leaked: %#v", got[0].Plan.Actions[0].Params)
	}
	if got[0].Result.Artifacts[0].Fields["status"] != "valid" || got[0].Result.Artifacts[0].Children[0].Headers[0] != "status" {
		t.Fatalf("artifact leaked: %#v", got[0].Result.Artifacts[0])
	}
	if got[0].Result.Rows[0].NormalizedFields["status"] != "valid" || got[0].Result.Rows[0].EvidenceRef[0] != "records.csv:2" {
		t.Fatalf("row leaked: %#v", got[0].Result.Rows[0])
	}
	if got[0].Result.Contributions[0].EvidenceRefs[0] != "records.csv:2" {
		t.Fatalf("contribution leaked: %#v", got[0].Result.Contributions[0])
	}
	if got[0].Result.EntityResolutions[0].Candidates[0].ID.String() != "candidate" {
		t.Fatalf("entity resolution leaked: %#v", got[0].Result.EntityResolutions[0])
	}
	if got[0].Result.Reconcile.Differences[0] != "none" || got[0].Result.Reconcile.Groups[0].GroupKey.String() != "g1" {
		t.Fatalf("reconcile leaked: %#v", got[0].Result.Reconcile)
	}
	if string(got[0].Result.ResultPatches[0].Value) != `"patched"` {
		t.Fatalf("patch leaked: %s", got[0].Result.ResultPatches[0].Value)
	}
	if got[0].Evaluation.MissingInputs[0] != "amount" {
		t.Fatalf("evaluation leaked: %#v", got[0].Evaluation)
	}
	if got[0].Admission.Guard.Violations[0].MissingFields[0] != "amount" {
		t.Fatalf("admission leaked: %#v", got[0].Admission)
	}

	eval := dataquery.Evaluation{Status: dataquery.EvalContinueData, MissingInputs: []string{"next"}}
	records = rt.AttachLastEvaluation(eval)
	eval.MissingInputs[0] = "mutated"
	records[0].Evaluation.MissingInputs[0] = "mutated again"
	if rt.Records()[0].Evaluation.MissingInputs[0] != "next" {
		t.Fatalf("attached evaluation leaked: %#v", rt.Records()[0].Evaluation)
	}
	records = rt.AttachLastError("failed")
	records[0].Err = "mutated"
	if rt.Records()[0].Err != "failed" {
		t.Fatalf("attached error leaked: %#v", rt.Records()[0])
	}
	with := rt.RecordsWith(WorkflowRecord{Err: "preview"})
	with[1].Err = "mutated"
	if len(rt.Records()) != 1 {
		t.Fatalf("RecordsWith mutated runtime length: %#v", rt.Records())
	}
}

func TestWorkflowRuntimeOwnsLiveProcessEvents(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	plan := dataquery.TaskPlan{
		WhyThisBatch: "extract records",
		Actions: []dataquery.DataAction{{
			ID:   "extract",
			Kind: dataquery.DataActionExtractRecords,
		}},
	}
	rt.AppendRecord(WorkflowRecord{Plan: plan})
	events := rt.ProcessEvents()
	if len(events) != 1 || events[0].Kind != "action_batch" || events[0].Round != 1 {
		t.Fatalf("ProcessEvents=%+v, want one live action batch event", events)
	}
	events[0].Kind = "mutated"
	if rt.ProcessEvents()[0].Kind != "action_batch" {
		t.Fatalf("runtime process events leaked returned slice: %+v", rt.ProcessEvents())
	}

	rt.AppendProcessEvent(WorkflowJournalEvent{Kind: "evaluate", Round: 1, Reason: "needs next stage"})
	events = rt.ProcessEvents()
	if len(events) != 2 || events[1].Kind != "evaluate" || events[1].Reason != "needs next stage" {
		t.Fatalf("ProcessEvents=%+v, want appended evaluate event", events)
	}
}
