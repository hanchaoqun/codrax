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
	dispatched := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:   "join",
		Kind: dataquery.DataActionJoinRecords,
	}}}
	remainder := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:     "compute",
		Kind:   dataquery.DataActionComputeContribs,
		Params: map[string]string{"value_field": "amount"},
	}}}
	queue := rt.DispatchDeferred(3, dispatched, remainder, DeferredDispatchStatus{Ready: true, ReadyActions: 1}, "dispatch ready rank")
	remainder.Actions[0].Params["value_field"] = "mutated"
	queue.Plan.Actions[0].Params["value_field"] = "mutated again"
	gotDeferredAfterDispatch := rt.DeferredPlan()
	if gotDeferredAfterDispatch.Actions[0].Params["value_field"] != "amount" {
		t.Fatalf("runtime deferred dispatch leaked queue state: %#v", gotDeferredAfterDispatch.Actions[0].Params)
	}
	if len(rt.DeferredQueue().Events) != 2 || rt.DeferredQueue().Events[1].Action != DeferredQueueTransitionDispatch {
		t.Fatalf("deferred dispatch event not recorded: %#v", rt.DeferredQueue().Events)
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

func TestWorkflowRuntimeAdvancesDeferredQueueDispatch(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	deferred := dataquery.TaskPlan{Actions: []dataquery.DataAction{
		{ID: "join", Kind: dataquery.DataActionJoinRecords},
		{ID: "compute", Kind: dataquery.DataActionComputeContribs},
	}}
	rt.EnqueueDeferred(1, deferred, "split graph")
	dispatched := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "join", Kind: dataquery.DataActionJoinRecords}}}
	remainder := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "compute", Kind: dataquery.DataActionComputeContribs}}}

	advance := rt.AdvanceDeferredQueue(2, dispatched, remainder, DeferredDispatchStatus{Ready: true, ReadyActions: 1}, true, "dispatch ready rank", "")
	if advance.Action != DeferredQueueTransitionDispatch || len(advance.Queue.Plan.Actions) != 1 {
		t.Fatalf("advance=%+v, want dispatch with one remaining action", advance)
	}
	if rt.DeferredPlan().Actions[0].ID != "compute" {
		t.Fatalf("runtime deferred plan=%+v, want remaining compute", rt.DeferredPlan())
	}
	events := rt.DeferredQueue().Events
	if len(events) != 2 || events[1].Action != DeferredQueueTransitionDispatch || events[1].RemainingActions != 1 {
		t.Fatalf("deferred events=%+v, want dispatch transition", events)
	}
	remainder.Actions[0].ID = "mutated"
	if rt.DeferredPlan().Actions[0].ID != "compute" {
		t.Fatalf("runtime deferred plan leaked remainder mutation: %+v", rt.DeferredPlan())
	}
}

func TestWorkflowRuntimeAdvancesDeferredQueueLifecycle(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	deferred := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "join", Kind: dataquery.DataActionJoinRecords}}}
	rt.EnqueueDeferred(1, deferred, "split graph")

	retained := rt.AdvanceDeferredQueue(2, dataquery.TaskPlan{}, dataquery.TaskPlan{}, DeferredDispatchStatus{
		Actions:        1,
		BlockedActions: 1,
		ReasonCode:     DeferredBlockInputUnavailable,
		Reason:         "input alias is not available",
	}, false, "", "")
	if retained.Action != DeferredQueueTransitionRetain || len(retained.Queue.Plan.Actions) != 1 {
		t.Fatalf("retained=%+v, want retained queue", retained)
	}
	if retained.Lifecycle.Action != DeferredQueueLifecycleRetain {
		t.Fatalf("lifecycle=%+v, want retain", retained.Lifecycle)
	}

	discarded := rt.AdvanceDeferredQueue(3, dataquery.TaskPlan{}, dataquery.TaskPlan{}, DeferredDispatchStatus{
		Actions:        1,
		BlockedActions: 1,
		ReasonCode:     DeferredBlockAdmissionRejected,
		Reason:         "candidate was rejected",
	}, false, "", "discard rejected queue")
	if discarded.Action != DeferredQueueTransitionDiscard || len(discarded.Queue.Plan.Actions) != 0 {
		t.Fatalf("discarded=%+v, want discarded queue", discarded)
	}
	events := rt.DeferredQueue().Events
	if len(events) != 3 || events[1].Action != DeferredQueueTransitionRetain || events[2].Action != DeferredQueueTransitionDiscard {
		t.Fatalf("deferred events=%+v, want retain then discard", events)
	}
}

