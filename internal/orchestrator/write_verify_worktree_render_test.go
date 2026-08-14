package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRenderVerifySuccessDisclosesRetainedUntrackedOutput(t *testing.T) {
	report := &types.ChangeReport{
		PlanID:      "plan-1",
		Passed:      true,
		TestResults: []types.TestResult{{AssertionID: "tests", Passed: true}},
		WorktreeAudit: &types.VerificationWorktreeAudit{
			Status:               types.VerificationWorktreeAuditUntrackedSideEffects,
			UntrackedEffectCount: 1,
			Effects: []types.VerificationWorktreeEffect{{
				Path: "main", Kind: types.VerificationWorktreeEffectUntrackedCreated,
				Ownership: "unproven_generated_artifact", Action: "retained_not_committed_not_auto_deleted",
			}},
		},
	}
	for _, tc := range []struct {
		lang string
		want []string
	}{
		{lang: "zh", want: []string{"测试通过", "`main`", "未纳入交付提交", "未被自动删除", "两个独立事实"}},
		{lang: "en", want: []string{"Tests verified", "`main`", "not part of the delivery commit", "not auto-deleted", "separate facts"}},
	} {
		got := renderVerifySuccess(report, tc.lang)
		for _, want := range tc.want {
			if !strings.Contains(got, want) {
				t.Fatalf("lang=%s missing %q:\n%s", tc.lang, want, got)
			}
		}
	}
}
