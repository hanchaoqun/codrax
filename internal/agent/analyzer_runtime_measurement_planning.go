package agent

import (
	"github.com/hanchaoqun/codrax/internal/analysis/amplifier"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Material origins are checked outside the pure amplifier. A single attachment
// is not necessarily a single physical population (e.g. a trace bundle), and a
// preview alone cannot reconstruct the preparer's in-process receipt.
func analyzerRuntimeMeasurementPlanningFacts(ctx *types.AgentContext, rm types.RequestModel) amplifier.PlanningFacts {
	if ctx == nil || ctx.AttachedTraceMaterial == nil ||
		!ctx.AttachedTraceMaterial.SinglePhysicalSource() ||
		ctx.AttachedTraceMaterial.Validate(ctx.Context(), ctx.AttachedHitrace) != nil {
		return amplifier.PlanningFacts{}
	}
	viewCtx := ctx.ShallowClone()
	viewCtx.AnalysisIR = &types.AnalysisIR{RequestModel: rm}
	view := types.RuntimeArtifactSelectionViewFromAgentContext(viewCtx)
	item, single := view.SingleTraceArtifact()
	if !single || view.TraceCount != 1 || view.LogCount != 0 || len(view.Items) != 1 ||
		!ctx.AttachedTraceMaterial.MatchesPath(item.Source) {
		return amplifier.PlanningFacts{}
	}
	return amplifier.PlanningFacts{SinglePhysicalRuntimeArtifact: true}
}
