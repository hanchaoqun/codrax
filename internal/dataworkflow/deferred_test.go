package dataworkflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

func TestBuildDeferredDispatchPlanSelectsReadyRank(t *testing.T) {
	deferred := dataquery.TaskPlan{
		Goal: "finish data workflow",
		Actions: []dataquery.DataAction{
			{ID: "derive", Kind: dataquery.DataActionDeriveFields, InputPaths: []string{"records.json"}, OutputArtifact: "derived.json"},
			{ID: "expand", Kind: dataquery.DataActionExpandRecords, InputPaths: []string{"derived.json"}, OutputArtifact: "expanded.json"},
			{ID: "compute", Kind: dataquery.DataActionComputeContribs, InputPaths: []string{"expanded.json"}, OutputArtifact: "contribs.json"},
		},
		ContinueAfter: true,
	}
	next, remainder, status, ok := BuildDeferredDispatchPlan(DeferredDispatchInput{
		Plan: deferred,
		AllowedNextActions: []string{
			string(dataquery.DataActionDeriveFields),
			string(dataquery.DataActionExpandRecords),
			string(dataquery.DataActionComputeContribs),
		},
		Candidates: []DeferredActionCandidate{
			{Index: 0, Action: deferred.Actions[0], Ready: true},
			{Index: 1, Action: deferred.Actions[1], Ready: true},
			{Index: 2, Action: deferred.Actions[2], Ready: true},
		},
	})
	if !ok || !status.Ready {
		t.Fatalf("ok/status=%v/%+v, want ready dispatch", ok, status)
	}
	if len(next.Actions) != 2 || next.Actions[0].ID != "derive" || next.Actions[1].ID != "expand" {
		t.Fatalf("next actions=%+v, want first same dependency rank", next.Actions)
	}
	if len(remainder.Actions) != 1 || remainder.Actions[0].ID != "compute" {
		t.Fatalf("remainder actions=%+v, want compute deferred", remainder.Actions)
	}
	if !next.ContinueAfter || !remainder.ContinueAfter {
		t.Fatalf("continue flags next=%v remainder=%v, want both true", next.ContinueAfter, remainder.ContinueAfter)
	}
}

func TestBuildDeferredDispatchPlanReportsBlockedQueue(t *testing.T) {
	deferred := dataquery.TaskPlan{
		Actions: []dataquery.DataAction{
			{ID: "join", Kind: dataquery.DataActionJoinRecords, InputPaths: []string{"left.json", "right.json"}},
		},
	}
	_, _, status, ok := BuildDeferredDispatchPlan(DeferredDispatchInput{
		Plan:               deferred,
		AllowedNextActions: []string{string(dataquery.DataActionJoinRecords)},
		Candidates: []DeferredActionCandidate{{
			Index:         0,
			Action:        deferred.Actions[0],
			Ready:         false,
			BlockedCode:   DeferredBlockInputUnavailable,
			BlockedReason: "right.json is missing",
		}},
	})
	if ok || status.Ready {
		t.Fatalf("ok/status=%v/%+v, want blocked queue", ok, status)
	}
	if status.ReadyActions != 0 || status.BlockedActions != 1 || status.Reason != "right.json is missing" {
		t.Fatalf("status=%+v, want blocked reason from candidate", status)
	}
	if status.ReasonCode != DeferredBlockInputUnavailable {
		t.Fatalf("ReasonCode=%q, want %q", status.ReasonCode, DeferredBlockInputUnavailable)
	}
	decision := DecideDeferredQueueLifecycle(status)
	if decision.Action != DeferredQueueLifecycleRetain || decision.ReasonCode != DeferredBlockInputUnavailable {
		t.Fatalf("decision=%+v, want retain input-unavailable queue", decision)
	}
}

