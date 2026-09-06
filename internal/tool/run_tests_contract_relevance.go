package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

type projectTestObservationExecutionMatch struct {
	CandidateIndex int
	CommandIndex   int
	ResultIndex    int
}

// BuildVerifyFailureContractRelevance binds project assertions through the
// runner's execution and path resolver before the types layer projects refs.
// It only reads typed report/plan fields and never executes a tool.
func BuildVerifyFailureContractRelevance(report *types.ChangeReport, plan *types.ChangePlan) types.VerifyFailureContractRelevance {
	var bindings []types.ProjectTestFailureBinding
	if report != nil && plan != nil {
		for _, observation := range types.ChangePlanVerificationProjectTestObservations(plan) {
			for _, match := range projectTestObservationExecutionMatches(observation, report, false) {
				if !projectTestFailureExecutionIsUnambiguous(observation, report, match) {
					continue
				}
				bindings = append(bindings, types.ProjectTestFailureBinding{
					ObservationID: observation.ID, TestPath: observation.TestPath,
					AssertionSuite: observation.AssertionSuite, AssertionID: observation.AssertionID,
					ResultIndex: match.ResultIndex,
				})
			}
		}
	}
	return types.BuildVerifyFailureContractRelevance(report, plan, bindings...)
}

// TestResult does not yet carry the execution that produced it. A report with
// more than one failed project execution therefore cannot assign a failed row
// to one command merely by matching its display names. Retain expectations
// until an exact execution source is available; never guess ownership.
func projectTestFailureExecutionIsUnambiguous(observation types.ProjectTestObservation, report *types.ChangeReport, match projectTestObservationExecutionMatch) bool {
	owner := report.ExecutedCommands[match.CommandIndex]
	ownerKey := testSurfaceCandidateKey(owner.Runner, owner.Framework, owner.WorkingDir)
	for _, command := range report.ExecutedCommands {
		if strings.TrimSpace(command.Outcome) != types.ExecutedCommandOutcomeExecuted || command.ExitCode == 0 ||
			strings.TrimSpace(command.Runner) == "verification_probe" {
			continue
		}
		if testSurfaceCandidateKey(command.Runner, command.Framework, command.WorkingDir) != ownerKey ||
			strings.TrimSpace(command.Suite) != strings.TrimSpace(owner.Suite) {
			return false
		}
	}
	candidate := report.TestSurface.Candidates[match.CandidateIndex]
	if candidate.Runner == "make" {
		// A Make target can execute many files while its nested assertion rows
		// omit the file. Its bounded roster proves the member only when that
		// roster has one unique path; a many-file roster is not a row binding.
		paths := map[string]struct{}{}
		for _, raw := range candidate.DeclaredCoveragePaths {
			if path := cleanRepoRelPath(raw); path != "" {
				paths[path] = struct{}{}
			}
		}
		_, contains := paths[cleanRepoRelPath(observation.TestPath)]
		return contains && len(paths) == 1
	}
	return true
}