func TestWorkflowRuntimeAppliesPostResultDeferredDispatch(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.EnqueueDeferred(1, dataquery.TaskPlan{Actions: []dataquery.DataAction{
		{ID: "join", Kind: dataquery.DataActionJoinRecords},
		{ID: "compute", Kind: dataquery.DataActionComputeContribs},
	}}, "split graph")
	dispatched := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "join", Kind: dataquery.DataActionJoinRecords}}}
	remainder := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "compute", Kind: dataquery.DataActionComputeContribs}}}

	decision := rt.ApplyPostResultDecision(3, PostResultDecision{
		Action:         PostResultDispatchDeferred,
		Plan:           dispatched,
		Remainder:      remainder,
		DeferredStatus: DeferredDispatchStatus{Ready: true, ReadyActions: 1},
	})
	if !decision.Applied || decision.Advance.Action != DeferredQueueTransitionDispatch || rt.DeferredPlan().Actions[0].ID != "compute" {
		t.Fatalf("decision=%+v deferred=%+v, want dispatched remainder", decision, rt.DeferredPlan())
	}
	if events := rt.DeferredQueue().Events; len(events) != 2 || events[1].Round != 4 {
		t.Fatalf("events=%+v, want dispatch at next data round", events)
	}
	dispatched.Actions[0].ID = "mutated"
	decision.PostResult.Plan.Actions[0].ID = "mutated-decision"
	if decision.Advance.DispatchedPlan.Actions[0].ID != "join" {
		t.Fatalf("advance leaked plan mutation: %+v", decision.Advance.DispatchedPlan)
	}
}

func TestWorkflowRuntimeAppliesPostResultDeferredLifecycle(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.EnqueueDeferred(1, dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "join", Kind: dataquery.DataActionJoinRecords}}}, "split graph")
	retained := rt.ApplyPostResultDecision(2, PostResultDecision{
		Action:         PostResultUpdateDeferred,
		DeferredPlan:   rt.DeferredPlan(),
		DeferredStatus: DeferredDispatchStatus{Actions: 1, BlockedActions: 1, ReasonCode: DeferredBlockInputUnavailable, Reason: "input unavailable"},
	})
	if retained.Advance.Action != DeferredQueueTransitionRetain || len(rt.DeferredPlan().Actions) != 1 {
		t.Fatalf("retained=%+v deferred=%+v, want retained queue", retained, rt.DeferredPlan())
	}
	discarded := rt.ApplyPostResultDecision(3, PostResultDecision{
		Action:         PostResultUpdateDeferred,
		DeferredPlan:   rt.DeferredPlan(),
		DeferredStatus: DeferredDispatchStatus{Actions: 1, BlockedActions: 1, ReasonCode: DeferredBlockAdmissionRejected, Reason: "admission rejected"},
	})
	if discarded.Advance.Action != DeferredQueueTransitionDiscard || len(rt.DeferredPlan().Actions) != 0 {
		t.Fatalf("discarded=%+v deferred=%+v, want discarded queue", discarded, rt.DeferredPlan())
	}
}

func TestWorkflowRuntimeAppliesPostResultEvaluateClearsEmptyQueue(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	decision := rt.ApplyPostResultDecision(2, PostResultDecision{Action: PostResultEvaluate})
	if !decision.Applied || decision.Advance.Action != DeferredQueueLifecycleNone {
		t.Fatalf("decision=%+v, want no deferred queue lifecycle", decision)
	}
}