func TestBuildDeferredActionCandidatesNarrowsSingleRecordSetAction(t *testing.T) {
	actions := []dataquery.DataAction{{
		ID:         "filter_ready",
		Kind:       dataquery.DataActionFilterRecords,
		InputPaths: []string{"raw.json", "ready.json"},
		Params: map[string]string{
			"filters_json": `[{"field":"status","op":"eq","value":"ready"}]`,
		},
	}}
	candidates := BuildDeferredActionCandidates(DeferredActionCandidatesInput{
		Actions: actions,
		SchemaProjections: []ArtifactSchemaProjection{
			{ID: "raw", Aliases: []string{"raw.json"}, Fields: []string{"id"}},
			{ID: "ready", Aliases: []string{"ready.json"}, Fields: []string{"id", "status"}},
		},
	})
	if len(candidates) != 1 || !candidates[0].Ready {
		t.Fatalf("candidates=%+v, want ready narrowed action", candidates)
	}
	if got := candidates[0].Action.InputPaths; len(got) != 1 || got[0] != "ready.json" {
		t.Fatalf("narrowed inputs=%+v, want ready.json", got)
	}
}

func TestBuildDeferredActionCandidatesNarrowsComputeContributionsByFieldContract(t *testing.T) {
	actions := []dataquery.DataAction{{
		ID:         "contrib",
		Kind:       dataquery.DataActionComputeContribs,
		InputPaths: []string{"active_users", "rules_artifacts.json"},
		Params: map[string]string{
			"value_field":   "id",
			"item_id_field": "id",
			"group_key":     "ids",
			"metric":        "id_value",
			"operation":     "include",
		},
	}}
	candidates := BuildDeferredActionCandidates(DeferredActionCandidatesInput{
		Actions: actions,
		SchemaProjections: []ArtifactSchemaProjection{
			{ID: "active_users", Aliases: []string{"active_users"}, Fields: []string{"id", "status"}},
			{ID: "rules", Aliases: []string{"rules_artifacts.json"}, Fields: []string{"rule_id", "rule_text", "status"}},
		},
	})
	if len(candidates) != 1 || !candidates[0].Ready {
		t.Fatalf("candidates=%+v, want compute action narrowed and ready", candidates)
	}
	if got := candidates[0].Action.InputPaths; len(got) != 1 || got[0] != "active_users" {
		t.Fatalf("narrowed inputs=%+v, want active_users", got)
	}
}

func TestBuildDeferredActionCandidatesRewritesNormalizeSourceToCompatibleArtifact(t *testing.T) {
	actions := []dataquery.DataAction{{
		ID:         "normalize",
		Kind:       dataquery.DataActionNormalizeEntities,
		InputPaths: []string{"raw.json"},
		Params: map[string]string{
			"source_fields": `["name"]`,
		},
	}}
	candidates := BuildDeferredActionCandidates(DeferredActionCandidatesInput{
		Actions: actions,
		SchemaProjections: []ArtifactSchemaProjection{
			{ID: "raw", Aliases: []string{"raw.json"}, Fields: []string{"id"}},
			{ID: "names", Aliases: []string{"names.json"}, Fields: []string{"id", "name"}},
		},
	})
	if len(candidates) != 1 || !candidates[0].Ready {
		t.Fatalf("candidates=%+v, want ready rewritten action", candidates)
	}
	if got := candidates[0].Action.InputPaths; len(got) != 1 || got[0] != "names.json" {
		t.Fatalf("rewritten inputs=%+v, want names.json", got)
	}
}

func TestDeferredActionBlockedStatusReportsMissingInput(t *testing.T) {
	_, reason := DeferredActionBlockedStatus(DeferredActionReadinessInput{
		Action: dataquery.DataAction{
			ID:         "join",
			Kind:       dataquery.DataActionJoinRecords,
			InputPaths: []string{"missing.json"},
		},
	})
	if reason == "" {
		t.Fatal("blocked reason is empty, want missing input reason")
	}
	code, _ := DeferredActionBlockedStatus(DeferredActionReadinessInput{
		Action: dataquery.DataAction{
			ID:         "join",
			Kind:       dataquery.DataActionJoinRecords,
			InputPaths: []string{"missing.json"},
		},
	})
	if code != DeferredBlockInputUnavailable {
		t.Fatalf("code=%q, want %q", code, DeferredBlockInputUnavailable)
	}
}

