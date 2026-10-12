package agent

import (
	"fmt"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Only display consumers use this source-owned request ruler. The original
// request, complete observations and causal/supplement inputs stay unchanged.
func answerDocRuntimeDisplayScope(ctx *types.AgentContext, ledger types.ObservationLedger, record types.ObservationRecord) (*types.RuntimeArtifactScopeProfile, bool) {
	requested := ledger.RuntimeArtifactScopeProfile
	if ctx != nil && ctx.AnalysisIR != nil {
		requested = ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
	}
	return ledger.RuntimeDisplayScope(record, requested)
}

func traceQueryObservationSourceRequestedScopeNote(record types.ObservationRecord, ledger types.ObservationLedger, lang string) string {
	requested, qualified := ledger.RuntimeDisplayScope(record, ledger.RuntimeArtifactScopeProfile)
	if !qualified {
		return "original-source/window ownership unverified; background only"
	}
	return traceQueryObservationRequestedScopeNote(record, requested, lang)
}

func answerDocComparisonScopeBoundary(ctx *types.AgentContext, count int) string {
	if count == 0 || ctx == nil || ctx.AnalysisIR == nil || ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile == nil || ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.Comparison == nil {
		return ""
	}
	return fmt.Sprintf("- %d query observations are outside their own original input's requested window, or lack a current original-source/time receipt. They remain unchanged in the full ledger and audit as background/boundary evidence, not this comparison's principal facts. Each original has its own window; an unspecified side does not inherit the other side's window. Valid local chain projections and their full closing witnesses remain available.\n\n", count)
}
