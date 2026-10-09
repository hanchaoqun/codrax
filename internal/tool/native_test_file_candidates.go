package tool

import "github.com/hanchaoqun/codrax/internal/types"

// NativeTestFileExecutionCandidates reports supported exact-file selections
// from the same current filesystem/typed target compiler used by RunTests.
// This is continuation guidance only, never proof of execution or behavior.
func NativeTestFileExecutionCandidates(repoRoot string, plan *types.ChangePlan, surface *types.TestSurface) []string {
	if surface == nil || repoRoot == "" || plan == nil {
		return nil
	}
	selected := impactRunnerPlansFromChangePlan(repoRoot, *surface, plan)
	var out []string
	for _, target := range nativeObservedTestPaths(plan) {
		for _, invocation := range selected {
			wd := runnerPlanRel(repoRoot, invocation)
			if !types.ExistingTestExactFileSelector(invocation.Runner, invocation.Framework, wd, invocation.Suite, target) {
				continue
			}
			matches := 0
			for _, candidate := range surface.Candidates {
				if candidate.HasTestSignal && candidate.Runner == invocation.Runner && candidate.Framework == invocation.Framework && candidate.WorkingDir == wd {
					matches++
				}
			}
			if matches == 1 {
				out = append(out, target)
				break
			}
		}
	}
	return out
}
