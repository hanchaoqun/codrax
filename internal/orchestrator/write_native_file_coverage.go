package orchestrator

import (
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
	"path"
)

// This map is revocation scope only. An exact modern file selector prevents a
// saved "verified" label bypassing a missing/rejected receipt; it never grants
// coverage and a directory selector does not expand to its children.
func nativeUnittestFileReceiptScopes(report *types.ChangeReport) map[string]bool {
	scopes := map[string]bool{}
	if report == nil {
		return scopes
	}
	for _, command := range report.ExecutedCommands {
		if !nativeUnittestCommandNeedsFileReceipt(command) {
			continue
		}
		target := path.Join(command.WorkingDir, command.Suite)
		if types.ExistingTestExactFileSelector(command.Runner, command.Framework, command.WorkingDir, command.Suite, target) {
			scopes[target] = true
		}
	}
	return scopes
}

// Identified unittest producers have a physical exact-file receipt protocol.
// Their display suite must not bypass that protocol when a receipt is absent
// or rejected (skip, indirect import, stale delivery, ambiguous invocation).
// Entirely legacy reports retain their existing compatibility path.
func nativeUnittestCommandNeedsFileReceipt(command types.ExecutedCommand) bool {
	return command.InvocationID != "" && command.Runner == "python" && command.Framework == "unittest"
}

func nativeUnittestResultNeedsFileReceipt(report *types.ChangeReport, result types.TestResult) bool {
	if result.InvocationID == "" {
		return false
	}
	for _, command := range report.ExecutedCommands {
		if command.InvocationID == result.InvocationID && nativeUnittestCommandNeedsFileReceipt(command) {
			return true
		}
	}
	return false
}

// A cumulative review has a synthetic plan identity. Validate receipts against
// their actual current delivery first, then copy only exact test-file execution
// into that view. No receipt, contract, source coverage or old batch is rewritten.
func applyNativeTestFileCoverage(view, source *types.ChangePlan, report *types.ChangeReport, err error) {
	if view == nil || err != nil || report == nil || !report.Passed || report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		return
	}
	paths := map[string]bool{}
	for _, path := range types.VerifiedNativeTestExecutionPaths(source, report) {
		paths[path] = true
	}
	if view.ImpactAnalysis != nil {
		analysis := types.NormalizeImpactAnalysisResult(*view.ImpactAnalysis)
		for i := range analysis.VerificationTargets {
			row := &analysis.VerificationTargets[i]
			if row.Kind == "test_surface" && paths[normalizeVerifyCoveragePath(firstNonEmptyController(row.RelatedPath, row.Path))] {
				row.CoverageStatus = impactCoverageVerified
			}
		}
		view.ImpactAnalysis = &analysis
	}
	if view.PatchReview != nil {
		review := types.NormalizePatchReviewRecord(*view.PatchReview)
		for i := range review.Findings {
			row := &review.Findings[i]
			if row.Category == types.PatchReviewCategorySemanticCoverage && row.Code == "related_test_surface_unverified" && paths[normalizeVerifyCoveragePath(firstNonEmptyController(row.RelatedPath, row.Path))] {
				row.CoverageStatus = types.PatchReviewCoverageVerified
			}
		}
		view.PatchReview = &review
	}
}

func nativeTestFileRetryCandidates(plan *types.ChangePlan, report *types.ChangeReport) []string {
	if plan == nil || report == nil || report.PlanID != plan.ID || report.Channel != types.ChangeReportChannelPostApplyVerify {
		return nil
	}
	return tool.NativeTestFileExecutionCandidates(plan.WorktreePath, plan, report.TestSurface)
}
