package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

const requiredExistingTestContinuation = "required_existing_test_execution"

// Called only on one parsed native invocation, before merging project reports.
// Probe/syntax/skip/zero-test rows never mint an execution receipt. Selection
// precision and consumer precision use the same protocol allowlist.
func existingTestExecutionReceipts(ctx *types.BusContext, invocation runnerPlan, leaf *types.ChangeReport, surface types.TestSurface, commands []types.ExecutedCommand, observed *existingTestUnittestInvocation) []types.ExistingTestExecutionReceipt {
	if ctx == nil || ctx.Mutable == nil || leaf == nil || len(commands) == 0 || observed == nil || len(observed.rows) != len(leaf.TestResults) {
		return nil
	}
	plan := ctx.Mutable.ChangePlan()
	sha, current := observed.delivery.currentTestSHA(ctx, observed.target)
	delivery := observed.delivery.snapshot
	effect := delivery.PatchEffect
	if !current || sha != observed.targetSHA || !observed.registrationMatches(ctx) || effect == nil || effect.RecordID == "" || effect.DiffFingerprint == "" {
		return nil
	}
	commandIndex := len(commands) - 1
	cmd := commands[commandIndex]
	wd := runnerPlanRel(ctx.RepoRoot, invocation)
	if cmd.Runner != invocation.Runner || cmd.Framework != invocation.Framework || cmd.WorkingDir != wd || cmd.Suite != strings.TrimSpace(invocation.Suite) || cmd.Outcome != types.ExecutedCommandOutcomeExecuted || cmd.ProbeExecution != nil || cmd.SourceCheckExecution != nil {
		return nil
	}
	assertions, failures := 0, 0
	var digests []string
	for i, row := range leaf.TestResults {
		origin := observed.rows[i]
		if row.ObservationScope != types.TestObservationScopeAssertion || row.AssertionID == "" || origin.Source != observed.targetAbs || origin.Module != observed.targetAbs || origin.SHA256 != observed.targetSHA {
			continue
		}
		assertions++
		digests = append(digests, types.ExistingTestAssertionDigest(row))
		if !row.Passed {
			failures++
		}
	}
	if assertions == 0 || (cmd.ExitCode == 0) != (failures == 0) {
		return nil
	}
	candidateID := ""
	for _, c := range surface.Candidates {
		if c.HasTestSignal && c.Runner == cmd.Runner && c.Framework == cmd.Framework && c.WorkingDir == wd {
			candidateID = c.ID
			break
		}
	}
	if candidateID == "" {
		return nil
	}
	var out []types.ExistingTestExecutionReceipt
	for _, target := range nativeObservedTestPaths(plan) {
		if target != observed.target || safeImpactRelatedPath(ctx.RepoRoot, target) == "" || !types.ExistingTestExactFileSelector(cmd.Runner, cmd.Framework, wd, cmd.Suite, target) {
			continue
		}
		out = append(out, types.ExistingTestExecutionReceipt{PlanID: plan.ID, SourcePlanID: delivery.SourcePlanID, AppliedCommitSHA: delivery.AppliedCommitSHA, PatchEffectID: effect.RecordID, DiffFingerprint: effect.DiffFingerprint, TestPath: target, CandidateID: candidateID, Runner: cmd.Runner, Framework: cmd.Framework, WorkingDir: wd, Suite: cmd.Suite, CommandIndex: commandIndex, AssertionCount: assertions, FailedAssertionCount: failures, TestFileSHA256: observed.targetSHA, CommandSHA256: types.ExistingTestExecutionDigest(cmd.Command), AssertionDigests: digests, NativeTestRegistrationDigest: observed.registrationDigest})
	}
	return out
}

func appendExistingTestExecutionReceipts(prior, next []types.ExistingTestExecutionReceipt) []types.ExistingTestExecutionReceipt {
	for _, receipt := range next {
		if len(prior) == types.MaxExistingTestExecutionReceipts {
			break
		}
		prior = append(prior, receipt)
	}
	return prior
}
