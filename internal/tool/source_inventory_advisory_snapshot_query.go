package tool

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

func sourceInventoryAdvisorySnapshotQuery(ctx *types.BusContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	if profile := ctx.AnalysisIR.RequestModel.SourceInventoryProfile; profile != nil && profile.Active() {
		// Category inventories need the complete typed role/scope universe. The
		// profile quotes are category context, not a closed symbol selector. Exact
		// construct families are intersected with parser-owned SurfaceTerms later.
		if ctx.AnalysisIR.RequestModel.Predicates.IsCategoryEnumeration {
			return ""
		}
		if query := strings.Join(trimStringSlice(profile.SourceQuotes), " "); strings.TrimSpace(query) != "" {
			return query
		}
		if profile.Confidence > 0 && profile.Confidence < 0.70 {
			return sourceInventoryAdvisoryQueryFromRequest(ctx)
		}
		return ""
	}
	return sourceInventoryAdvisoryQueryFromRequest(ctx)
}

func sourceInventoryAdvisoryQueryFromRequest(ctx *types.BusContext) string {
	if ctx == nil || ctx.AnalysisIR == nil {
		return ""
	}
	policy := types.CompileRepoMapNavigationPolicy(ctx.AnalysisIR.RequestModel, &ctx.AnalysisIR.AnswerContract, ctx.ExploreLanePlan)
	return strings.Join(policy.QueryTerms, " ")
}