func TestWorkflowRuntimeQueueDeferredReturnsTypedDecision(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	plan := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:     "compute",
		Kind:   dataquery.DataActionComputeContribs,
		Params: map[string]string{"value_field": "amount"},
	}}}
	decision := rt.QueueDeferred(5, plan, "split next rank")
	plan.Actions[0].Params["value_field"] = "mutated"
	decision.QueuedPlan.Actions[0].Params["value_field"] = "mutated again"
	if decision.Action != DeferredQueueTransitionEnqueue || decision.Reason != "split next rank" {
		t.Fatalf("decision=%+v, want enqueue decision", decision)
	}
	if got := rt.DeferredPlan(); len(got.Actions) != 1 || got.Actions[0].Params["value_field"] != "amount" {
		t.Fatalf("runtime deferred=%+v, want clone-isolated queued plan", got)
	}
	if len(decision.Queue.Events) != 1 || decision.Queue.Events[0].Action != DeferredQueueTransitionEnqueue {
		t.Fatalf("queue=%+v, want enqueue event", decision.Queue)
	}
}

func TestWorkflowRuntimeAppliesGuardRecoveryFallbackAndQueuesRemainder(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{Status: "ready"})
	fallback := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:     "extract",
		Kind:   dataquery.DataActionExtractRecords,
		Params: map[string]string{"limit": "100"},
	}}}
	remainder := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:     "compute",
		Kind:   dataquery.DataActionComputeContribs,
		Params: map[string]string{"value_field": "amount"},
	}}}
	decision := GuardRecoveryDecision{
		Action:    GuardRecoveryFallbackPlan,
		Source:    "stage_prefix",
		Plan:      fallback,
		Remainder: remainder,
		Guard:     NewGuardResult("stage_guard", "error", RepairNeedsTypedAction, "split graph rank"),
		Reason:    "trimmed multi-stage data plan to current DAG stage",
	}
	applied := rt.ApplyGuardRecoveryDecision(4, decision, func(plan dataquery.TaskPlan) dataquery.TaskPlan {
		plan.Goal = "protected"
		return plan
	})
	if !applied.Applied || !applied.DeferredQueued {
		t.Fatalf("applied=%+v, want applied with deferred queue", applied)
	}
	if applied.Plan.Goal != "protected" || rt.CurrentPlan().Actions[0].ID != "extract" {
		t.Fatalf("current=%+v applied=%+v, want protected fallback current plan", rt.CurrentPlan(), applied)
	}
	if rt.DeferredPlan().Actions[0].ID != "compute" {
		t.Fatalf("deferred=%+v, want queued remainder", rt.DeferredPlan())
	}
	if events := rt.DeferredQueue().Events; len(events) != 1 || events[0].Action != DeferredQueueTransitionEnqueue || events[0].Round != 4 {
		t.Fatalf("deferred events=%+v, want enqueue at round 4", events)
	}
	transitions := rt.PlanTransitions()
	if len(transitions) != 1 || transitions[0].Source != "continue" || transitions[0].Round != 4 {
		t.Fatalf("transitions=%+v, want continue transition at round 4", transitions)
	}
	fallback.Actions[0].Params["limit"] = "mutated"
	remainder.Actions[0].Params["value_field"] = "mutated"
	decision.Plan.Actions[0].ID = "mutated-decision"
	applied.Plan.Actions[0].ID = "mutated-applied"
	if rt.CurrentPlan().Actions[0].Params["limit"] != "100" || rt.DeferredPlan().Actions[0].Params["value_field"] != "amount" {
		t.Fatalf("runtime leaked guard recovery mutation: current=%+v deferred=%+v", rt.CurrentPlan(), rt.DeferredPlan())
	}
}

