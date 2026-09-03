package types

import (
	"strings"
	"testing"
)

// write_final_report_worktree_test.go — V5-2 (§40.11): the final report's
// residual-risk codes keep refused drift as an error and name the disclosed
// lane as a warning carrying path=class rows; the two never merge.
func TestWriteFinalResidualRisksSeparateRefusedAndDisclosedWorktreeDrift(t *testing.T) {
	report := WriteFinalReport{}
	report.Verification.WorktreeAuditStatus = VerificationWorktreeAuditTrackedDrift
	report.Verification.WorktreeEffects = []VerificationWorktreeEffect{
		{Path: "src/other.rs", Kind: VerificationWorktreeEffectTrackedChanged, DriftClass: VerificationWorktreeDriftUnclassified, Disposition: VerificationWorktreeEffectRefused},
	}
	risks := writeFinalResidualRisks(report)
	if !writeFinalHasRisk(risks, "verification_worktree_tracked_drift", "error", "src/other.rs") {
		t.Fatalf("refused drift must stay an error risk: %+v", risks)
	}
	if writeFinalHasRisk(risks, VerificationTrackedSideEffectDisclosedReason, "", "") {
		t.Fatalf("a refused audit must not raise the disclosed code: %+v", risks)
	}
	report.Verification.WorktreeAuditStatus = VerificationWorktreeAuditTrackedDriftDisclosed
	report.Verification.WorktreeEffects = []VerificationWorktreeEffect{
		{Path: "Cargo.lock", Kind: VerificationWorktreeEffectTrackedChanged, DriftClass: VerificationWorktreeDriftDependencyLockfileRefresh, OwnerRunner: "rust", Disposition: VerificationWorktreeEffectDisclosed},
	}
	risks = writeFinalResidualRisks(report)
	if !writeFinalHasRisk(risks, VerificationTrackedSideEffectDisclosedReason, "warning", "Cargo.lock=dependency_lockfile_refresh") {
		t.Fatalf("disclosed drift must be a warning naming path=class: %+v", risks)
	}
	if writeFinalHasRisk(risks, "verification_worktree_tracked_drift", "", "") {
		t.Fatalf("a disclosed audit must not raise the refused error: %+v", risks)
	}
}

func writeFinalHasRisk(risks []WriteFinalResidualRisk, code, severity, detail string) bool {
	for _, risk := range risks {
		if risk.Code != code {
			continue
		}
		if severity != "" && risk.Severity != severity {
			continue
		}
		if detail != "" && !strings.Contains(risk.Detail, detail) {
			continue
		}
		return true
	}
	return false
}

// Review fold-in (§40.36 复核): a mixed refusal lists only refused rows in the
// error risk and names disclosed rows in a separate warning; a disclosed run
// still raises the untracked-output warning.
func TestWriteFinalResidualRisksMixedAndUntrackedWorktreeLanes(t *testing.T) {
	report := WriteFinalReport{}
	report.Verification.WorktreeAuditStatus = VerificationWorktreeAuditTrackedDrift
	report.Verification.WorktreeEffects = []VerificationWorktreeEffect{
		{Path: "Cargo.lock", Kind: VerificationWorktreeEffectTrackedChanged, DriftClass: VerificationWorktreeDriftDependencyLockfileRefresh, OwnerRunner: "rust", Disposition: VerificationWorktreeEffectDisclosed},
		{Path: "src/other.rs", Kind: VerificationWorktreeEffectTrackedChanged, DriftClass: VerificationWorktreeDriftUnclassified, Disposition: VerificationWorktreeEffectRefused},
	}
	risks := writeFinalResidualRisks(report)
	if !writeFinalHasRisk(risks, "verification_worktree_tracked_drift", "error", "src/other.rs") {
		t.Fatalf("refused row must be the error: %+v", risks)
	}
	for _, risk := range risks {
		if risk.Code == "verification_worktree_tracked_drift" && strings.Contains(risk.Detail, "Cargo.lock") {
			t.Fatalf("the disclosed row must not leak into the refused error: %+v", risk)
		}
	}
	if !writeFinalHasRisk(risks, VerificationTrackedSideEffectDisclosedReason, "warning", "Cargo.lock=dependency_lockfile_refresh") {
		t.Fatalf("the disclosed row must be named as a warning beside the refusal: %+v", risks)
	}
	report.Verification.WorktreeAuditStatus = VerificationWorktreeAuditTrackedDriftDisclosed
	report.Verification.WorktreeEffects = []VerificationWorktreeEffect{
		{Path: "Cargo.lock", Kind: VerificationWorktreeEffectTrackedChanged, DriftClass: VerificationWorktreeDriftDependencyLockfileRefresh, Disposition: VerificationWorktreeEffectDisclosed},
		{Path: "generated.bin", Kind: VerificationWorktreeEffectUntrackedCreated},
	}
	risks = writeFinalResidualRisks(report)
	if !writeFinalHasRisk(risks, "verification_worktree_untracked_side_effects", "warning", "generated.bin") {
		t.Fatalf("a disclosed run must still name its retained untracked output: %+v", risks)
	}
}
