package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPendingAppliesForActivePlanScopeUsesActiveSlice(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-slice",
		Changes: []types.FileChange{
			{Path: "a.go", Kind: "create", Rationale: "a"},
			{Path: "b.go", Kind: "create", Rationale: "b"},
			{Path: "c.go", Kind: "create", Rationale: "c"},
		},
		TargetPaths: []string{"a.go", "b.go", "c.go"},
	}
	mu := types.NewMutableState("test")
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchApplying,
			ActiveSliceID: "slice-002",
			Slices: []types.WriteWorkflowSlice{{
				ID:            "slice-001",
				Status:        types.ChangePlanSliceVerified,
				PlanID:        "plan-slice",
				ChangeIndexes: []int{0},
			}, {
				ID:            "slice-002",
				Status:        types.ChangePlanSliceApplying,
				PlanID:        "plan-slice",
				ChangeIndexes: []int{1},
			}},
		}},
	})
	pending, active := pendingAppliesForActivePlanScope(mu, plan, "test")
	if !active {
		t.Fatal("expected active slice pending scope")
	}
	if len(pending) != 1 || pending[0].Path != "b.go" || pending[0].Origin != "test" {
		t.Fatalf("pending = %+v, want only b.go from active slice", pending)
	}
}

func TestPendingAppliesForActivePlanScopeFallsBackToFullPlan(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-full",
		Changes: []types.FileChange{
			{Path: "a.go", Kind: "create", Rationale: "a"},
			{Path: "b.go", Kind: "create", Rationale: "b"},
		},
		TargetPaths: []string{"a.go", "b.go"},
	}
	pending, active := pendingAppliesForActivePlanScope(types.NewMutableState("test"), plan, "test")
	if active {
		t.Fatal("full-plan fallback should not report active slice")
	}
	if len(pending) != 2 || pending[0].Path != "a.go" || pending[1].Path != "b.go" {
		t.Fatalf("pending = %+v, want full plan order", pending)
	}
}