func TestWorkflowRuntimeAppliesFailureRecoveryFallback(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{Status: "ready"})
	fallback := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{
		ID:     "assemble",
		Kind:   dataquery.DataActionAssembleAnswer,
		Params: map[string]string{"format": "plain_single_line"},
	}}}
	recovery := FailureRecoveryDecision{
		Action: FailureRecoveryFallbackPlan,
		Plan:   fallback,
		Reason: "complete missing final projection",
		Guard:  NewGuardResult("completion_gate", "error", RepairNeedsTypedAction, "missing final projection"),
	}
	applied := rt.ApplyFailureRecoveryDecision(7, "ledger", recovery, func(plan dataquery.TaskPlan) dataquery.TaskPlan {
		plan.Goal = "protected"
		return plan
	})
	if !applied.Applied || applied.Source != "ledger" || applied.Plan.Goal != "protected" {
		t.Fatalf("applied=%+v, want protected ledger fallback", applied)
	}
	if got := rt.CurrentPlan(); got.Actions[0].ID != "assemble" || got.Goal != "protected" {
		t.Fatalf("current=%+v, want applied fallback current plan", got)
	}
	transitions := rt.PlanTransitions()
	if len(transitions) != 1 || transitions[0].Source != "ledger" || transitions[0].Round != 7 {
		t.Fatalf("transitions=%+v, want ledger transition at round 7", transitions)
	}
	fallback.Actions[0].Params["format"] = "mutated"
	recovery.Plan.Actions[0].ID = "mutated-decision"
	applied.Plan.Actions[0].ID = "mutated-applied"
	if rt.CurrentPlan().Actions[0].Params["format"] != "plain_single_line" {
		t.Fatalf("runtime leaked failure recovery mutation: current=%+v", rt.CurrentPlan())
	}
}

func TestWorkflowRuntimeAdmitsCandidatePlanTransition(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	candidate := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{
		{ID: "extract", Kind: dataquery.DataActionExtractRecords},
		{ID: "compute", Kind: dataquery.DataActionComputeContribs},
	}}
	decision := rt.AdmitCandidatePlan(3, "repair", ActionDAGAdmissionInput{
		Plan: candidate,
		PrefixFallback: func(plan dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, string, bool) {
			prefix := plan
			prefix.Actions = append([]dataquery.DataAction(nil), plan.Actions[:1]...)
			remainder := plan
			remainder.Actions = append([]dataquery.DataAction(nil), plan.Actions[1:]...)
			return prefix, remainder, "split rank", true
		},
	})
	if !decision.Accepted || !decision.Rewritten || !decision.AppendedRecord || decision.Source != "continue" {
		t.Fatalf("decision=%+v, want accepted rewritten transition", decision)
	}
	if rt.CurrentPlan().Actions[0].ID != "extract" {
		t.Fatalf("current plan=%+v, want prefix plan", rt.CurrentPlan())
	}
	records := rt.Records()
	if len(records) != 1 || records[0].Admission == nil || !records[0].Admission.Rewritten {
		t.Fatalf("records=%+v, want rewritten admission record", records)
	}
	transitions := rt.PlanTransitions()
	if len(transitions) != 1 || transitions[0].Source != "continue" || transitions[0].FirstActionID != "extract" {
		t.Fatalf("transitions=%+v, want continue transition", transitions)
	}
	var transitionEvents int
	for _, event := range decision.ProcessEvents {
		if event.Kind == "plan_transition" {
			transitionEvents++
			if event.Round != 3 || event.Status != "continue" || !stringSliceContains(event.AuditDetails, "transition_first_action=extract") {
				t.Fatalf("transition event=%+v, want runtime plan transition", event)
			}
		}
	}
	if transitionEvents != 1 {
		t.Fatalf("process_events=%+v, want one plan transition event", decision.ProcessEvents)
	}
	if len(rt.DeferredPlan().Actions) != 0 {
		t.Fatalf("deferred plan=%+v, AdmitCandidatePlan should not queue remainder", rt.DeferredPlan())
	}
}

func TestWorkflowRuntimeAdmitsCandidatePlanAndQueuesRemainder(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	candidate := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{
		{ID: "extract", Kind: dataquery.DataActionExtractRecords},
		{ID: "compute", Kind: dataquery.DataActionComputeContribs},
	}}
	decision := rt.AdmitCandidatePlanAndQueueRemainder(3, "repair", ActionDAGAdmissionInput{
		Plan: candidate,
		PrefixFallback: func(plan dataquery.TaskPlan) (dataquery.TaskPlan, dataquery.TaskPlan, string, bool) {
			prefix := plan
			prefix.Actions = append([]dataquery.DataAction(nil), plan.Actions[:1]...)
			remainder := plan
			remainder.Actions = append([]dataquery.DataAction(nil), plan.Actions[1:]...)
			return prefix, remainder, "split rank", true
		},
	})
	if !decision.Accepted || !decision.Rewritten || !decision.DeferredQueued {
		t.Fatalf("decision=%+v, want accepted rewritten queued admission", decision)
	}
	if len(decision.DeferredPlan.Actions) != 1 || decision.DeferredPlan.Actions[0].ID != "compute" {
		t.Fatalf("deferred plan=%+v, want queued remainder", decision.DeferredPlan)
	}
	if len(rt.DeferredPlan().Actions) != 1 || rt.DeferredPlan().Actions[0].ID != "compute" {
		t.Fatalf("runtime deferred=%+v, want queued remainder", rt.DeferredPlan())
	}
	if len(decision.DeferredQueue.Events) != 1 || decision.DeferredQueue.Events[0].Action != DeferredQueueTransitionEnqueue {
		t.Fatalf("deferred queue=%+v, want enqueue event", decision.DeferredQueue)
	}
	if decision.DeferredReason != "split rank" {
		t.Fatalf("DeferredReason=%q, want split rank", decision.DeferredReason)
	}
}

