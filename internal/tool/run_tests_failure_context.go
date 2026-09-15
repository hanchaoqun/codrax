package tool

import (
	"fmt"

	"github.com/hanchaoqun/codrax/internal/types"
)

const runnerFailureDetailMaxBytes = 8 * 1024

func renderRunTestsFailureContextSummary(report *types.ChangeReport) string {
	var summary string
	if failures := types.RenderVerificationRunnerFailures(report); failures != "" {
		summary = "\n\n" + failures
	}
	if observations := types.RenderVerificationProbeExecutionObservations(types.CurrentReportProbeExecutionObservations(report), false); observations != "" {
		summary += "\n\n" + observations
	}
	return summary
}

// boundedRunnerFailureDetail preserves short output exactly. A composite
// runner has no assertion protocol: keep both output ends by position only,
// without guessing which compiler, exception or business sentence is causal.
func boundedRunnerFailureDetail(output string) string {
	if len(output) <= runnerFailureDetailMaxBytes {
		return output
	}
	const marker = "\n…[runner output excerpt: omitted %d bytes]…\n"
	available := runnerFailureDetailMaxBytes - len(fmt.Sprintf(marker, len(output)))
	head := types.CutPrefixRuneSafe(output, available/3)
	tail := types.CutSuffixRuneSafe(output, available-available/3)
	return head + fmt.Sprintf(marker, len(output)-len(head)-len(tail)) + tail
}
