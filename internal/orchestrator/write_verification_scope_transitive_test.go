package orchestrator

import (
	"reflect"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestStampCumulativeVerificationScopeKeepsTransitiveClosureAcrossRestore(t *testing.T) {
	t0 := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	planA := &types.ChangePlan{
		ID:                 "plan-a",
		TargetPaths:        []string{"old_test.java"},
		Changes:            []types.FileChange{{Path: "old_test.java", Kind: "modify"}},
		BehaviorContracts:  []types.WriteBehaviorContract{{ID: "contract-a"}},
		VerificationProbes: []types.VerificationProbe{{ID: "probe-a"}},
	}
	planB := &types.ChangePlan{
		ID:                 "plan-b",
		TargetPaths:        []string{"source.java"},
		Changes:            []types.FileChange{{Path: "source.java", Kind: "modify"}},
		BehaviorContracts:  []types.WriteBehaviorContract{{ID: "contract-b"}},
		VerificationProbes: []types.VerificationProbe{{ID: "probe-b"}},
		CumulativeVerificationScope: &types.CumulativeVerificationScope{
			SourcePlanIDs:      []string{planA.ID},
			TargetPaths:        []string{"old_test.java"},
			BehaviorContracts:  append([]types.WriteBehaviorContract(nil), planA.BehaviorContracts...),
			VerificationProbes: append([]types.VerificationProbe(nil), planA.VerificationProbes...),
		},
	}
	planC := &types.ChangePlan{
		ID:          "plan-c",
		TargetPaths: []string{"followup.md"},
		Changes:     []types.FileChange{{Path: "followup.md", Kind: "modify"}},
		// Simulate untrusted planner input. The controller must discard it.
		CumulativeVerificationScope: &types.CumulativeVerificationScope{
			SourcePlanIDs: []string{"planner-injected"},
			TargetPaths:   []string{"outside.txt"},
		},
	}
	run := &types.WriteWorkflowRun{
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-1",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: planA.ID, FinishedAt: t0},
				{Kind: "apply", Status: "applied", PlanID: planB.ID, FinishedAt: t0.Add(time.Minute)},
			},
			SliceEvents: []types.WriteWorkflowSliceEvent{{
				Event:  types.WriteWorkflowSliceEventRestored,
				PlanID: planB.ID,
				At:     t0.Add(2 * time.Minute),
			}},
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			ReasonCode: "checkpoint_restored_before_replan",
			At:         t0.Add(2 * time.Minute),
		}},
	}

	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.stampCumulativeVerificationScope(planC, run, planA, planB)

	got := planC.CumulativeVerificationScope
	if got == nil {
		t.Fatal("cumulative verification scope is nil")
	}
	if !reflect.DeepEqual(got.SourcePlanIDs, []string{"plan-a", "plan-b"}) {
		t.Fatalf("source plan closure = %+v", got.SourcePlanIDs)
	}
	if !reflect.DeepEqual(got.TargetPaths, []string{"old_test.java", "source.java"}) {
		t.Fatalf("target path closure = %+v", got.TargetPaths)
	}
	if ids := behaviorContractIDs(got.BehaviorContracts); !reflect.DeepEqual(ids, []string{"contract-b", "contract-a"}) {
		t.Fatalf("behavior contract closure = %+v", ids)
	}
	if ids := verificationProbeIDs(got.VerificationProbes); !reflect.DeepEqual(ids, []string{"probe-b", "probe-a"}) {
		t.Fatalf("verification probe closure = %+v", ids)
	}
	if verificationScopeContainsString(got.SourcePlanIDs, "planner-injected") || verificationScopeContainsString(got.TargetPaths, "outside.txt") {
		t.Fatalf("planner-owned scope escaped rebuild: %+v", got)
	}
	if paths, _ := types.ActiveChangePlanApplyTargetPaths(planC, nil); !reflect.DeepEqual(paths, []string{"followup.md"}) {
		t.Fatalf("verification closure widened active apply scope: %+v", paths)
	}
}

func behaviorContractIDs(in []types.WriteBehaviorContract) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		out = append(out, item.ID)
	}
	return out
}

func verificationProbeIDs(in []types.VerificationProbe) []string {
	out := make([]string, 0, len(in))
	for _, item := range in {
		out = append(out, item.ID)
	}
	return out
}

func verificationScopeContainsString(in []string, want string) bool {
	for _, item := range in {
		if item == want {
			return true
		}
	}
	return false
}