func TestWorkflowRuntimeAdmitsCandidatePlanBlocked(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "current", Kind: dataquery.DataActionExtractRecords}}})
	candidate := dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{{ID: "bad", Kind: dataquery.DataActionFilterRecords}}}
	decision := rt.AdmitCandidatePlan(4, "continue", ActionDAGAdmissionInput{
		Plan: candidate,
		Guard: func(dataquery.TaskPlan) GuardResult {
			return NewGuardResult("field_contract_violation", "error", RepairNeedsTypedAction, "missing field", WorkflowViolation{
				Code:     "field_contract_violation",
				ActionID: "bad",
			})
		},
	})
	if decision.Accepted || !decision.Blocked || decision.Admission.FinalGuard.Code != "field_contract_violation" {
		t.Fatalf("decision=%+v, want blocked admission", decision)
	}
	if rt.CurrentPlan().Actions[0].ID != "bad" {
		t.Fatalf("current plan=%+v, blocked admission should keep rejected candidate as current audit plan", rt.CurrentPlan())
	}
	if len(rt.PlanTransitions()) != 0 {
		t.Fatalf("transitions=%+v, blocked admission should not add transition", rt.PlanTransitions())
	}
	for _, event := range decision.ProcessEvents {
		if event.Kind == "plan_transition" {
			t.Fatalf("process_events=%+v, blocked admission should not emit plan transition", decision.ProcessEvents)
		}
	}
}

func TestWorkflowRuntimeBlocksDuplicateFailedActionEdge(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	failedAction := dataquery.DataAction{
		ID:             "first_filter",
		Kind:           dataquery.DataActionFilterRecords,
		InputPaths:     []string{"records.json"},
		OutputArtifact: "paid.json",
		Params: map[string]string{
			"filters_json": `[{"field":"status","op":"eq","value":"paid"}]`,
		},
	}
	rt.AppendRecord(WorkflowRecord{
		Plan: dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{failedAction}},
		Err:  "field contract failed",
	})
	replayed := failedAction
	replayed.ID = "renamed_filter"
	decision := rt.AdmitCandidatePlan(2, "repair", ActionDAGAdmissionInput{
		Plan: dataquery.TaskPlan{Status: "ready", Actions: []dataquery.DataAction{replayed}},
	})
	if decision.Accepted || !decision.Blocked || decision.Admission.FinalGuard.Code != "duplicate_action_edge" {
		t.Fatalf("decision=%+v, want duplicate edge blocked before transition", decision)
	}
	if len(rt.PlanTransitions()) != 0 {
		t.Fatalf("transitions=%+v, duplicate blocked admission should not transition", rt.PlanTransitions())
	}
	if got := rt.CurrentPlan(); len(got.Actions) != 1 || got.Actions[0].ID != "renamed_filter" {
		t.Fatalf("current plan=%+v, want rejected candidate kept for audit", got)
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

func TestWorkflowRuntimeSwitchCurrentPlanWithEventsReturnsDelta(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.AppendProcessEventFromInput(WorkflowProcessEventInput{
		Kind:   "evaluate",
		Round:  1,
		Reason: "prior event",
	})
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:      "assemble",
		Kind:    dataquery.DataActionAssembleAnswer,
		Purpose: "project final grouped values",
	}}}

	decision := rt.SwitchCurrentPlanWithEvents(4, "continue", plan, "next typed output stage")
	if !decision.Switched || decision.Source != "continue" || decision.Round != 4 {
		t.Fatalf("decision=%+v, want switched continue decision", decision)
	}
	if len(decision.ProcessEvents) != 1 || decision.ProcessEvents[0].Kind != "plan_transition" {
		t.Fatalf("process_events=%+v, want one plan transition delta", decision.ProcessEvents)
	}
	if len(rt.ProcessEvents()) != 2 {
		t.Fatalf("runtime events=%+v, want prior event plus transition", rt.ProcessEvents())
	}
	decision.ProcessEvents[0].Kind = "mutated"
	decision.Plan.Actions[0].Purpose = "mutated"
	gotEvents := rt.ProcessEvents()
	if gotEvents[1].Kind != "plan_transition" {
		t.Fatalf("runtime transition event leaked returned delta mutation: %+v", gotEvents)
	}
	if rt.CurrentPlan().Actions[0].Purpose != "project final grouped values" {
		t.Fatalf("runtime current plan leaked returned decision mutation: %+v", rt.CurrentPlan())
	}
}