func TestDecideDeferredQueueLifecycleDiscardsAdmissionRejectedQueue(t *testing.T) {
	status := DeferredDispatchStatus{
		Actions:    1,
		ReasonCode: DeferredBlockAdmissionRejected,
		Reason:     "typed admission rejected the deferred action",
	}
	decision := DecideDeferredQueueLifecycle(status)
	if decision.Action != DeferredQueueLifecycleDiscard {
		t.Fatalf("decision=%+v, want discard admission-rejected queue", decision)
	}
}

func TestDeferredQueueSnapshotCarriesStatusAndLifecycle(t *testing.T) {
	status := DeferredDispatchStatus{
		Actions:         2,
		ReadyActions:    0,
		BlockedActions:  2,
		FirstActionID:   "join_later",
		FirstActionKind: string(dataquery.DataActionJoinRecords),
		ReasonCode:      DeferredBlockInputUnavailable,
		Reason:          "right side missing",
	}
	decision := DecideDeferredQueueLifecycle(status)
	snapshot := DeferredQueueSnapshotForStatus(status, decision)
	if snapshot.Actions != 2 || snapshot.FirstActionID != "join_later" || snapshot.ReasonCode != DeferredBlockInputUnavailable {
		t.Fatalf("snapshot=%+v, want deferred status fields", snapshot)
	}
	if snapshot.LifecycleAction != DeferredQueueLifecycleRetain {
		t.Fatalf("LifecycleAction=%q, want retain", snapshot.LifecycleAction)
	}
}

func TestDeferredQueueStateTransitionsCarryTypedEvents(t *testing.T) {
	plan := dataquery.TaskPlan{Actions: []dataquery.DataAction{{
		ID:   "join_later",
		Kind: dataquery.DataActionJoinRecords,
		Params: map[string]string{
			"join_type": "left",
		},
	}}}
	queue := EnqueueDeferredQueue(DeferredQueueState{}, 3, plan, "split staged plan")
	if len(queue.Plan.Actions) != 1 || len(queue.Events) != 1 || queue.Events[0].Action != DeferredQueueTransitionEnqueue {
		t.Fatalf("queue=%+v, want enqueue event and cloned plan", queue)
	}
	plan.Actions[0].Params["join_type"] = "inner"
	if queue.Plan.Actions[0].Params["join_type"] != "left" {
		t.Fatalf("queue plan was mutated through source plan: %+v", queue.Plan.Actions[0].Params)
	}
	status := DeferredDispatchStatus{
		Actions:         1,
		FirstActionID:   "join_later",
		FirstActionKind: string(dataquery.DataActionJoinRecords),
		ReasonCode:      DeferredBlockInputUnavailable,
		Reason:          "right side missing",
	}
	queue = RetainDeferredQueue(queue, 4, status)
	if len(queue.Events) != 2 || queue.Events[1].Action != DeferredQueueTransitionRetain || queue.Events[1].ReasonCode != DeferredBlockInputUnavailable {
		t.Fatalf("queue events=%+v, want retain event", queue.Events)
	}
	dispatched := dataquery.TaskPlan{Actions: queue.Plan.Actions[:1]}
	queue = DispatchDeferredQueue(queue, 5, dispatched, dataquery.TaskPlan{}, DeferredDispatchStatus{Ready: true}, "dispatch next rank")
	if len(queue.Plan.Actions) != 0 || len(queue.Events) != 3 || queue.Events[2].Action != DeferredQueueTransitionDispatch || queue.Events[2].Actions != 1 {
		t.Fatalf("queue=%+v, want dispatch event and empty remainder", queue)
	}
	queue = EnqueueDeferredQueue(queue, 6, plan, "requeue for discard")
	queue = DiscardDeferredQueue(queue, 7, status, "stale queue")
	if len(queue.Plan.Actions) != 0 || len(queue.Events) != 5 || queue.Events[4].Action != DeferredQueueTransitionDiscard {
		t.Fatalf("queue=%+v, want discarded plan and discard event", queue)
	}
}
