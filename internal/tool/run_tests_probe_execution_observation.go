package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Keep the raw execution output before a subsequent project suite can replace
// failed probe result rows. This is a display-only addition to existing
// diagnostics, not a new failure, comparator, or continuation classification.
func retainVerificationProbeExecutionOutput(ctx *types.BusContext, planID string, probe types.VerificationProbe, result *verificationProbeRunResult) {
	if ctx == nil || result == nil || result.Report == nil || result.Report.Passed ||
		planID == "" || probe.ID == "" || strings.TrimSpace(result.Output) == "" {
		return
	}
	if result.OutputCommandIndex < 0 || result.OutputCommandIndex >= len(result.Commands) {
		return
	}
	command := result.Commands[result.OutputCommandIndex]
	receipt := command.ProbeExecution
	if receipt == nil || receipt.DefinitionSHA256 != verificationProbeExecutionDigest(probe) || command.Runner != "verification_probe" {
		return
	}
	observation := types.VerificationProbeExecutionObservation{
		PlanID: planID, ProbeID: probe.ID, ExecutionID: receipt.ExecutionID,
		DefinitionSHA256: receipt.DefinitionSHA256, InvocationSHA256: receipt.InvocationSHA256,
		OutputExcerpt: boundedRunnerFailureDetail(result.Output),
	}
	// This temporary view validates identity only. It is never installed
	// as a diagnostic: Python/Java may not create their actual diagnostic
	// until finishReportForPlan, after suite continuation was decided.
	candidate := types.VerificationDiagnostic{Runner: command.Runner, ProbeExecutionObservations: []types.VerificationProbeExecutionObservation{observation}}
	if len(types.CurrentReportProbeExecutionObservations(&types.ChangeReport{
		PlanID: planID, ExecutedCommands: []types.ExecutedCommand{command},
		VerificationDiagnostics: []types.VerificationDiagnostic{candidate},
	})) == 0 {
		return
	}
	observation.OutputRef = StoreBlobArtifact(ctx.WorkDir, "run_tests",
		"verification-probe-execution-"+receipt.ExecutionID+".txt", result.Output)
	result.ExecutionObservations = types.MergeVerificationProbeExecutionObservations(result.ExecutionObservations, []types.VerificationProbeExecutionObservation{observation})
	// Existing early diagnostics may carry the same observation. The final
	// attachment is idempotent; this does not create late diagnostics early.
	view := *result.Report
	view.PlanID, view.ExecutedCommands = planID, result.Commands
	attachVerificationProbeExecutionObservations(&view, result.ExecutionObservations)
}

func attachVerificationProbeExecutionObservations(report *types.ChangeReport, observations []types.VerificationProbeExecutionObservation) {
	if report == nil {
		return
	}
	for _, observation := range observations {
		if observation.PlanID != report.PlanID {
			continue
		}
		for _, command := range report.ExecutedCommands {
			receipt := command.ProbeExecution
			if receipt == nil || observation.ExecutionID != receipt.ExecutionID ||
				observation.DefinitionSHA256 != receipt.DefinitionSHA256 || observation.InvocationSHA256 != receipt.InvocationSHA256 {
				continue
			}
			for i := range report.VerificationDiagnostics {
				diagnostic := &report.VerificationDiagnostics[i]
				if probeExecutionDiagnosticMatchesCommand(*diagnostic, command) {
					diagnostic.ProbeExecutionObservations = types.MergeVerificationProbeExecutionObservations(diagnostic.ProbeExecutionObservations, []types.VerificationProbeExecutionObservation{observation})
				}
			}
		}
	}
}

func probeExecutionDiagnosticMatchesCommand(diagnostic types.VerificationDiagnostic, command types.ExecutedCommand) bool {
	// These are producer-supplied fields, compared verbatim. No parsing of the
	// display command, no category/outcome widening, and no message heuristics.
	return diagnostic.Runner == command.Runner && diagnostic.Framework == command.Framework &&
		diagnostic.Source == command.Source && diagnostic.WorkingDir == command.WorkingDir &&
		diagnostic.Command == command.Command && diagnostic.ExitCode == command.ExitCode
}