func TestWorkflowRuntimeIterationDecisionsOwnRoundAllocation(t *testing.T) {
	current := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:   "filter",
		Kind: dataquery.DataActionFilterRecords,
	}}}
	rt := NewWorkflowRuntime(current)
	rt.AppendRecord(WorkflowRecord{Plan: current, Result: &dataquery.Result{Answer: "intermediate"}})
	rt.EnqueueDeferred(1, dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:   "compute",
		Kind: dataquery.DataActionComputeContribs,
	}}}, "defer next rank")

	dataIter := rt.BeginDataIteration()
	if dataIter.Phase != "data" || dataIter.DataRounds != 1 || dataIter.RepairRounds != 0 {
		t.Fatalf("data iteration=%+v, want first data round", dataIter)
	}
	if len(dataIter.Records) != 1 || dataIter.CurrentPlan.Actions[0].ID != "filter" || dataIter.DeferredPlan.Actions[0].ID != "compute" {
		t.Fatalf("data iteration snapshot=%+v, want runtime state", dataIter)
	}

	repairIter := rt.BeginRepairIteration()
	if repairIter.Phase != "repair" || repairIter.DataRounds != 1 || repairIter.RepairRounds != 1 {
		t.Fatalf("repair iteration=%+v, want first repair round after one data round", repairIter)
	}
	repairIter.CurrentPlan.Actions[0].ID = "mutated"
	repairIter.Records[0].Plan.Actions[0].ID = "mutated-record"
	if rt.CurrentPlan().Actions[0].ID != "filter" || rt.Records()[0].Plan.Actions[0].ID != "filter" {
		t.Fatalf("runtime state leaked returned iteration mutation: current=%+v records=%+v", rt.CurrentPlan(), rt.Records())
	}
}

func TestWorkflowRuntimeRecordsPlanTransitionProcessEvent(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	plan := dataquery.TaskPlan{
		Goal:         "answer the data question",
		WhyThisBatch: "compute reusable grouped contributions",
		NextBatch:    "assemble the requested final output",
		Actions: []dataquery.DataAction{{
			ID:      "compute",
			Kind:    dataquery.DataActionComputeContribs,
			Purpose: "sum values for each requested group",
		}},
	}
	rt.SwitchCurrentPlan(5, "ledger", plan, "contribution ledger is missing")
	plan.Goal = "mutated"
	plan.Actions[0].Purpose = "mutated"

	events := rt.ProcessEvents()
	if len(events) != 1 {
		t.Fatalf("ProcessEvents=%+v, want one transition event", events)
	}
	event := events[0]
	if event.Kind != "plan_transition" || event.Round != 5 || event.Status != "ledger" || event.Reason != "contribution ledger is missing" {
		t.Fatalf("transition event=%+v", event)
	}
	if event.Goal != "answer the data question" || event.BatchPurpose != "compute reusable grouped contributions" || event.NextStep != "assemble the requested final output" {
		t.Fatalf("transition event intent not preserved: %+v", event)
	}
	if event.ActionSummary != "sum values for each requested group" {
		t.Fatalf("ActionSummary=%q, want action purpose", event.ActionSummary)
	}
	events[0].Goal = "mutated"
	if rt.ProcessEvents()[0].Goal != "answer the data question" {
		t.Fatalf("runtime transition event leaked returned slice: %+v", rt.ProcessEvents())
	}
}

