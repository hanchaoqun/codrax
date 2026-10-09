package types

import "sort"

// VerifiedNativeTestExecutionPaths projects only exact file execution from
// current delivery receipts. It does not create an execution requirement,
// contract mapping, source-path coverage, or authority from a suite's name.
// Legacy reports cannot acquire this new precision without an invocation ID.
func VerifiedNativeTestExecutionPaths(plan *ChangePlan, report *ChangeReport) []string {
	delivery, ok := ResolveVerificationDelivery(plan)
	if !ok || !delivery.Valid() || report == nil || len(report.ExistingTestExecutions) > MaxExistingTestExecutionReceipts {
		return nil
	}
	index := NewNativeTestInvocationIndex(report)
	passed, failed := map[string]bool{}, map[string]bool{}
	for _, receipt := range report.ExistingTestExecutions {
		if receipt.SourcePlanID != delivery.SourcePlanID || !existingTestExecutionReceiptMatches(plan, delivery, report, receipt, index) ||
			report.ExecutedCommands[receipt.CommandIndex].InvocationID == "" {
			continue
		}
		if receipt.FailedAssertionCount != 0 {
			failed[receipt.TestPath] = true
		} else {
			passed[receipt.TestPath] = true
		}
	}
	var paths []string
	for path := range passed {
		if !failed[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}
