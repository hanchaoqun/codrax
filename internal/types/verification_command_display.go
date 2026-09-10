package types

import "strings"

// VerificationCommandNonExecutionDisplay is display-only. An exact suite-skip
// outcome has no measured exit code; its existing report fields and proof
// classification remain unchanged. Other outcomes retain their current display.
func VerificationCommandNonExecutionDisplay(cmd ExecutedCommand) string {
	if strings.TrimSpace(cmd.Outcome) != ExecutedCommandOutcomeSuiteSkipped {
		return ""
	}
	if strings.TrimSpace(cmd.Source) == "probe_primary_suite_skipped" {
		return "execution=not_run (verification policy: bounded probes passed; not evidence of a missing environment; verifier labels are not customer-source search targets)"
	}
	return "execution=not_run (suite skipped; source does not establish why)"
}
