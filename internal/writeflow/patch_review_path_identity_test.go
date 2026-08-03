package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReviewAppliedPatchCanonicalPathAliasesDoNotEscapeScope(t *testing.T) {
	plan := &types.ChangePlan{
		ID:           "plan-path-identity",
		Status:       types.PlanStatusAppliedPendingVerify,
		TargetPaths:  []string{"src/main//java/App.java"},
		AppliedPaths: []string{"./src/main/java/App.java"},
		PatchEffect: &types.PatchEffectRecord{
			Files: []types.PatchEffectFile{{Path: "src/main/java/App.java", Status: "modified"}},
		},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{
		ID:    "slice-path-identity",
		Paths: []string{"src/main//java/App.java"},
	})
	if review.HardBlock || review.Status != "passed" {
		t.Fatalf("lexical aliases of one path must remain in scope: %+v", review)
	}
	want := "src/main/java/App.java"
	if len(review.TargetPaths) != 1 || review.TargetPaths[0] != want {
		t.Fatalf("target paths=%v, want [%s]", review.TargetPaths, want)
	}
	if len(review.AppliedPaths) != 1 || review.AppliedPaths[0] != want {
		t.Fatalf("applied paths=%v, want [%s]", review.AppliedPaths, want)
	}
}
