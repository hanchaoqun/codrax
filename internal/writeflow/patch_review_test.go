package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReviewAppliedPatchScopePassesActiveSliceScope(t *testing.T) {
	plan := &types.ChangePlan{
		ID:           "plan-1",
		Status:       types.PlanStatusAppliedPendingVerify,
		TargetPaths:  []string{"a.py", "b.py"},
		AppliedPaths: []string{"a.py"},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{
		ID:    "slice-1",
		Paths: []string{"a.py"},
	})
	if review.HardBlock || review.Status != "passed" {
		t.Fatalf("in-scope patch should pass, got %+v", review)
	}
	if len(review.AppliedPaths) != 1 || review.AppliedPaths[0] != "a.py" {
		t.Fatalf("applied paths not normalized: %+v", review.AppliedPaths)
	}
}

func TestReviewAppliedPatchScopeBlocksOutsideActiveSlice(t *testing.T) {
	plan := &types.ChangePlan{
		ID:           "plan-1",
		Status:       types.PlanStatusAppliedPendingVerify,
		TargetPaths:  []string{"a.py", "b.py"},
		AppliedPaths: []string{"b.py"},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{
		ID:    "slice-1",
		Paths: []string{"a.py"},
	})
	if !review.HardBlock || review.Status != "failed" {
		t.Fatalf("outside-slice patch should hard block, got %+v", review)
	}
	if len(review.Findings) != 1 || review.Findings[0].Code != "applied_path_outside_active_slice" {
		t.Fatalf("unexpected findings: %+v", review.Findings)
	}
}

func TestReviewAppliedPatchScopeBlocksOutsidePlanTarget(t *testing.T) {
	plan := &types.ChangePlan{
		ID:           "plan-1",
		Status:       types.PlanStatusAppliedPendingVerify,
		TargetPaths:  []string{"a.py"},
		AppliedPaths: []string{"c.py"},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{})
	if !review.HardBlock {
		t.Fatalf("outside-plan patch should hard block, got %+v", review)
	}
	if !patchReviewHasFinding(review, "applied_path_outside_plan_scope") {
		t.Fatalf("unexpected findings: %+v", review.Findings)
	}
}

func TestReviewAppliedPatchScopeWarnsWhenAppliedPathsMissing(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"a.py"},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("missing applied_paths should warn, not block: %+v", review)
	}
	if len(review.Findings) != 1 || review.Findings[0].Code != "applied_paths_missing" {
		t.Fatalf("unexpected findings: %+v", review.Findings)
	}
}

func patchReviewHasFinding(review types.PatchReviewRecord, code string) bool {
	for _, finding := range review.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