func TestWorkflowRuntimeSnapshotClonesLiveState(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	current := rt.SwitchCurrentPlan(2, "continue", dataquery.TaskPlan{
		Actions: []dataquery.DataAction{{
			ID:     "filter",
			Kind:   dataquery.DataActionFilterRecords,
			Params: map[string]string{"field": "status"},
		}},
	}, "next batch")
	rt.EnqueueDeferred(2, dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:     "compute",
		Kind:   dataquery.DataActionComputeContribs,
		Params: map[string]string{"value_field": "amount"},
	}}}, "later rank")
	rt.SetAdmission(ActionDAGAdmissionDecision{Plan: current})
	rt.SetRounds(3, 4)
	rt.AppendRecord(WorkflowRecord{Plan: current, Result: &dataquery.Result{Answer: "42"}})

	snapshot := rt.Snapshot()
	snapshot.CurrentPlan.Actions[0].Params["field"] = "mutated"
	snapshot.DeferredPlan.Actions[0].Params["value_field"] = "mutated"
	snapshot.DeferredQueue.Plan.Actions[0].Params["value_field"] = "mutated"
	snapshot.Records[0].Result.Answer = "mutated"
	snapshot.ProcessEvents[0].Kind = "mutated"
	snapshot.PlanTransitions[0].Source = "mutated"
	snapshot.Admission.Plan.Actions[0].Params["field"] = "mutated"

	next := rt.Snapshot()
	if next.CurrentPlan.Actions[0].Params["field"] != "status" {
		t.Fatalf("snapshot current plan leaked mutation: %+v", next.CurrentPlan.Actions[0].Params)
	}
	if next.DeferredPlan.Actions[0].Params["value_field"] != "amount" || next.DeferredQueue.Plan.Actions[0].Params["value_field"] != "amount" {
		t.Fatalf("snapshot deferred state leaked mutation: %+v / %+v", next.DeferredPlan.Actions[0].Params, next.DeferredQueue.Plan.Actions[0].Params)
	}
	if next.Records[0].Result.Answer != "42" || next.ProcessEvents[0].Kind == "mutated" || next.PlanTransitions[0].Source != "continue" {
		t.Fatalf("snapshot runtime state leaked mutation: records=%+v events=%+v transitions=%+v", next.Records, next.ProcessEvents, next.PlanTransitions)
	}
	if next.Admission.Plan.Actions[0].Params["field"] != "status" {
		t.Fatalf("snapshot admission leaked mutation: %+v", next.Admission.Plan.Actions[0].Params)
	}
	if next.DataRounds != 3 || next.RepairRounds != 4 {
		t.Fatalf("snapshot rounds=%d/%d, want 3/4", next.DataRounds, next.RepairRounds)
	}
}

func TestBuildWorkflowRuntimeViewPrefersRuntimeSnapshot(t *testing.T) {
	fallbackRecord := WorkflowRecord{Err: "fallback"}
	fallbackCurrent := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "fallback_current", Kind: dataquery.DataActionInspectMaterial}}}
	fallbackDeferred := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "fallback_deferred", Kind: dataquery.DataActionInspectMaterial}}}

	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	current := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "runtime_current", Kind: dataquery.DataActionExtractRecords}}}
	rt.SwitchCurrentPlan(3, "continue", current, "runtime current")
	rt.EnqueueDeferred(3, dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "runtime_deferred", Kind: dataquery.DataActionJoinRecords}}}, "next rank")
	rt.AppendRecord(WorkflowRecord{Plan: current, Err: "runtime"})
	rt.SetRounds(4, 2)

	view := BuildWorkflowRuntimeView(WorkflowRuntimeViewInput{
		Runtime:              rt,
		FallbackRecords:      []WorkflowRecord{fallbackRecord},
		FallbackCurrent:      fallbackCurrent,
		FallbackDeferred:     fallbackDeferred,
		FallbackDataRounds:   1,
		FallbackRepairRounds: 1,
	})
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
	view.Records[0].Err = "mutated"
	view.CurrentPlan.Actions[0].ID = "mutated"
	if next := BuildWorkflowRuntimeView(WorkflowRuntimeViewInput{Runtime: rt}); next.Records[0].Err != "runtime" || next.CurrentPlan.Actions[0].ID != "runtime_current" {
		t.Fatalf("runtime view leaked mutation: %+v", next)
	}
}

