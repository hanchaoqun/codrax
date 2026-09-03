package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// run_tests_worktree_summary_test.go — V5-2: the run_tests summary keeps
// refused and disclosed rows distinguishable.
func TestRenderRunTestsWorktreeAuditSummaryDistinguishesDisclosedRows(t *testing.T) {
	report := &types.ChangeReport{WorktreeAudit: &types.VerificationWorktreeAudit{
		Status: types.VerificationWorktreeAuditTrackedDriftDisclosed, DisclosedTrackedEffectCount: 1,
		Effects: []types.VerificationWorktreeEffect{{Path: "Cargo.lock", Kind: types.VerificationWorktreeEffectTrackedChanged,
			DriftClass: types.VerificationWorktreeDriftDependencyLockfileRefresh, OwnerRunner: "rust", Disposition: types.VerificationWorktreeEffectDisclosed}},
	}}
	got := renderRunTestsWorktreeAuditSummary(report)
	if !strings.Contains(got, "disclosed") || !strings.Contains(got, "Cargo.lock=dependency_lockfile_refresh(rust)") || strings.Contains(got, "verification failed") {
		t.Fatalf("disclosed summary = %q", got)
	}
	report.WorktreeAudit.Status = types.VerificationWorktreeAuditTrackedDrift
	report.WorktreeAudit.RefusedTrackedEffectCount = 1
	report.WorktreeAudit.Effects = append(report.WorktreeAudit.Effects, types.VerificationWorktreeEffect{Path: "src/other.rs",
		Kind: types.VerificationWorktreeEffectTrackedChanged, DriftClass: types.VerificationWorktreeDriftUnclassified, Disposition: types.VerificationWorktreeEffectRefused})
	got = renderRunTestsWorktreeAuditSummary(report)
	if !strings.Contains(got, "verification failed: src/other.rs") || !strings.Contains(got, "Cargo.lock=dependency_lockfile_refresh(rust)") || strings.Contains(got, "failed: Cargo.lock") {
		t.Fatalf("mixed summary must name refused paths as failed and disclosed rows separately: %q", got)
	}
}
