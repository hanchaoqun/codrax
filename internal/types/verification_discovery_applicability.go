package types

import "strings"

const verificationDefaultDiscoveryAdvisory = "default_discovery_no_additional_requirement"

// An empty convention-only discovery invocation is not a second test
// obligation when all required files have exact, successful native receipts
// in nested packages in this very report. Keep the actual zero-test command
// visible; it proves neither execution nor coverage. Explicit/configured
// suites, other failures, and incomplete or ambiguous reports are unchanged.
func verificationDefaultDiscoveryAdvisories(plan *ChangePlan, report *ChangeReport) map[int]bool {
	if plan == nil || report.PlanID != plan.ID || report.Channel != ChangeReportChannelPostApplyVerify ||
		report.NormalizeVerificationStatus() != VerificationStatusPassed || report.TestSurface == nil ||
		len(report.ExistingTestExecutions) == 0 || len(report.ExistingTestExecutions) > MaxExistingTestExecutionReceipts {
		return nil
	}
	files := map[string]bool{}
	for _, file := range RequiredExistingTestPaths(plan) {
		if file == "" {
			return nil
		}
		files[file] = true
	}
	if plan.NativeTestRegistration != nil || plan.PersistenceKind == PlanPersistenceNativeTestRegistration {
		if NativeTestRegistrationDigest(plan) == "" {
			return nil
		}
		for _, file := range plan.NativeTestRegistration.Tests {
			files[file.Path] = true
		}
	}
	if len(files) == 0 || len(files) > MaxExistingTestExecutionReceipts {
		return nil
	}
	delivery, valid := ResolveVerificationDelivery(plan)
	if !valid {
		return nil
	}
	invocations := NewNativeTestInvocationIndex(report)
	type packageScope struct{ runner, framework, dir string }
	proofs := make(map[string][]packageScope, len(files))
	for _, receipt := range report.ExistingTestExecutions {
		if !files[receipt.TestPath] || !existingTestExecutionReceiptMatches(plan, delivery, report, receipt, invocations) {
			continue
		}
		if receipt.FailedAssertionCount != 0 {
			return nil
		}
		proofs[receipt.TestPath] = append(proofs[receipt.TestPath], packageScope{receipt.Runner, receipt.Framework, receipt.WorkingDir})
	}
	if len(proofs) != len(files) {
		return nil
	}
	resultInvocations := make(map[string]bool, len(invocations.resultIDs))
	for _, id := range invocations.resultIDs {
		resultInvocations[id] = true
	}
	// The compatibility ledger key does not include an ordinary invocation ID.
	// Keep duplicate display keys conservative instead of changing how old
	// ledgers deduplicate or letting one advisory rename another execution.
	counts := map[string]int{}
	for _, cmd := range report.ExecutedCommands {
		if cmd.Source == TestExecutionSourceSurfaceDefault && cmd.Outcome == ExecutedCommandOutcomeZeroTests {
			counts[verificationProofCommandLedgerID(report.PlanID, cmd)]++
		}
	}
	out := map[int]bool{}
	for i, cmd := range report.ExecutedCommands {
		if cmd.Source != TestExecutionSourceSurfaceDefault || cmd.Outcome != ExecutedCommandOutcomeZeroTests ||
			cmd.ExitCode != 0 || cmd.Suite != "" || cmd.WorkingDir == "" || cmd.Command == "" ||
			cmd.InvocationID == "" || invocations.commandOwners[cmd.InvocationID] != i || resultInvocations[cmd.InvocationID] ||
			cmd.ProbeExecution != nil || cmd.SourceCheckExecution != nil || len(cmd.CoveredPaths) != 0 ||
			counts[verificationProofCommandLedgerID(report.PlanID, cmd)] != 1 || !verificationConventionDiscoveryCandidate(report.TestSurface, cmd) {
			continue
		}
		allNested := true
		for _, scopes := range proofs {
			for _, scope := range scopes {
				if scope.runner != cmd.Runner || scope.framework != cmd.Framework || scope.dir == cmd.WorkingDir ||
					(cmd.WorkingDir != "." && !strings.HasPrefix(scope.dir, cmd.WorkingDir+"/")) {
					allNested = false
					break
				}
			}
			if !allNested {
				break
			}
		}
		if allNested {
			out[i] = true
		}
	}
	return out
}

func verificationConventionDiscoveryCandidate(surface *TestSurface, cmd ExecutedCommand) bool {
	matched := false
	for _, candidate := range surface.Candidates {
		if candidate.Runner != cmd.Runner || candidate.Framework != cmd.Framework || candidate.WorkingDir != cmd.WorkingDir {
			continue
		}
		if matched || candidate.ID == "" || !candidate.HasTestSignal || candidate.Source != TestSurfaceSourceFileConvention ||
			candidate.MakeTarget != "" || len(candidate.DeclaredCoveragePaths) != 0 || len(candidate.DeclaredExecutionLanguageFamilies) != 0 {
			return false
		}
		matched = true
	}
	return matched
}
