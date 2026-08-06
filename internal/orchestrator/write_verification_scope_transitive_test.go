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

func TestStampCumulativeVerificationScopePreservesControllerScopeOnExactPlanRestore(t *testing.T) {
	t0 := time.Date(2026, 8, 2, 13, 0, 0, 0, time.UTC)
	restored := &types.ChangePlan{
		ID:                 "plan-b",
		TargetPaths:        []string{"source.java"},
		Changes:            []types.FileChange{{Path: "source.java", Kind: "modify"}},
		BehaviorContracts:  []types.WriteBehaviorContract{{ID: "contract-b"}},
		VerificationProbes: []types.VerificationProbe{{ID: "probe-b"}},
		CumulativeVerificationScope: &types.CumulativeVerificationScope{
			SourcePlanIDs:      []string{"plan-a"},
			TargetPaths:        []string{"old_test.java"},
			BehaviorContracts:  []types.WriteBehaviorContract{{ID: "contract-a"}},
			VerificationProbes: []types.VerificationProbe{{ID: "probe-a"}},
		},
	}
	run := &types.WriteWorkflowRun{
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-1",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-a", FinishedAt: t0},
				{Kind: "apply", Status: "applied", PlanID: restored.ID, FinishedAt: t0.Add(time.Minute)},
			},
			SliceEvents: []types.WriteWorkflowSliceEvent{{
				Event:  types.WriteWorkflowSliceEventRestored,
				PlanID: restored.ID,
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
	// The scheduler passes priorPlan as a candidate. Pointer identity is the
	// precise signal that this is the controller-restored object rather than a
	// new planner-authored plan with an untrusted cumulative field.
	o.stampCumulativeVerificationScope(restored, run, restored)

	got := restored.CumulativeVerificationScope
	if got == nil {
		t.Fatal("restored plan lost its controller-owned cumulative verification scope")
	}
	if !reflect.DeepEqual(got.SourcePlanIDs, []string{"plan-a"}) {
		t.Fatalf("restored source plan closure = %+v", got.SourcePlanIDs)
	}
	if !reflect.DeepEqual(got.TargetPaths, []string{"old_test.java"}) {
		t.Fatalf("restored target path closure = %+v", got.TargetPaths)
	}
	if ids := behaviorContractIDs(got.BehaviorContracts); !reflect.DeepEqual(ids, []string{"contract-a"}) {
		t.Fatalf("restored behavior contract closure = %+v", ids)
	}
	if ids := verificationProbeIDs(got.VerificationProbes); !reflect.DeepEqual(ids, []string{"probe-a"}) {
		t.Fatalf("restored verification probe closure = %+v", ids)
	}
	if paths, _ := types.ActiveChangePlanApplyTargetPaths(restored, nil); !reflect.DeepEqual(paths, []string{"source.java"}) {
		t.Fatalf("verification closure widened restored plan apply scope: %+v", paths)
	}
}

func TestStampCumulativeVerificationScopeActiveGenerationShadowsSameIDRetainedRows(t *testing.T) {
	t0 := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	retained := &types.ChangePlan{
		ID:          "plan-old",
		TargetPaths: []string{"old.c"},
		Changes:     []types.FileChange{{Path: "old.c", Kind: "patch"}},
		BehaviorContracts: []types.WriteBehaviorContract{
			{ID: "outcome-2", Expected: "line 16 remains unchanged"},
			{ID: "retained-only", Expected: "line 12 remains repaired"},
			{ID: "stale-fallback", Expected: "only one expression changes", Source: types.WriteBehaviorContractSourceExpectedOutcomeFallback},
		},
		VerificationProbes: []types.VerificationProbe{
			{ID: "behavior-probe", Code: "old generation"},
			{ID: "retained-probe", Code: "retained generation"},
		},
	}
	active := &types.ChangePlan{
		ID:          "plan-new",
		TargetPaths: []string{"new.c"},
		Changes:     []types.FileChange{{Path: "new.c", Kind: "patch"}},
		BehaviorContracts: []types.WriteBehaviorContract{
			{ID: "outcome-2", Expected: "line 16 returns the negative lookup error"},
		},
		BehaviorContractGeneration: types.WriteBehaviorContractGenerationPlanAcceptanceRebase,
		VerificationProbes: []types.VerificationProbe{
			{ID: "behavior-probe", Code: "current generation"},
		},
	}
	run := &types.WriteWorkflowRun{Batches: []types.WriteWorkflowBatch{{
		ID: "batch-1",
		Attempts: []types.WriteWorkflowAttempt{{
			Kind: "apply", Status: "applied", PlanID: retained.ID, FinishedAt: t0,
		}},
	}}}

	o := New(types.PipelineSettings{}, nil, nil, nil)
	o.stampCumulativeVerificationScope(active, run, retained)

	got := active.CumulativeVerificationScope
	if got == nil {
		t.Fatal("expected retained verification scope")
	}
	if ids := behaviorContractIDs(got.BehaviorContracts); !reflect.DeepEqual(ids, []string{"retained-only"}) {
		t.Fatalf("same-ID and prior-generation fallback contracts must not re-enter cumulative scope: %+v", got.BehaviorContracts)
	}
	if ids := verificationProbeIDs(got.VerificationProbes); !reflect.DeepEqual(ids, []string{"retained-probe"}) {
		t.Fatalf("same-ID stale probe must not re-enter cumulative scope: %+v", got.VerificationProbes)
	}
	combined := types.ChangePlanVerificationBehaviorContracts(active)
	if ids := behaviorContractIDs(combined); !reflect.DeepEqual(ids, []string{"outcome-2", "retained-only"}) {
		t.Fatalf("verification view must retain active generation plus unique prior rows: %+v", combined)
	}
	if combined[0].Expected != "line 16 returns the negative lookup error" {
		t.Fatalf("active contract generation lost authority: %+v", combined[0])
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
