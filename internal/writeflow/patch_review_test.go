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

func TestReviewAppliedPatchScopeBlocksActualDiffOutsidePlanTarget(t *testing.T) {
	plan := &types.ChangePlan{
		ID:           "plan-1",
		Status:       types.PlanStatusAppliedPendingVerify,
		TargetPaths:  []string{"a.py"},
		AppliedPaths: []string{"a.py"},
		PatchEffect: &types.PatchEffectRecord{
			RecordID:        "patch-effect:plan-1:slice-1:abcdef123456",
			DiffFingerprint: "abcdef1234567890",
			Files: []types.PatchEffectFile{{
				Path:   "c.py",
				Status: "modified",
			}},
		},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{})
	if !review.HardBlock {
		t.Fatalf("actual diff outside plan should hard block, got %+v", review)
	}
	if review.PatchEffectID == "" || review.DiffFingerprint == "" {
		t.Fatalf("patch effect metadata missing from review: %+v", review)
	}
	if !patchReviewHasFinding(review, "patch_effect_path_outside_plan_scope") {
		t.Fatalf("unexpected findings: %+v", review.Findings)
	}
	if !patchReviewPathPresent(review.AppliedPaths, "a.py") || !patchReviewPathPresent(review.AppliedPaths, "c.py") {
		t.Fatalf("applied paths should merge declared and actual diff paths: %+v", review.AppliedPaths)
	}
}

func TestReviewAppliedPatchScopeBlocksActualDiffOutsideActiveSlice(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"a.py", "b.py"},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-1:slice-1:abcdef123456",
			Files: []types.PatchEffectFile{{
				Path:   "b.py",
				Status: "modified",
			}},
		},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{
		ID:    "slice-1",
		Paths: []string{"a.py"},
	})
	if !review.HardBlock {
		t.Fatalf("actual diff outside active slice should hard block, got %+v", review)
	}
	if !patchReviewHasFinding(review, "patch_effect_path_outside_active_slice") {
		t.Fatalf("unexpected findings: %+v", review.Findings)
	}
}

func TestReviewAppliedPatchScopeUsesPatchEffectWhenDeclaredAppliedPathsMissing(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"a.py"},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-1:slice-1:abcdef123456",
			Files: []types.PatchEffectFile{{
				Path:   "a.py",
				Status: "modified",
			}},
		},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{})
	if review.HardBlock || review.Status != "passed" {
		t.Fatalf("in-scope actual diff should pass, got %+v", review)
	}
	if patchReviewHasFinding(review, "applied_paths_missing") {
		t.Fatalf("actual diff paths should satisfy applied-path evidence: %+v", review.Findings)
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

func patchReviewPathPresent(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

func patchReviewHasFinding(review types.PatchReviewRecord, code string) bool {
	for _, finding := range review.Findings {
		if finding.Code == code {
			return true
		}
	}
	return false
}
