package agent

import "github.com/hanchaoqun/codrax/internal/types"

// explicitRuntimeTraceArtifactOnlyRequest reports whether the current request
// carries an anchored typed policy that excludes current-checkout evidence for
// an external trace artifact. It deliberately does not inspect raw user prose;
// default external observations remain mixed with current-source analysis, and
// log-only observation exclusions use the generic runtime-artifact path.
func explicitRuntimeTraceArtifactOnlyRequest(ctx *types.AgentContext) bool {
	if ctx == nil || ctx.AnalysisIR == nil {
		return false
	}
	rm := ctx.AnalysisIR.RequestModel
	if rm.PerfTrace == nil {
		return false
	}
	if !rm.HasExternalOnlyRuntimeArtifact() {
		return false
	}
	return rm.HasObservationOnlyRuntimeArtifact()
}
