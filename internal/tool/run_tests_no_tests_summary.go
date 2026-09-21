package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func renderNoTestsInvocationSummary(report *types.ChangeReport) string {
	labels := report.NoTestsInvocationLabels()
	if len(labels) == 0 {
		return ""
	}
	return "\n\nNote: the following invocation scope(s) had no test-case verdict: " + strings.Join(labels, ", ") +
		". Other invocations' assertion results are retained independently. " +
		"Tests may be absent, unmatched, or unavailable; this alone does not prove execution or success. " +
		"Any compile/syntax observations are limited to the recorded successful checks and their exact input paths."
}
