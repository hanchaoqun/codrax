package tool

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Invocation setup belongs to the actual execution path, not inventory/schema
// command descriptions. No model-supplied field selects the reporting nonce.
// Other reporters retain their current adapters pending their own provenance
// support; in particular Gradle cache replay is not a live afterTest receipt.
func prepareJUnitRunnerInvocation(plan runnerPlan, command, extraFile string) (*junitInvocation, string, string, error) {
	if plan.Runner == "java" && detectJavaBuildSystem(plan.Root) == "maven" {
		invocation, err := newMavenJUnitInvocation(plan.Root)
		if err != nil {
			return nil, command, extraFile, err
		}
		return invocation, command + " " + shellQuoteWord("-Dsurefire.reportNameSuffix="+invocation.MavenSuffix), extraFile, nil
	}
	if plan.Runner == "cmake" {
		invocation, err := newCTestJUnitInvocation(plan.Root)
		if err != nil {
			return nil, command, extraFile, err
		}
		// The old argument is produced immediately above by our CTest adapter,
		// not searched for in caller input or command output. Replace the exact
		// option-and-argument pair once, keeping selector/build-dir bytes intact.
		oldArgument := "--output-junit " + fmt.Sprintf("%q", extraFile)
		if extraFile == "" || strings.Count(command, oldArgument) != 1 {
			invocation.Cleanup()
			return nil, command, extraFile, fmt.Errorf("CTest invocation has no unique declared report argument")
		}
		shell, _ := shellSpec()
		bound := strings.Replace(command, oldArgument, "--output-junit "+junitReportArgumentForShell(invocation.ReportPath, shell), 1)
		return invocation, bound, invocation.ReportPath, nil
	}
	return nil, command, extraFile, nil
}

func junitReportArgumentForShell(reportPath, shell string) string {
	name := strings.TrimSuffix(strings.ToLower(filepath.Base(shell)), ".exe")
	if name == "cmd" {
		// Preserve the native adapter's double-quoted representation on the
		// cmd fallback. POSIX single quotes are literal filename characters
		// there. Native Windows shell/path coverage remains a separate lane.
		return fmt.Sprintf("%q", reportPath)
	}
	return shellQuoteWord(reportPath)
}

func junitInvocationUnavailableReport(reason string, err error) *types.ChangeReport {
	return &types.ChangeReport{
		Passed: false, FailureKind: types.FailureKindVerificationIncomplete,
		FailureReasonCode: reason,
		FailureSummary:    fmt.Sprintf("no usable test report bound to this invocation: %v; existing reports were not treated as current test results", err),
	}
}
