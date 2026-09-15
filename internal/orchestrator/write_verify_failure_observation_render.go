package orchestrator

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This is an independent supplement to system-owned verification cards. It
// never changes their verdict or edits a model-authored answer.
func renderVerifyFailureObservationNote(report *types.ChangeReport, lang string) string {
	var notes []string
	if body := types.RenderVerificationFailureObservations(types.CurrentReportFailureObservations(report), isLangZh(lang)); body != "" {
		notes = append(notes, body)
	}
	if body := types.RenderVerificationProbeExecutionObservations(types.CurrentReportProbeExecutionObservations(report), isLangZh(lang)); body != "" {
		notes = append(notes, body)
	}
	if len(notes) == 0 {
		return ""
	}
	return "\n\n" + strings.Join(notes, "\n\n") + "\n"
}