func TestBuildWorkflowRuntimeViewKeepsFallbackCurrentWhenRuntimeCurrentEmpty(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	fallbackCurrent := dataquery.TaskPlan{Actions: []dataquery.DataAction{{ID: "fallback_current", Kind: dataquery.DataActionComputeContribs}}}
	fallbackRecords := []WorkflowRecord{{Err: "fallback"}}

	view := BuildWorkflowRuntimeView(WorkflowRuntimeViewInput{
		Runtime:              rt,
		FallbackRecords:      fallbackRecords,
		FallbackCurrent:      fallbackCurrent,
		FallbackDataRounds:   1,
		FallbackRepairRounds: 1,
	})
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

func TestWorkflowRuntimeAppendsTypedGuardEvent(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	plan := dataquery.TaskPlan{
		Goal:         "compute final output",
		WhyThisBatch: "assemble answer",
		Actions: []dataquery.DataAction{{
			ID:      "assemble",
			Kind:    dataquery.DataActionAssembleAnswer,
			Purpose: "assemble final answer",
		}},
	}
	guard := NewGuardResult("output_missing_projection", "error", RepairNeedsTypedAction, "missing final projection", WorkflowViolation{
		Code:       "output_missing_projection",
		ActionKind: string(dataquery.DataActionAssembleAnswer),
		Reason:     "missing final projection",
	})

	event := rt.AppendGuardEvent("completion_gate", 4, plan, guard)
	if event.Kind != "completion_gate" || event.Guard == nil || event.Guard.Code != "output_missing_projection" {
		t.Fatalf("event=%+v, want typed guard event", event)
	}
	events := rt.ProcessEvents()
	if len(events) != 1 || events[0].Guard == nil || events[0].Goal != "compute final output" {
		t.Fatalf("runtime events=%+v, want persisted guard event with plan intent", events)
	}
	guard.Violations[0].Reason = "mutated"
	if rt.ProcessEvents()[0].Guard.Violations[0].Reason != "missing final projection" {
		t.Fatalf("runtime guard event leaked mutation: %+v", rt.ProcessEvents()[0].Guard)
	}
}

func TestWorkflowRuntimeOwnsRoundCounters(t *testing.T) {
	rt := NewWorkflowRuntime(dataquery.TaskPlan{})
	rt.SetRounds(-3, -4)
	if rt.DataRounds() != 0 || rt.RepairRounds() != 0 {
		t.Fatalf("negative rounds should clamp to zero: data=%d repair=%d", rt.DataRounds(), rt.RepairRounds())
	}
	if got := rt.IncrementDataRound(); got != 1 {
		t.Fatalf("IncrementDataRound=%d, want 1", got)
	}
	if got := rt.IncrementRepairRound(); got != 1 {
		t.Fatalf("IncrementRepairRound=%d, want 1", got)
	}
	rt.SetRounds(7, 8)
	if rt.IncrementDataRound() != 8 || rt.IncrementRepairRound() != 9 {
		t.Fatalf("round counters not owned by runtime: data=%d repair=%d", rt.DataRounds(), rt.RepairRounds())
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
		Violations: []dataquery.DataTaskViolation{{
			Code:                 "field_contract_violation",
			InputAlias:           "records.json",
			MissingFields:        []string{"amount"},
			AvailableFieldSample: []string{"status"},
		}},
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
	record.Violations[0].MissingFields[0] = "mutated"
	record.Violations[0].AvailableFieldSample[0] = "mutated"
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
	if got[0].Violations[0].MissingFields[0] != "amount" || got[0].Violations[0].AvailableFieldSample[0] != "status" {
		t.Fatalf("violations leaked: %#v", got[0].Violations)
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
