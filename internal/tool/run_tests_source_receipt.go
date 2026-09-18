package tool

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Every receipt belongs to one actual process, not the aggregate Passed bit.
// exactFiles is supplied only when those files are explicit checker inputs;
// project builds without an input manifest retain their execution but no
// changed-path authority. Preparation/lookup failures never invent exit zero.
func runRecordedSourceCheckCommand(commands *[]types.ExecutedCommand, ctx *types.BusContext, dir string, env []string, exactFiles []string, binary string, args ...string) ([]byte, error) {
	output, err, command := executeSourceCheckCommand(ctx, dir, env, binary, args...)
	if command.SourceCheckExecution.Completed && command.SourceCheckExecution.ExitCodeKnown && command.ExitCode == 0 {
		for _, file := range exactFiles {
			if !filepath.IsAbs(file) {
				file = filepath.Join(dir, file)
			}
			if absolute, absErr := filepath.Abs(file); absErr == nil {
				command.CoveredPaths = append(command.CoveredPaths, absolute)
			}
		}
		command.SourceCheckExecution.CheckedPaths = append([]string(nil), command.CoveredPaths...)
	}
	*commands = append(*commands, command)
	return output, err
}

func executeSourceCheckCommand(ctx *types.BusContext, dir string, env []string, binary string, args ...string) ([]byte, error, types.ExecutedCommand) {
	parent := ctx.Context()
	receipt := &types.SourceCheckExecutionReceipt{Version: types.SourceCheckExecutionReceiptVersion, ExitCode: -1}
	command := types.ExecutedCommand{Command: shellQuoteWord(binary), WorkingDir: dir, ExitCode: -1, SourceCheckExecution: receipt}
	for _, arg := range args {
		command.Command += " " + shellQuoteWord(arg)
	}
	if err := parent.Err(); err != nil {
		return nil, err, command
	}
	cmd := exec.CommandContext(parent, binary, args...)
	cmd.Dir, cmd.Env = dir, env
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	started := time.Now()
	result := SupervisedRun(parent, cmd, SupervisedRunOptions{})
	command.DurationMS = time.Since(started).Milliseconds()
	receipt.Started = cmd.Process != nil
	// Do not read ProcessState if Wait may still be running. Cancellation is
	// an interrupted attempt even if a concurrent process exit happened to be 0.
	if err := parent.Err(); err != nil {
		return output.Bytes(), err, command
	}
	if !errors.Is(result.Err, errSupervisedWaitIncomplete) && cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		receipt.ExitCodeKnown = true
		receipt.ExitCode = cmd.ProcessState.ExitCode()
		command.ExitCode = receipt.ExitCode
		var exitError *exec.ExitError
		receipt.Completed = result.ExitKind == SupervisedExitNormal && (result.Err == nil || errors.As(result.Err, &exitError))
	}
	return output.Bytes(), result.Err, command
}

// Named-return providers attach all completed/failed/interrupted attempts on
// every return path. Parent providers explicitly accumulate child receipts;
// generic report merging deliberately remains unchanged.
func attachSourceCheckCommands(report **types.ChangeReport, commands *[]types.ExecutedCommand) {
	if *report != nil {
		(*report).ExecutedCommands = append((*report).ExecutedCommands, (*commands)...)
	}
}

// Both source-check entry lanes publish the same leaf receipts. Keep their
// syntax outcome: ordinary executed commands have broader runner-scope rules.
func sourceCheckCommandsForReport(repoRoot string, plan runnerPlan, source, outcome string, report *types.ChangeReport) []types.ExecutedCommand {
	var commands []types.ExecutedCommand
	if report != nil {
		commands = append(commands, report.ExecutedCommands...)
	}
	if len(commands) == 0 {
		commands = []types.ExecutedCommand{{ExitCode: -1, SourceCheckExecution: &types.SourceCheckExecutionReceipt{Version: types.SourceCheckExecutionReceiptVersion, ExitCode: -1}}}
	}
	for i := range commands {
		cmd := &commands[i]
		cmd.Runner, cmd.Framework = plan.Runner, plan.Framework
		executionPlan := plan
		if cmd.WorkingDir != "" {
			executionPlan.Root = cmd.WorkingDir
		}
		cmd.WorkingDir, cmd.Suite = runnerPlanRel(repoRoot, executionPlan), strings.TrimSpace(plan.Suite)
		cmd.Source, cmd.Outcome = source, outcome
		cmd.CoveredPaths = nil
		if cmd.SourceCheckExecution != nil {
			receipt := *cmd.SourceCheckExecution
			receipt.CheckedPaths = repoRelativeCoveragePaths(repoRoot, receipt.CheckedPaths)
			cmd.SourceCheckExecution = &receipt
			cmd.CoveredPaths = append([]string(nil), receipt.CheckedPaths...)
		}
	}
	return commands
}

// A command failure without an attributable compiler diagnostic is not a
// source error and not a pass. Preserve its actual exit/diagnostic separately.
func sourceCompileExecutionUnavailableReport(runner, detail string) *types.ChangeReport {
	return &types.ChangeReport{
		Passed: false, VerificationStatus: types.VerificationStatusUnavailable,
		FailureKind: types.FailureKindVerificationIncomplete, FailureReasonCode: "source_compile_execution_unavailable",
		NoTestsRunners: []string{runner}, FailureSummary: "Source checker did not complete successfully; no source-error verdict is available. " + detail,
	}
}
