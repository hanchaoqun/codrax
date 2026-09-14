package types

import "strings"

// ProjectRunnerChangedPathCapability bounds the existing project-runner
// protocol, independently of the language of its driver or declared inputs.
// The Make adapter emits only a process-scoped aggregate, not a target or
// assertion execution receipt. Its successful command and exact checked-path
// scope remain useful; neither can establish target behavior. Native runner
// protocols retain their established authority. Independently observed target
// probes are resolved separately and may supply stronger evidence.
func ProjectRunnerChangedPathCapability(runner string) VerificationCapability {
	if strings.EqualFold(strings.TrimSpace(runner), "make") {
		return VerificationCapabilityUnknown
	}
	return VerificationCapabilityTargetBehavior
}

// NativeProjectCommandCoversChangedPath checks existing executor-owned native
// command scope. It does not discover new paths or inspect commands/output.
// Producers call it after deriving CoveredPaths; historical readers can reuse
// only those recorded paths. An unknown runner cannot borrow a known framework.
func NativeProjectCommandCoversChangedPath(command ExecutedCommand, path string) bool {
	if command.Outcome != ExecutedCommandOutcomeExecuted || command.ExitCode != 0 ||
		ProjectRunnerChangedPathCapability(command.Runner) == VerificationCapabilityUnknown ||
		!probeTargetRelativePath(path) || len(VerificationLanguageFamiliesFromRunner(command.Runner, "")) == 0 {
		return false
	}
	workingDir := normalizeVerificationWorkingDir(command.WorkingDir)
	if workingDir != "." && (!probeTargetRelativePath(workingDir) ||
		(path != workingDir && !strings.HasPrefix(path, workingDir+"/"))) {
		return false
	}
	covered := false
	for _, member := range command.CoveredPaths {
		covered = covered || member == path
	}
	if !covered {
		return false
	}
	for _, target := range VerificationLanguageFamiliesFromPath(path) {
		if target == VerificationLanguageConfigWorkflow {
			continue
		}
		for _, runtime := range VerificationLanguageFamiliesFromRunner(command.Runner, command.Framework) {
			if target == runtime {
				return true
			}
		}
	}
	return false
}
