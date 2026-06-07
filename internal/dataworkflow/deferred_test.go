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
			BlockedReason: "right.json is missing",
		}},
	})
	if ok || status.Ready {
		t.Fatalf("ok/status=%v/%+v, want blocked queue", ok, status)
	}
	if status.ReadyActions != 0 || status.BlockedActions != 1 || status.Reason != "right.json is missing" {
		t.Fatalf("status=%+v, want blocked reason from candidate", status)
	}
}
