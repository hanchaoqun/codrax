package orchestrator

import "github.com/hanchaoqun/codrax/internal/types"

// This is an independent supplement to system-owned verification cards. It
// never changes their verdict or edits a model-authored answer.
func renderVerifyFailureObservationNote(report *types.ChangeReport, lang string) string {
	body := types.RenderVerificationFailureObservations(types.CurrentReportFailureObservations(report), isLangZh(lang))
	if body == "" {
		return ""
	}
	return "\n\n" + body + "\n"
}
